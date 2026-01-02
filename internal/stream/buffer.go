// Package stream handles audio stream management and distribution
package stream

import (
	"sync"
	"sync/atomic"
	"time"
)

// Buffer is a ring buffer for streaming audio data to multiple listeners
// Optimized for low latency streaming with synchronized listener positions
type Buffer struct {
	data       []byte
	size       int
	writePos   int64
	mu         sync.RWMutex
	burstSize  int
	created    time.Time
	bytesTotal int64

	// Listener synchronization
	listeners   map[*int64]struct{} // Track listener positions
	listenersMu sync.RWMutex
	notifyChan  chan struct{}
	notifyMu    sync.Mutex
}

// NewBuffer creates a new stream buffer with the specified size
// Ultra-low latency defaults:
// - Tiny burst (2KB) for instant playback start
// - Smaller buffer (128KB) for minimum latency
func NewBuffer(size int, burstSize int) *Buffer {
	if size <= 0 {
		size = 131072 // 128KB default - ~4 seconds at 256kbps
	}
	if burstSize <= 0 {
		burstSize = 2048 // 2KB burst - ~50ms at 320kbps for instant start
	}
	if burstSize > size {
		burstSize = size
	}

	return &Buffer{
		data:       make([]byte, size),
		size:       size,
		writePos:   0,
		burstSize:  burstSize,
		created:    time.Now(),
		listeners:  make(map[*int64]struct{}),
		notifyChan: make(chan struct{}, 1),
	}
}

// Write writes data to the buffer
// Optimized: batch copy instead of byte-by-byte
func (b *Buffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	n = len(p)

	// Optimized: copy in chunks instead of byte-by-byte
	startPos := b.writePos % int64(b.size)

	if int(startPos)+n <= b.size {
		// Fast path: single copy
		copy(b.data[startPos:], p)
	} else {
		// Wrap around: two copies
		firstPart := b.size - int(startPos)
		copy(b.data[startPos:], p[:firstPart])
		copy(b.data[0:], p[firstPart:])
	}

	b.writePos += int64(n)
	atomic.AddInt64(&b.bytesTotal, int64(n))
	b.mu.Unlock()

	// Non-blocking notify
	b.notifyMu.Lock()
	select {
	case b.notifyChan <- struct{}{}:
	default:
	}
	b.notifyMu.Unlock()

	return n, nil
}

// WritePos returns the current write position
func (b *Buffer) WritePos() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.writePos
}

// GetBurst returns minimal burst data for instant listener start
// Ultra-low latency: only returns tiny amount to prime player buffer
func (b *Buffer) GetBurst() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.writePos == 0 {
		return nil
	}

	burstBytes := b.burstSize
	if b.writePos < int64(burstBytes) {
		burstBytes = int(b.writePos)
	}

	result := make([]byte, burstBytes)
	startPos := b.writePos - int64(burstBytes)

	// Optimized: batch copy
	readStart := int(startPos % int64(b.size))
	if readStart+burstBytes <= b.size {
		copy(result, b.data[readStart:readStart+burstBytes])
	} else {
		firstPart := b.size - readStart
		copy(result[:firstPart], b.data[readStart:])
		copy(result[firstPart:], b.data[:burstBytes-firstPart])
	}

	return result
}

// ReadFrom reads data from the buffer starting at the given position
// Returns the data and the new position
// Optimized: smaller max read size for lower latency
func (b *Buffer) ReadFrom(pos int64, maxBytes int) ([]byte, int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Check if position is valid
	if pos >= b.writePos {
		return nil, pos
	}

	// Check if position is too old (data overwritten)
	oldestPos := b.writePos - int64(b.size)
	if oldestPos < 0 {
		oldestPos = 0
	}
	if pos < oldestPos {
		pos = oldestPos
	}

	// Calculate how much data is available
	available := int(b.writePos - pos)
	if available > maxBytes {
		available = maxBytes
	}

	if available <= 0 {
		return nil, pos
	}

	result := make([]byte, available)
	readStart := int(pos % int64(b.size))

	// Optimized: batch copy
	if readStart+available <= b.size {
		copy(result, b.data[readStart:readStart+available])
	} else {
		firstPart := b.size - readStart
		copy(result[:firstPart], b.data[readStart:])
		copy(result[firstPart:], b.data[:available-firstPart])
	}

	return result, pos + int64(available)
}

// ReadFromSync reads from a synchronized position for listener sync
// All listeners calling this will get data from approximately the same position
func (b *Buffer) ReadFromSync(maxBytes int) ([]byte, int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.writePos == 0 {
		return nil, 0
	}

	// Read from a position that's slightly behind write position
	// This keeps all listeners synced to the same point
	lagBytes := int64(b.burstSize / 2) // Half burst behind for sync
	pos := b.writePos - lagBytes
	if pos < 0 {
		pos = 0
	}

	available := int(b.writePos - pos)
	if available > maxBytes {
		available = maxBytes
	}

	if available <= 0 {
		return nil, b.writePos
	}

	result := make([]byte, available)
	readStart := int(pos % int64(b.size))

	if readStart+available <= b.size {
		copy(result, b.data[readStart:readStart+available])
	} else {
		firstPart := b.size - readStart
		copy(result[:firstPart], b.data[readStart:])
		copy(result[firstPart:], b.data[:available-firstPart])
	}

	return result, pos + int64(available)
}

// GetLivePosition returns position at the live edge for instant playback
// Ultra-low latency: start as close to live as possible
func (b *Buffer) GetLivePosition() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Start at live edge minus tiny buffer (512 bytes = ~12ms at 320kbps)
	// This is the minimum needed to avoid buffer underrun
	lagBytes := int64(512)
	pos := b.writePos - lagBytes
	if pos < 0 {
		pos = 0
	}
	return pos
}

// NotifyChan returns the notification channel for new data
func (b *Buffer) NotifyChan() <-chan struct{} {
	return b.notifyChan
}

// BytesTotal returns the total number of bytes written
func (b *Buffer) BytesTotal() int64 {
	return atomic.LoadInt64(&b.bytesTotal)
}

// Size returns the buffer size
func (b *Buffer) Size() int {
	return b.size
}

// BurstSize returns the burst size
func (b *Buffer) BurstSize() int {
	return b.burstSize
}

// Created returns when the buffer was created
func (b *Buffer) Created() time.Time {
	return b.created
}

// Reset resets the buffer
func (b *Buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.writePos = 0
	b.bytesTotal = 0
	b.created = time.Now()

	// Reset notify channel
	b.notifyMu.Lock()
	select {
	case <-b.notifyChan:
	default:
	}
	b.notifyMu.Unlock()
}

// SetBurstSize allows runtime adjustment of burst size
func (b *Buffer) SetBurstSize(size int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if size > 0 && size <= b.size {
		b.burstSize = size
	}
}
