// Package stream handles audio stream management and distribution
// High-performance, lock-free ring buffer optimized for live streaming
package stream

import (
	"sync"
	"sync/atomic"
	"time"
)

// Buffer is a high-performance ring buffer for streaming audio data
// Design goals:
// - Lock-free reads for maximum throughput
// - Atomic write position updates
// - Broadcast notifications for waiting listeners
// - Graceful handling of buffer wraparound
type Buffer struct {
	// Core buffer data - never reallocated after creation
	data []byte
	size int64 // Use int64 to avoid conversions
	mask int64 // size - 1 for fast modulo (requires power of 2 size)

	// Write position - updated atomically
	writePos atomic.Int64

	// Write lock - only one writer at a time
	writeMu sync.Mutex

	// Notification for waiting readers
	notify     chan struct{}
	notifyOnce sync.Once

	// Configuration
	burstSize int

	// Stats
	bytesTotal atomic.Int64
	created    time.Time
}

// NewBuffer creates a new stream buffer
// Size is rounded up to nearest power of 2 for fast modulo operations
func NewBuffer(size int, burstSize int) *Buffer {
	// Default sizes optimized for 320kbps streaming
	if size <= 0 {
		size = 262144 // 256KB = ~6.5 seconds at 320kbps
	}
	if burstSize <= 0 {
		burstSize = 8192 // 8KB = ~200ms at 320kbps
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
		notify:    make(chan struct{}, 1),
		created:   time.Now(),
	}

	return b
}

// Write writes data to the buffer (thread-safe, single writer)
func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.writeMu.Lock()

	writePos := b.writePos.Load()
	n := len(p)

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

	// Non-blocking notification to waiting readers
	select {
	case b.notify <- struct{}{}:
	default:
		// Channel full, readers will poll
	}

	return n, nil
}

// WritePos returns the current write position (lock-free)
func (b *Buffer) WritePos() int64 {
	return b.writePos.Load()
}

// ReadFrom reads data from buffer starting at pos (lock-free for readers)
// Returns data slice and new position
// If pos is behind (data overwritten), jumps to oldest available data
// If pos is ahead of write position, returns nil
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
		// Data was overwritten, jump to oldest available
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

	// Read from ring buffer
	startIdx := pos & b.mask

	if startIdx+int64(available) <= b.size {
		// Fast path: single contiguous read
		copy(result, b.data[startIdx:startIdx+int64(available)])
	} else {
		// Wrap around: two copies
		firstPart := int(b.size - startIdx)
		copy(result[:firstPart], b.data[startIdx:])
		copy(result[firstPart:], b.data[:available-firstPart])
	}

	return result, pos + int64(available)
}

// ReadFromInto reads data into provided buffer (zero-allocation read)
// Returns number of bytes read and new position
func (b *Buffer) ReadFromInto(pos int64, buf []byte) (int, int64) {
	if len(buf) == 0 {
		return 0, pos
	}

	writePos := b.writePos.Load()

	if pos >= writePos {
		return 0, pos
	}

	oldestPos := writePos - b.size
	if oldestPos < 0 {
		oldestPos = 0
	}
	if pos < oldestPos {
		pos = oldestPos
	}

	available := int(writePos - pos)
	if available > len(buf) {
		available = len(buf)
	}
	if available <= 0 {
		return 0, pos
	}

	startIdx := pos & b.mask

	if startIdx+int64(available) <= b.size {
		copy(buf[:available], b.data[startIdx:])
	} else {
		firstPart := int(b.size - startIdx)
		copy(buf[:firstPart], b.data[startIdx:])
		copy(buf[firstPart:available], b.data[:available-firstPart])
	}

	return available, pos + int64(available)
}

// GetBurst returns burst data for new listener
func (b *Buffer) GetBurst() []byte {
	writePos := b.writePos.Load()

	if writePos == 0 {
		return nil
	}

	burstBytes := int64(b.burstSize)
	if writePos < burstBytes {
		burstBytes = writePos
	}

	startPos := writePos - burstBytes
	result := make([]byte, burstBytes)

	startIdx := startPos & b.mask

	if startIdx+burstBytes <= b.size {
		copy(result, b.data[startIdx:])
	} else {
		firstPart := int(b.size - startIdx)
		copy(result[:firstPart], b.data[startIdx:])
		copy(result[firstPart:], b.data[:int(burstBytes)-firstPart])
	}

	return result
}

// GetLivePosition returns position for live-edge listening
func (b *Buffer) GetLivePosition() int64 {
	writePos := b.writePos.Load()

	// Small lag to prevent underrun (1KB = ~25ms at 320kbps)
	lag := int64(1024)
	pos := writePos - lag
	if pos < 0 {
		pos = 0
	}
	return pos
}

// WaitForData waits for new data with timeout
// Returns true if data is available, false on timeout
func (b *Buffer) WaitForData(pos int64, timeout time.Duration) bool {
	// Check if data already available
	if b.writePos.Load() > pos {
		return true
	}

	// Wait for notification or timeout
	select {
	case <-b.notify:
		return b.writePos.Load() > pos
	case <-time.After(timeout):
		return b.writePos.Load() > pos
	}
}

// NotifyChan returns the notification channel
func (b *Buffer) NotifyChan() <-chan struct{} {
	return b.notify
}

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
	b.created = time.Now()
	b.writeMu.Unlock()

	// Drain notification channel
	select {
	case <-b.notify:
	default:
	}
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
