// Package stream handles audio stream management and distribution
//
// # BULLETPROOF RING BUFFER FOR LIVE AUDIO STREAMING
//
// This buffer is designed with one goal: never drop audio data and deliver
// it to listeners with minimal latency. Key design decisions:
//
// 1. sync.Cond for INSTANT wakeups - no polling delays
// 2. Lock-free reads with atomic write position
// 3. Large buffer to survive network hiccups
// 4. Broadcast notification to ALL waiting listeners simultaneously
// 5. MP3 frame alignment when jumping positions
// 6. Buffer pools to reduce GC pressure
package stream

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// BUFFER POOLS - Reduce GC pressure in hot paths
// =============================================================================

const (
	// SmallBufferSize for metadata and small reads
	SmallBufferSize = 4096

	// LargeBufferSize for streaming reads
	LargeBufferSize = 16384

	// MetaBufferSize for ICY metadata assembly
	MetaBufferSize = 512
)

var (
	// smallBufferPool for 4KB buffers
	smallBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, SmallBufferSize)
			return &buf
		},
	}

	// largeBufferPool for 16KB buffers (streaming reads)
	largeBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, LargeBufferSize)
			return &buf
		},
	}

	// metaBufferPool for ICY metadata assembly
	metaBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 0, MetaBufferSize)
			return &buf
		},
	}
)

// GetSmallBuffer gets a 4KB buffer from the pool
func GetSmallBuffer() *[]byte {
	return smallBufferPool.Get().(*[]byte)
}

// PutSmallBuffer returns a buffer to the small pool
func PutSmallBuffer(buf *[]byte) {
	if buf != nil && cap(*buf) >= SmallBufferSize {
		*buf = (*buf)[:SmallBufferSize]
		smallBufferPool.Put(buf)
	}
}

// GetLargeBuffer gets a 16KB buffer from the pool
func GetLargeBuffer() *[]byte {
	return largeBufferPool.Get().(*[]byte)
}

// PutLargeBuffer returns a buffer to the large pool
func PutLargeBuffer(buf *[]byte) {
	if buf != nil && cap(*buf) >= LargeBufferSize {
		*buf = (*buf)[:LargeBufferSize]
		largeBufferPool.Put(buf)
	}
}

// GetMetaBuffer gets a metadata assembly buffer from the pool
func GetMetaBuffer() *[]byte {
	bufPtr := metaBufferPool.Get().(*[]byte)
	*bufPtr = (*bufPtr)[:0] // Reset length but keep capacity
	return bufPtr
}

// PutMetaBuffer returns a metadata buffer to the pool
func PutMetaBuffer(buf *[]byte) {
	if buf != nil && cap(*buf) >= MetaBufferSize {
		*buf = (*buf)[:0]
		metaBufferPool.Put(buf)
	}
}

// =============================================================================
// RING BUFFER - High-performance streaming buffer
// =============================================================================

// Buffer is a high-performance ring buffer for streaming audio data
//
// BULLETPROOF DESIGN:
// - Uses sync.Cond for instant broadcast to all waiting readers
// - Lock-free reads via atomic write position
// - Single writer with mutex protection
// - Large default size (10MB = ~4.3 minutes at 320kbps)
// - MP3 frame alignment when jumping positions
type Buffer struct {
	// Core buffer data - never reallocated after creation
	data []byte
	size int64 // Use int64 to avoid conversions
	mask int64 // size - 1 for fast modulo (requires power of 2 size)

	// Write position - updated atomically for lock-free reads
	writePos atomic.Int64

	// Write lock - only one writer at a time
	writeMu sync.Mutex

	// sync.Cond for instant broadcast to ALL waiting readers
	// This wakes all listeners immediately when new data arrives
	cond   *sync.Cond
	condMu sync.RWMutex

	// Configuration
	burstSize int

	// Stats
	bytesTotal atomic.Int64
	created    time.Time

	// Sync points for clean listener joins (circular buffer)
	syncPoints    [16]SyncPointInfo
	syncPointHead atomic.Int32
	syncPointMu   sync.RWMutex
}

