// Package stream provides state-of-the-art audio streaming primitives
//
// This file provides the Broadcaster which manages pushing data to listeners,
// along with frame detection and utility functions.
//
// The main ring buffer implementation is in buffer.go.

package stream

import (
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// DefaultBufferSize is 10MB - enough for ~4.3 minutes of 320kbps audio
	DefaultBufferSize = 10 * 1024 * 1024

	// DefaultBurstSize is 128KB - ~3.2 seconds of 320kbps audio
	DefaultBurstSize = 128 * 1024

	// MaxListenerLag before forcing skip-to-live (512KB = ~13 seconds of 320kbps audio)
	MaxListenerLag = 512 * 1024

	// SyncPointInterval - create sync points every N bytes
	SyncPointInterval = 16 * 1024
)

// =============================================================================
// BROADCASTER - Manages pushing data to all listeners
// =============================================================================

// Broadcaster manages a buffer and its listeners
type Broadcaster struct {
	buffer *Buffer
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewBroadcaster creates a new broadcaster with the specified buffer sizes
func NewBroadcaster(bufSize, burstSize int) *Broadcaster {
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	if burstSize <= 0 {
		burstSize = DefaultBurstSize
	}

	return &Broadcaster{
		buffer: NewBuffer(bufSize, burstSize),
		done:   make(chan struct{}),
	}
}

// Write writes data to the buffer and notifies all listeners
func (bc *Broadcaster) Write(p []byte) (int, error) {
	return bc.buffer.Write(p)
}

// Buffer returns the underlying buffer
func (bc *Broadcaster) Buffer() *Buffer {
	return bc.buffer
}

// Done returns the done channel
func (bc *Broadcaster) Done() <-chan struct{} {
	return bc.done
}

// Close shuts down the broadcaster
func (bc *Broadcaster) Close() {
	select {
	case <-bc.done:
		// Already closed
	default:
		close(bc.done)
	}
	bc.wg.Wait()
}

// IsClosed returns true if the broadcaster is closed
func (bc *Broadcaster) IsClosed() bool {
	select {
	case <-bc.done:
		return true
	default:
		return false
	}
}

// =============================================================================
// FRAME DETECTION - For clean audio boundaries
// =============================================================================

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

	// Validate indices
	if bitrateIdx == 0 || bitrateIdx == 15 || samplingIdx == 3 {
		return 0
	}

	// Bitrate and sampling rate tables
	var bitrate, samplingRate int

	switch version {
	case 3: // MPEG1
		switch layer {
		case 1: // Layer 3
			bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
			bitrate = bitrates[bitrateIdx] * 1000
		case 2: // Layer 2
			bitrates := []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0}
			bitrate = bitrates[bitrateIdx] * 1000
		case 3: // Layer 1
			bitrates := []int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0}
			bitrate = bitrates[bitrateIdx] * 1000
		default:
			return 0
		}
		samplingRates := []int{44100, 48000, 32000, 0}
		samplingRate = samplingRates[samplingIdx]
	case 2: // MPEG2
		switch layer {
		case 1: // Layer 3
			bitrates := []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
			bitrate = bitrates[bitrateIdx] * 1000
		default:
			return 0
		}
		samplingRates := []int{22050, 24000, 16000, 0}
		samplingRate = samplingRates[samplingIdx]
	case 0: // MPEG2.5
		switch layer {
		case 1: // Layer 3
			bitrates := []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
			bitrate = bitrates[bitrateIdx] * 1000
		default:
			return 0
		}
		samplingRates := []int{11025, 12000, 8000, 0}
		samplingRate = samplingRates[samplingIdx]
	default:
		return 0
	}

	if bitrate == 0 || samplingRate == 0 {
		return 0
	}

	// Calculate frame size based on layer
	var frameSize int
	switch layer {
	case 3: // Layer 1
		frameSize = (12*bitrate/samplingRate + int(padding)) * 4
	case 2, 1: // Layer 2 or 3
		if version == 3 { // MPEG1
			frameSize = 144*bitrate/samplingRate + int(padding)
		} else { // MPEG2 or MPEG2.5
			frameSize = 72*bitrate/samplingRate + int(padding)
		}
	}

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

// ValidateMP3Frame checks if data starts with a valid MP3 frame
func ValidateMP3Frame(data []byte) bool {
	return DetectMP3Frame(data) > 0
}

// =============================================================================
// LISTENER POSITION TRACKER - For tracking individual listener positions
// =============================================================================

// ListenerPosition tracks a listener's position in the buffer
type ListenerPosition struct {
	ID        string
	Position  atomic.Int64
	LastRead  atomic.Int64 // Unix nano timestamp
	BytesSent atomic.Int64
	SkipCount atomic.Int32
	Connected time.Time
	buffer    *Buffer
	done      chan struct{}
	closeOnce sync.Once
}

// NewListenerPosition creates a new listener position tracker
func NewListenerPosition(id string, buf *Buffer) *ListenerPosition {
	lp := &ListenerPosition{
		ID:        id,
		buffer:    buf,
		Connected: time.Now(),
		done:      make(chan struct{}),
	}

	// Start at a sync point for clean audio
	lp.Position.Store(buf.GetSyncPoint())

	return lp
}

// Read reads available data for this listener into the provided buffer
// Returns bytes read and whether data was available
func (lp *ListenerPosition) Read(buf []byte) (int, bool) {
	pos := lp.Position.Load()
	writePos := lp.buffer.WritePos()

	// Check if we're too far behind
	lag := writePos - pos
	if lag > MaxListenerLag {
		// Skip to a sync point
		newPos := lp.buffer.GetSyncPoint()
		lp.Position.Store(newPos)
		pos = newPos
		lp.SkipCount.Add(1)
	}

	// Read data
	n, newPos := lp.buffer.ReadFromInto(pos, buf)
	if n > 0 {
		lp.Position.Store(newPos)
		lp.LastRead.Store(time.Now().UnixNano())
		lp.BytesSent.Add(int64(n))
		return n, true
	}

	return 0, false
}

// GetLag returns how far behind live edge this listener is
func (lp *ListenerPosition) GetLag() int64 {
	return lp.buffer.WritePos() - lp.Position.Load()
}

// IsHealthy returns true if listener is keeping up
func (lp *ListenerPosition) IsHealthy() bool {
	return lp.GetLag() < MaxListenerLag/2
}

// Close closes the listener position tracker
func (lp *ListenerPosition) Close() {
	lp.closeOnce.Do(func() {
		close(lp.done)
	})
}

// Done returns the done channel
func (lp *ListenerPosition) Done() <-chan struct{} {
	return lp.done
}

// =============================================================================
// JITTER BUFFER - Smooths out network variations (optional use)
// =============================================================================

// JitterBuffer smooths out timing variations in data arrival
type JitterBuffer struct {
	target   time.Duration
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

// Flush returns all buffered data immediately
func (jb *JitterBuffer) Flush() [][]byte {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	var result [][]byte
	for jb.head != jb.tail {
		if jb.buffer[jb.head] != nil {
			result = append(result, jb.buffer[jb.head])
			jb.buffer[jb.head] = nil
		}
		jb.head = (jb.head + 1) % jb.size
	}

	return result
}

// Reset clears the jitter buffer
func (jb *JitterBuffer) Reset() {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	for i := range jb.buffer {
		jb.buffer[i] = nil
	}
	jb.head = 0
	jb.tail = 0
}
