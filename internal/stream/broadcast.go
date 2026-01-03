// Package stream provides state-of-the-art audio streaming primitives
// This file implements a lock-free broadcast buffer with listener synchronization

package stream

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ---------------------------------------------------------
// CONSTANTS
// ---------------------------------------------------------

const (
	// CacheLineSize for avoiding false sharing
	CacheLineSize = 64

	// DefaultBroadcastBufferSize is 1MB - enough for ~26 seconds of 320kbps audio
	// This large buffer prevents any data loss even with slow listeners
	DefaultBroadcastBufferSize = 1024 * 1024

	// MaxListenerLag before forcing skip-to-live (512KB = ~13 seconds of 320kbps audio)
	// Only skip if listener is EXTREMELY far behind - almost never in practice
	MaxListenerLag = 512 * 1024

	// MinChunkSize for frame-aligned writes
	MinChunkSize = 417 // Typical MP3 frame size at 128kbps

	// SyncPointInterval - create sync points every N bytes
	SyncPointInterval = 16 * 1024

	// BytePoolSize for zero-copy operations
	BytePoolSize = 4096
)

// ---------------------------------------------------------
// BYTE POOL - Reduces GC pressure
// ---------------------------------------------------------

var bytePool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, BytePoolSize)
		return &buf
	},
}

// GetPooledBuffer gets a buffer from the pool
func GetPooledBuffer() *[]byte {
	return bytePool.Get().(*[]byte)
}

// PutPooledBuffer returns a buffer to the pool
func PutPooledBuffer(buf *[]byte) {
	if buf != nil && cap(*buf) >= BytePoolSize {
		*buf = (*buf)[:BytePoolSize]
		bytePool.Put(buf)
	}
}

// ---------------------------------------------------------
// CACHE-LINE PADDED ATOMICS - Avoids false sharing
// ---------------------------------------------------------

// PaddedInt64 is a cache-line padded atomic int64
type PaddedInt64 struct {
	value int64
	_     [CacheLineSize - 8]byte // Padding to fill cache line
}

// Load atomically loads the value
func (p *PaddedInt64) Load() int64 {
	return atomic.LoadInt64(&p.value)
}

// Store atomically stores the value
func (p *PaddedInt64) Store(val int64) {
	atomic.StoreInt64(&p.value, val)
}

// Add atomically adds to the value
func (p *PaddedInt64) Add(delta int64) int64 {
	return atomic.AddInt64(&p.value, delta)
}

// ---------------------------------------------------------
// SYNC POINT - For clean listener joins
// ---------------------------------------------------------

// SyncPoint represents a position where listeners can cleanly join
type SyncPoint struct {
	Position  int64
	Timestamp time.Time
	FrameType byte // 0 = unknown, 1 = MP3 sync, 2 = AAC sync, 3 = silence
}

// ---------------------------------------------------------
// BROADCAST BUFFER - Lock-free SPMC ring buffer
// ---------------------------------------------------------

// BroadcastBuffer is a lock-free single-producer, multiple-consumer ring buffer
// optimized for audio streaming with listener synchronization
type BroadcastBuffer struct {
	// Data buffer (cache-line aligned)
	data []byte
	mask int64 // size - 1 for fast modulo via bitwise AND

	// Write position (only modified by producer)
	writePos PaddedInt64

	// Sync points for clean listener joins
	syncPoints    [16]SyncPoint // Circular buffer of recent sync points
	syncPointHead int32
	syncPointMu   sync.RWMutex

	// Statistics
	bytesWritten PaddedInt64
	startTime    time.Time

	// Configuration
	size      int
	burstSize int

	// Frame detection state
	lastFrameEnd int64
	frameBuffer  []byte
}