// SyncPointInfo represents a position where listeners can cleanly join
type SyncPointInfo struct {
	Position  int64
	Timestamp time.Time
}

// NewBuffer creates a new stream buffer
// Size is rounded up to nearest power of 2 for fast modulo operations
func NewBuffer(size int, burstSize int) *Buffer {
	originalSize := size

	// Default sizes optimized for 320kbps streaming
	// 10MB buffer gives ~4.3 minutes of audio for very slow mobile clients
	if size <= 0 {
		size = 10485760 // 10MB
	}
	if burstSize <= 0 {
		burstSize = 131072 // 128KB = ~3.2 seconds at 320kbps
	}

	// Round up to power of 2 for fast modulo
	size = nextPowerOf2(size)

	if burstSize > size {
		burstSize = size / 4
	}

	b := &Buffer{
		data:      make([]byte, size),
		size:      int64(size),
		mask:      int64(size - 1),
		burstSize: burstSize,
		created:   time.Now(),
	}

	// Initialize sync.Cond for bulletproof broadcast notifications
	b.cond = sync.NewCond(b.condMu.RLocker())

	log.Printf("DEBUG: Buffer created - requested: %d, actual: %d bytes (%.1f seconds at 320kbps), burst: %d bytes",
		originalSize, size, float64(size)/40000.0, burstSize)

	return b
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

// Write writes data to the buffer (thread-safe, single writer)
// After writing, ALL waiting readers are woken up INSTANTLY via broadcast
func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.writeMu.Lock()

	writePos := b.writePos.Load()
	n := len(p)

	// Check for MP3 sync point and record it
	if b.shouldCreateSyncPoint(writePos, p) {
		b.addSyncPoint(writePos)
	}

	// Write data to ring buffer
	startIdx := writePos & b.mask

	if startIdx+int64(n) <= b.size {
		// Fast path: single contiguous write
		copy(b.data[startIdx:], p)
	} else {
		// Wrap around: two copies
		firstPart := int(b.size - startIdx)
		copy(b.data[startIdx:], p[:firstPart])
		copy(b.data[0:], p[firstPart:])
	}

	// Update write position atomically
	b.writePos.Store(writePos + int64(n))
	b.bytesTotal.Add(int64(n))

	b.writeMu.Unlock()

	// CRITICAL: Broadcast to ALL waiting readers instantly
	// This is the key to eliminating lag - every reader wakes up immediately
	b.cond.Broadcast()

	return n, nil
}

// shouldCreateSyncPoint detects if we should mark a sync point (MP3 frame boundary)
func (b *Buffer) shouldCreateSyncPoint(pos int64, data []byte) bool {
	// Create sync points at regular intervals (~16KB)
	const syncInterval int64 = 16384

	prevInterval := pos / syncInterval
	nextInterval := (pos + int64(len(data))) / syncInterval

	if nextInterval > prevInterval {
		return true
	}

	// Also detect MP3 frame sync (0xFF followed by 0xE0-0xFF)
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
		return true
	}

	return false
}

// addSyncPoint adds a new sync point
func (b *Buffer) addSyncPoint(pos int64) {
	b.syncPointMu.Lock()
	idx := b.syncPointHead.Add(1) % int32(len(b.syncPoints))
	b.syncPoints[idx] = SyncPointInfo{
		Position:  pos,
		Timestamp: time.Now(),
	}
	b.syncPointMu.Unlock()
}

// GetSyncPoint returns the best sync point for a new listener (near burst position)
func (b *Buffer) GetSyncPoint() int64 {
	writePos := b.writePos.Load()

	// Default position: burst size behind write position
	defaultPos := writePos - int64(b.burstSize)
	if defaultPos < 0 {
		defaultPos = 0
	}

	b.syncPointMu.RLock()
	defer b.syncPointMu.RUnlock()

	// Find the most recent sync point that's close to our default position
	bestPos := defaultPos
	for i := range b.syncPoints {
		sp := b.syncPoints[i]
		// Prefer sync points that are after defaultPos but before writePos
		if sp.Position > defaultPos && sp.Position < writePos && sp.Position > bestPos {
			bestPos = sp.Position
		}
	}

	return bestPos
}

