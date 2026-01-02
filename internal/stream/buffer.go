// Package stream handles audio stream management and distribution
package stream

import (
	"sync"
	"sync/atomic"
	"time"
)

// Buffer is a ring buffer for streaming audio data to multiple listeners
type Buffer struct {
	data       []byte
	size       int
	writePos   int64
	mu         sync.RWMutex
	burstSize  int
	created    time.Time
	bytesTotal int64
}

// NewBuffer creates a new stream buffer with the specified size
func NewBuffer(size int, burstSize int) *Buffer {
	if size <= 0 {
		size = 524288 // 512KB default
	}
	if burstSize <= 0 {
		burstSize = 65535 // 64KB default burst
	}
	if burstSize > size {
		burstSize = size
	}

	return &Buffer{
		data:      make([]byte, size),
		size:      size,
		writePos:  0,
		burstSize: burstSize,
		created:   time.Now(),
	}
}

// Write writes data to the buffer
func (b *Buffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	n = len(p)

	// Write data to the ring buffer
	for i := 0; i < n; i++ {
		pos := int(b.writePos % int64(b.size))
		b.data[pos] = p[i]
		b.writePos++
	}

	atomic.AddInt64(&b.bytesTotal, int64(n))

	return n, nil
}

// WritePos returns the current write position
func (b *Buffer) WritePos() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.writePos
}

// GetBurst returns the most recent burst_size bytes for new listeners
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

	for i := 0; i < burstBytes; i++ {
		pos := int((startPos + int64(i)) % int64(b.size))
		result[i] = b.data[pos]
	}

	return result
}

// ReadFrom reads data from the buffer starting at the given position
// Returns the data and the new position
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
	for i := 0; i < available; i++ {
		bufPos := int((pos + int64(i)) % int64(b.size))
		result[i] = b.data[bufPos]
	}

	return result, pos + int64(available)
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
}