// NewBroadcastBuffer creates a new lock-free broadcast buffer
// Size must be a power of 2 for efficient modulo operations
func NewBroadcastBuffer(size, burstSize int) *BroadcastBuffer {
	// Round up to power of 2
	size = nextPowerOf2(size)
	if size < 16384 {
		size = 16384 // Minimum 16KB
	}

	if burstSize <= 0 {
		burstSize = 8192 // 8KB default burst
	}
	if burstSize > size/4 {
		burstSize = size / 4
	}

	return &BroadcastBuffer{
		data:        make([]byte, size),
		mask:        int64(size - 1),
		size:        size,
		burstSize:   burstSize,
		startTime:   time.Now(),
		frameBuffer: make([]byte, 0, 4096),
	}
}

// nextPowerOf2 returns the next power of 2 >= n
func nextPowerOf2(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

// Write writes data to the buffer (producer only - not thread safe for multiple writers)
// Returns bytes written
func (b *BroadcastBuffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	writePos := b.writePos.Load()
	n := len(p)

	// Check for sync point (frame boundary detection)
	if b.shouldCreateSyncPoint(writePos, p) {
		b.addSyncPoint(writePos)
	}

	// Write to ring buffer using bitwise AND for fast modulo
	for i := 0; i < n; i++ {
		b.data[(writePos+int64(i))&b.mask] = p[i]
	}

	// Memory barrier + atomic update
	b.writePos.Add(int64(n))
	b.bytesWritten.Add(int64(n))

	return n, nil
}

// WriteBatch writes data in a cache-friendly batch operation
func (b *BroadcastBuffer) WriteBatch(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	writePos := b.writePos.Load()
	n := len(p)

	// Check for sync point
	if b.shouldCreateSyncPoint(writePos, p) {
		b.addSyncPoint(writePos)
	}

	// Calculate positions using bitwise AND
	startIdx := writePos & b.mask
	endIdx := (writePos + int64(n)) & b.mask

	if startIdx < endIdx || endIdx == 0 {
		// No wrap - single copy
		copy(b.data[startIdx:], p)
	} else {
		// Wrap around - two copies
		firstPart := int64(b.size) - startIdx
		copy(b.data[startIdx:], p[:firstPart])
		copy(b.data[0:], p[firstPart:])
	}

	b.writePos.Add(int64(n))
	b.bytesWritten.Add(int64(n))

	return n, nil
}

// shouldCreateSyncPoint detects if we should mark a sync point
func (b *BroadcastBuffer) shouldCreateSyncPoint(pos int64, data []byte) bool {
	// Create sync points at regular intervals
	prevInterval := (pos / SyncPointInterval)
	nextInterval := ((pos + int64(len(data))) / SyncPointInterval)

	if nextInterval > prevInterval {
		return true
	}

	// Also detect MP3 frame sync (0xFF 0xFB, 0xFF 0xFA, 0xFF 0xF3, 0xFF 0xF2)
	if len(data) >= 2 {
		if data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
			return true
		}
	}

	return false
}

// addSyncPoint adds a new sync point
func (b *BroadcastBuffer) addSyncPoint(pos int64) {
	b.syncPointMu.Lock()
	defer b.syncPointMu.Unlock()

	idx := atomic.AddInt32(&b.syncPointHead, 1) % int32(len(b.syncPoints))
	b.syncPoints[idx] = SyncPoint{
		Position:  pos,
		Timestamp: time.Now(),
		FrameType: 1, // MP3
	}
}

// GetSyncPoint returns the best sync point for a new listener
func (b *BroadcastBuffer) GetSyncPoint() int64 {
	writePos := b.writePos.Load()

	b.syncPointMu.RLock()
	defer b.syncPointMu.RUnlock()

	// Find the most recent sync point that's not too far behind
	bestPos := writePos - int64(b.burstSize)
	if bestPos < 0 {
		bestPos = 0
	}

	for i := range b.syncPoints {
		sp := b.syncPoints[i]
		if sp.Position > bestPos && sp.Position < writePos {
			if sp.Position > bestPos {
				bestPos = sp.Position
			}
		}
	}

	return bestPos
}

// ---------------------------------------------------------
// READER METHODS
// ---------------------------------------------------------