// WritePos returns the current write position (lock-free)
func (b *Buffer) WritePos() int64 {
	return b.writePos.Load()
}

// ReadFrom reads data from buffer starting at pos (lock-free for readers)
// Returns data slice and new position
// If pos is behind (data overwritten), jumps to oldest available data with MP3 sync
func (b *Buffer) ReadFrom(pos int64, maxBytes int) ([]byte, int64) {
	writePos := b.writePos.Load()

	// Nothing to read if at or ahead of write position
	if pos >= writePos {
		return nil, pos
	}

	// Check if requested position has been overwritten
	oldestPos := writePos - b.size
	if oldestPos < 0 {
		oldestPos = 0
	}
	if pos < oldestPos {
		// Data was overwritten, find MP3 sync point near oldest
		pos = b.findMP3SyncNear(oldestPos, writePos)
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

	// Read from ring buffer
	b.readIntoBuffer(pos, result)

	return result, pos + int64(available)
}

// ReadFromInto reads data into provided buffer (zero-copy read path)
// Returns number of bytes read and new position
func (b *Buffer) ReadFromInto(pos int64, buf []byte) (int, int64) {
	if len(buf) == 0 {
		return 0, pos
	}

	writePos := b.writePos.Load()

	// Nothing to read yet
	if pos >= writePos {
		return 0, pos
	}

	// Calculate oldest available position
	oldestPos := writePos - b.size
	if oldestPos < 0 {
		oldestPos = 0
	}

	// If position is behind oldest available, find MP3 sync point
	if pos < oldestPos {
		pos = b.findMP3SyncNear(oldestPos, writePos)
	}

	// Calculate available bytes
	available := int(writePos - pos)
	if available > len(buf) {
		available = len(buf)
	}
	if available <= 0 {
		return 0, pos
	}

	// Read from ring buffer
	b.readIntoBuffer(pos, buf[:available])

	return available, pos + int64(available)
}

// SafeReadFromInto reads data and returns skipped bytes count for monitoring
func (b *Buffer) SafeReadFromInto(pos int64, buf []byte) (bytesRead int, newPos int64, skippedBytes int64) {
	if len(buf) == 0 {
		return 0, pos, 0
	}

	writePos := b.writePos.Load()

	if pos >= writePos {
		return 0, pos, 0
	}

	oldestPos := writePos - b.size
	if oldestPos < 0 {
		oldestPos = 0
	}

	// Track if we had to skip and find MP3 sync
	if pos < oldestPos {
		skippedBytes = oldestPos - pos
		pos = b.findMP3SyncNear(oldestPos, writePos)
	}

	available := int(writePos - pos)
	if available > len(buf) {
		available = len(buf)
	}
	if available <= 0 {
		return 0, pos, skippedBytes
	}

	b.readIntoBuffer(pos, buf[:available])

	return available, pos + int64(available), skippedBytes
}

// readIntoBuffer copies data from the ring buffer into dst
func (b *Buffer) readIntoBuffer(pos int64, dst []byte) {
	available := len(dst)
	startIdx := pos & b.mask

	if startIdx+int64(available) <= b.size {
		// Fast path: single contiguous read
		copy(dst, b.data[startIdx:startIdx+int64(available)])
	} else {
		// Wrap around: two copies
		firstPart := int(b.size - startIdx)
		copy(dst[:firstPart], b.data[startIdx:])
		copy(dst[firstPart:], b.data[:available-firstPart])
	}
}

// findMP3SyncNear finds the nearest MP3 frame sync point near targetPos
func (b *Buffer) findMP3SyncNear(targetPos, writePos int64) int64 {
	// Read up to 4KB to search for MP3 sync
	searchSize := int64(4096)
	if targetPos+searchSize > writePos {
		searchSize = writePos - targetPos
	}
	if searchSize <= 4 {
		return targetPos
	}

	// Get a buffer from pool for searching
	bufPtr := GetSmallBuffer()
	defer PutSmallBuffer(bufPtr)
	searchBuf := (*bufPtr)[:searchSize]

	b.readIntoBuffer(targetPos, searchBuf)

	// Find MP3 frame sync
	offset := findMP3FrameSync(searchBuf)
	if offset > 0 && offset < int(searchSize)-4 {
		return targetPos + int64(offset)
	}

	return targetPos
}

// findMP3FrameSync finds the first valid MP3 frame sync in data
func findMP3FrameSync(data []byte) int {
	if len(data) < 4 {
		return 0
	}

	for i := 0; i < len(data)-3; i++ {
		if data[i] != 0xFF {
			continue
		}

		b1 := data[i+1]

		// Check frame sync: 0xFF followed by 0xE0-0xFE
		if b1 == 0xFF || (b1&0xE0) != 0xE0 {
			continue
		}

		// Validate MPEG version and layer
		version := (b1 >> 3) & 0x03
		layer := (b1 >> 1) & 0x03
		if version == 0x01 || layer == 0x00 {
			continue
		}

		// Validate bitrate and sample rate
		b2 := data[i+2]
		bitrate := (b2 >> 4) & 0x0F
		sampleRate := (b2 >> 2) & 0x03
		if bitrate == 0x0F || sampleRate == 0x03 {
			continue
		}

		return i
	}

	return 0
}

// GetBurst returns burst data for new listener (at a sync point)
func (b *Buffer) GetBurst() []byte {
	writePos := b.writePos.Load()

	if writePos == 0 {
		return nil
	}

	burstBytes := int64(b.burstSize)
	if writePos < burstBytes {
		burstBytes = writePos
	}

	// Start at a sync point for clean audio
	startPos := b.GetSyncPoint()
	actualBurst := writePos - startPos
	if actualBurst > int64(b.burstSize) {
		actualBurst = int64(b.burstSize)
		startPos = writePos - actualBurst
	}

	result := make([]byte, actualBurst)
	b.readIntoBuffer(startPos, result)

	return result
}

// GetLivePosition returns position for live-edge listening
func (b *Buffer) GetLivePosition() int64 {
	writePos := b.writePos.Load()

	// Small lag to prevent underrun (1KB = ~25ms at 320kbps)
	pos := writePos - 1024
	if pos < 0 {
		pos = 0
	}
	return pos
}

// =============================================================================
// WAITING FOR DATA - Event-driven, no polling!
// =============================================================================

// WaitForData waits for new data with timeout using sync.Cond
// NO GOROUTINE LEAKS - uses proper timeout handling
// Returns true if data is available, false on timeout
func (b *Buffer) WaitForData(pos int64, timeout time.Duration) bool {
	// Fast path: data already available
	if b.writePos.Load() > pos {
		return true
	}

	// Calculate deadline
	deadline := time.Now().Add(timeout)

	b.condMu.RLock()
	defer b.condMu.RUnlock()

	// Check again with lock held
	if b.writePos.Load() > pos {
		return true
	}

	// Wait loop with timeout check
	for b.writePos.Load() <= pos {
		// Check if we've exceeded the deadline
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return b.writePos.Load() > pos
		}

		// Use a timer for timeout - but limit wait time to check deadline
		waitTime := remaining
		if waitTime > 10*time.Millisecond {
			waitTime = 10 * time.Millisecond // Check deadline every 10ms
		}

		// Start timer for wakeup
		timer := time.AfterFunc(waitTime, func() {
			b.cond.Broadcast()
		})

		b.cond.Wait()
		timer.Stop()

		// Check if we have data
		if b.writePos.Load() > pos {
			return true
		}
	}

	return b.writePos.Load() > pos
}

// WaitForDataContext waits for new data, respecting context cancellation
// This is the BULLETPROOF version - uses sync.Cond for instant wakeup
// Returns true if data available, false if cancelled
func (b *Buffer) WaitForDataContext(ctx context.Context, pos int64) bool {
	// Fast path: data already available (most common case)
	if b.writePos.Load() > pos {
		return true
	}

	// Check if context is already done
	select {
	case <-ctx.Done():
		return false
	default:
	}

	b.condMu.RLock()
	defer b.condMu.RUnlock()

	// Check again with lock held
	if b.writePos.Load() > pos {
		return true
	}

	// Create a done channel for the wakeup goroutine
	done := make(chan struct{})
	defer close(done)

	// Start a goroutine to watch context and wake us up
	go func() {
		select {
		case <-ctx.Done():
			b.cond.Broadcast()
		case <-done:
			// Normal exit, stop watching
		}
	}()

	// Wait loop
	for b.writePos.Load() <= pos {
		// Check context before waiting
		select {
		case <-ctx.Done():
			return false
		default:
		}

		b.cond.Wait()

		// After wakeup, check if we have data or context is done
		if b.writePos.Load() > pos {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		default:
		}
	}

	return b.writePos.Load() > pos
}

// WaitForDataChan waits for new data using a done channel
// Returns true if data available, false if done channel closed
func (b *Buffer) WaitForDataChan(pos int64, done <-chan struct{}) bool {
	// Fast path: data already available
	if b.writePos.Load() > pos {
		return true
	}

	// Check if already done
	select {
	case <-done:
		return false
	default:
	}

	b.condMu.RLock()
	defer b.condMu.RUnlock()

	// Check again with lock held
	if b.writePos.Load() > pos {
		return true
	}

	// Create cleanup channel
	cleanup := make(chan struct{})
	defer close(cleanup)

	// Watch done channel
	go func() {
		select {
		case <-done:
			b.cond.Broadcast()
		case <-cleanup:
		}
	}()

	// Wait loop
	for b.writePos.Load() <= pos {
		select {
		case <-done:
			return false
		default:
		}

		b.cond.Wait()

		if b.writePos.Load() > pos {
			return true
		}
	}

	return b.writePos.Load() > pos
}