// WritePos returns the current write position
func (b *BroadcastBuffer) WritePos() int64 {
	return b.writePos.Load()
}

// ReadAt reads data at a specific position (lock-free)
// Returns data read and new position
func (b *BroadcastBuffer) ReadAt(pos int64, maxBytes int) ([]byte, int64) {
	writePos := b.writePos.Load()

	// No data available
	if pos >= writePos {
		return nil, pos
	}

	// Check if position is too old (data overwritten)
	oldestPos := writePos - int64(b.size)
	if oldestPos < 0 {
		oldestPos = 0
	}
	if pos < oldestPos {
		// Skip to oldest available data
		pos = oldestPos
	}

	// Calculate available bytes
	available := int(writePos - pos)
	if available > maxBytes {
		available = maxBytes
	}
	if available <= 0 {
		return nil, pos
	}

	// Allocate result buffer
	result := make([]byte, available)

	// Read using bitwise AND for fast modulo
	startIdx := pos & b.mask
	endIdx := (pos + int64(available)) & b.mask

	if startIdx < endIdx || endIdx == 0 {
		// No wrap - single copy
		copy(result, b.data[startIdx:startIdx+int64(available)])
	} else {
		// Wrap around - two copies
		firstPart := int64(b.size) - startIdx
		copy(result[:firstPart], b.data[startIdx:])
		copy(result[firstPart:], b.data[:available-int(firstPart)])
	}

	return result, pos + int64(available)
}

// GetLivePosition returns position for lowest latency listening
func (b *BroadcastBuffer) GetLivePosition() int64 {
	writePos := b.writePos.Load()

	// Start very close to live edge (just 2KB behind)
	pos := writePos - 2048
	if pos < 0 {
		pos = 0
	}
	return pos
}

// GetBurst returns burst data for new listeners
func (b *BroadcastBuffer) GetBurst() []byte {
	writePos := b.writePos.Load()

	burstBytes := int64(b.burstSize)
	if writePos < burstBytes {
		burstBytes = writePos
	}
	if burstBytes <= 0 {
		return nil
	}

	// Try to start at a sync point
	startPos := b.GetSyncPoint()
	actualBurst := writePos - startPos
	if actualBurst > int64(b.burstSize) {
		actualBurst = int64(b.burstSize)
		startPos = writePos - actualBurst
	}

	result := make([]byte, actualBurst)

	startIdx := startPos & b.mask
	endIdx := (startPos + actualBurst) & b.mask

	if startIdx < endIdx || endIdx == 0 {
		copy(result, b.data[startIdx:startIdx+actualBurst])
	} else {
		firstPart := int64(b.size) - startIdx
		copy(result[:firstPart], b.data[startIdx:])
		copy(result[firstPart:], b.data[:actualBurst-firstPart])
	}

	return result
}

// ---------------------------------------------------------
// LISTENER TRACKING
// ---------------------------------------------------------

// BroadcastListener represents a listener's position in the stream
type BroadcastListener struct {
	ID         string
	Position   PaddedInt64
	LastRead   int64 // Unix nano timestamp
	BytesSent  int64
	SkipCount  int32 // Times we've skipped to live
	Connected  time.Time
	buffer     *BroadcastBuffer
	done       chan struct{}
	skipToLive bool
}

// NewBroadcastListener creates a new listener attached to a buffer
func NewBroadcastListener(id string, buf *BroadcastBuffer) *BroadcastListener {
	l := &BroadcastListener{
		ID:        id,
		buffer:    buf,
		Connected: time.Now(),
		done:      make(chan struct{}),
	}

	// Start at a sync point for clean audio
	l.Position.Store(buf.GetSyncPoint())

	return l
}