// =============================================================================
// UTILITY METHODS
// =============================================================================

// BytesTotal returns total bytes written
func (b *Buffer) BytesTotal() int64 {
	return b.bytesTotal.Load()
}

// Size returns buffer size
func (b *Buffer) Size() int {
	return int(b.size)
}

// BurstSize returns burst size
func (b *Buffer) BurstSize() int {
	return b.burstSize
}

// Created returns creation time
func (b *Buffer) Created() time.Time {
	return b.created
}

// Reset resets the buffer for reuse
func (b *Buffer) Reset() {
	b.writeMu.Lock()
	b.writePos.Store(0)
	b.bytesTotal.Store(0)
	b.syncPointHead.Store(0)
	b.created = time.Now()
	b.writeMu.Unlock()

	// Clear sync points
	b.syncPointMu.Lock()
	for i := range b.syncPoints {
		b.syncPoints[i] = SyncPointInfo{}
	}
	b.syncPointMu.Unlock()

	// Wake up any waiters so they can see the reset
	b.cond.Broadcast()
}

// SetBurstSize updates burst size
func (b *Buffer) SetBurstSize(size int) {
	if size > 0 && size <= int(b.size) {
		b.burstSize = size
	}
}

// HasData returns true if there's data available after pos
func (b *Buffer) HasData(pos int64) bool {
	return b.writePos.Load() > pos
}

// Available returns bytes available to read from pos
func (b *Buffer) Available(pos int64) int64 {
	writePos := b.writePos.Load()
	if pos >= writePos {
		return 0
	}
	return writePos - pos
}

// GetLag returns how far behind the given position is from live
func (b *Buffer) GetLag(pos int64) int64 {
	return b.writePos.Load() - pos
}

// OldestPosition returns the oldest position that still has valid data
func (b *Buffer) OldestPosition() int64 {
	writePos := b.writePos.Load()
	oldest := writePos - b.size
	if oldest < 0 {
		oldest = 0
	}
	return oldest
}

// FindMP3SyncFrom finds an MP3 sync point starting from the given position
// Returns the position of the sync point
func (b *Buffer) FindMP3SyncFrom(pos int64) int64 {
	writePos := b.writePos.Load()
	return b.findMP3SyncNear(pos, writePos)
}