// Read reads available data for this listener
// Automatically handles skip-to-live if listener falls too far behind
func (l *BroadcastListener) Read(maxBytes int) []byte {
	pos := l.Position.Load()
	writePos := l.buffer.WritePos()

	// Check if we're too far behind
	lag := writePos - pos
	if lag > MaxListenerLag {
		// Skip to live edge at a sync point
		newPos := l.buffer.GetSyncPoint()
		l.Position.Store(newPos)
		pos = newPos
		atomic.AddInt32(&l.SkipCount, 1)
	}

	// Read data
	data, newPos := l.buffer.ReadAt(pos, maxBytes)
	if len(data) > 0 {
		l.Position.Store(newPos)
		l.LastRead = time.Now().UnixNano()
		atomic.AddInt64(&l.BytesSent, int64(len(data)))
	}

	return data
}

// GetLag returns how far behind live edge this listener is
func (l *BroadcastListener) GetLag() int64 {
	return l.buffer.WritePos() - l.Position.Load()
}

// IsHealthy returns true if listener is keeping up
func (l *BroadcastListener) IsHealthy() bool {
	return l.GetLag() < MaxListenerLag/2
}

// Close closes the listener
func (l *BroadcastListener) Close() {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
}

// Done returns the done channel
func (l *BroadcastListener) Done() <-chan struct{} {
	return l.done
}

// ---------------------------------------------------------
// BROADCASTER - Pushes data to all listeners
// ---------------------------------------------------------

// Broadcaster manages pushing data to all listeners synchronously
type Broadcaster struct {
	buffer    *BroadcastBuffer
	listeners sync.Map // map[string]*BroadcastListener
	count     int32

	// Notify channel for new data
	notify chan struct{}

	// Control
	done chan struct{}
	wg   sync.WaitGroup
}

// NewBroadcaster creates a new broadcaster
func NewBroadcaster(bufSize, burstSize int) *Broadcaster {
	return &Broadcaster{
		buffer: NewBroadcastBuffer(bufSize, burstSize),
		notify: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
}

// Write writes data and notifies listeners
func (bc *Broadcaster) Write(p []byte) (int, error) {
	n, err := bc.buffer.WriteBatch(p)
	if err != nil {
		return n, err
	}

	// Non-blocking notify
	select {
	case bc.notify <- struct{}{}:
	default:
	}

	return n, nil
}

// AddListener adds a new listener
func (bc *Broadcaster) AddListener(id string) *BroadcastListener {
	listener := NewBroadcastListener(id, bc.buffer)
	bc.listeners.Store(id, listener)
	atomic.AddInt32(&bc.count, 1)
	return listener
}

// RemoveListener removes a listener
func (bc *Broadcaster) RemoveListener(id string) {
	if l, ok := bc.listeners.LoadAndDelete(id); ok {
		l.(*BroadcastListener).Close()
		atomic.AddInt32(&bc.count, -1)
	}
}

// GetListener returns a listener by ID
func (bc *Broadcaster) GetListener(id string) *BroadcastListener {
	if l, ok := bc.listeners.Load(id); ok {
		return l.(*BroadcastListener)
	}
	return nil
}

// ListenerCount returns number of active listeners
func (bc *Broadcaster) ListenerCount() int {
	return int(atomic.LoadInt32(&bc.count))
}

// Buffer returns the underlying buffer
func (bc *Broadcaster) Buffer() *BroadcastBuffer {
	return bc.buffer
}

// Notify returns the notification channel
func (bc *Broadcaster) Notify() <-chan struct{} {
	return bc.notify
}

// Close shuts down the broadcaster
func (bc *Broadcaster) Close() {
	close(bc.done)

	// Close all listeners
	bc.listeners.Range(func(key, value interface{}) bool {
		value.(*BroadcastListener).Close()
		return true
	})

	bc.wg.Wait()
}

// ---------------------------------------------------------
// FRAME DETECTOR - For clean audio boundaries
// ---------------------------------------------------------

// FrameType represents audio frame types
type FrameType int

const (
	FrameUnknown FrameType = iota
	FrameMP3
	FrameAAC
	FrameOgg
	FrameOpus
)

// FrameInfo contains detected frame information
type FrameInfo struct {
	Type     FrameType
	Size     int
	Bitrate  int
	Duration time.Duration
}

// DetectMP3Frame detects MP3 frame at given position
// Returns frame size or 0 if not a valid frame
func DetectMP3Frame(data []byte) int {
	if len(data) < 4 {
		return 0
	}

	// MP3 sync word: 11 bits set (0xFF followed by 0xE0 or higher in second byte)
	if data[0] != 0xFF || (data[1]&0xE0) != 0xE0 {
		return 0
	}

	// Parse header
	version := (data[1] >> 3) & 0x03
	layer := (data[1] >> 1) & 0x03
	bitrateIdx := (data[2] >> 4) & 0x0F
	samplingIdx := (data[2] >> 2) & 0x03
	padding := (data[2] >> 1) & 0x01

	// Bitrate table for MPEG1 Layer 3
	bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	samplingRates := []int{44100, 48000, 32000, 0}

	if bitrateIdx == 0 || bitrateIdx == 15 || samplingIdx == 3 {
		return 0
	}

	if version != 3 || layer != 1 { // MPEG1 Layer 3
		return 0
	}

	bitrate := bitrates[bitrateIdx] * 1000
	samplingRate := samplingRates[samplingIdx]

	// Frame size = 144 * bitrate / sampling_rate + padding
	frameSize := (144 * bitrate / samplingRate) + int(padding)

	return frameSize
}

// FindNextMP3Frame finds the next MP3 frame start in data
// Returns offset to frame start, or -1 if not found
func FindNextMP3Frame(data []byte) int {
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			frameSize := DetectMP3Frame(data[i:])
			if frameSize > 0 {
				return i
			}
		}
	}
	return -1
}

// ---------------------------------------------------------
// JITTER BUFFER - Smooths out network variations
// ---------------------------------------------------------

// JitterBuffer smooths out timing variations in data arrival
type JitterBuffer struct {
	target   time.Duration // Target buffering delay
	minDelay time.Duration
	maxDelay time.Duration

	buffer [][]byte
	times  []time.Time
	head   int
	tail   int
	size   int
	mu     sync.Mutex

	lastOutput time.Time
}

// NewJitterBuffer creates a new jitter buffer
func NewJitterBuffer(targetDelay time.Duration) *JitterBuffer {
	size := 32 // Buffer up to 32 chunks

	return &JitterBuffer{
		target:   targetDelay,
		minDelay: targetDelay / 2,
		maxDelay: targetDelay * 2,
		buffer:   make([][]byte, size),
		times:    make([]time.Time, size),
		size:     size,
	}
}

// Push adds data to the jitter buffer
func (jb *JitterBuffer) Push(data []byte) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	// Copy data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	jb.buffer[jb.tail] = dataCopy
	jb.times[jb.tail] = time.Now()
	jb.tail = (jb.tail + 1) % jb.size

	// Overflow: drop oldest
	if jb.tail == jb.head {
		jb.head = (jb.head + 1) % jb.size
	}
}

// Pop retrieves data from the jitter buffer
// Returns nil if buffer is empty or target delay not reached
func (jb *JitterBuffer) Pop() []byte {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.head == jb.tail {
		return nil // Empty
	}

	// Check if we've buffered enough
	age := time.Since(jb.times[jb.head])
	if age < jb.target {
		return nil // Not ready yet
	}

	data := jb.buffer[jb.head]
	jb.buffer[jb.head] = nil
	jb.head = (jb.head + 1) % jb.size
	jb.lastOutput = time.Now()

	return data
}

// Len returns number of items in buffer
func (jb *JitterBuffer) Len() int {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.tail >= jb.head {
		return jb.tail - jb.head
	}
	return jb.size - jb.head + jb.tail
}

// ---------------------------------------------------------
// UTILITIES
// ---------------------------------------------------------

// UnsafeString converts bytes to string without allocation
// WARNING: The string is only valid while the byte slice is valid
func UnsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// UnsafeBytes converts string to bytes without allocation
// WARNING: Do not modify the returned slice
func UnsafeBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
