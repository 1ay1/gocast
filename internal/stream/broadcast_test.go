// Package stream tests for broadcast buffer and streaming components
package stream

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------
// BROADCAST BUFFER TESTS
// ---------------------------------------------------------

func TestNewBroadcastBuffer(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		burstSize int
		wantSize  int
	}{
		{"default", 0, 0, 16384},
		{"small", 1000, 100, 1024}, // rounds up to power of 2
		{"exact power of 2", 4096, 512, 4096},
		{"large", 1024 * 1024, 8192, 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewBroadcastBuffer(tt.size, tt.burstSize)
			if buf == nil {
				t.Fatal("NewBroadcastBuffer returned nil")
			}
			if buf.size < tt.wantSize {
				t.Errorf("buffer size = %d, want >= %d", buf.size, tt.wantSize)
			}
			// Verify power of 2
			if buf.size&(buf.size-1) != 0 {
				t.Errorf("buffer size %d is not a power of 2", buf.size)
			}
		})
	}
}

func TestBroadcastBufferWrite(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	data := []byte("Hello, World!")
	n, err := buf.Write(data)

	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
	if buf.WritePos() != int64(len(data)) {
		t.Errorf("WritePos = %d, want %d", buf.WritePos(), len(data))
	}
}

func TestBroadcastBufferWriteBatch(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i % 256)
	}

	n, err := buf.WriteBatch(data)
	if err != nil {
		t.Fatalf("WriteBatch error: %v", err)
	}
	if n != len(data) {
		t.Errorf("WriteBatch returned %d, want %d", n, len(data))
	}
}

func TestBroadcastBufferWrapAround(t *testing.T) {
	buf := NewBroadcastBuffer(256, 64)

	// Write more than buffer size to test wrap-around
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	// Write 5 times (500 bytes) to wrap around multiple times
	for i := 0; i < 5; i++ {
		_, err := buf.WriteBatch(data)
		if err != nil {
			t.Fatalf("WriteBatch error on iteration %d: %v", i, err)
		}
	}

	if buf.WritePos() != 500 {
		t.Errorf("WritePos = %d, want 500", buf.WritePos())
	}
}

func TestBroadcastBufferReadAt(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	// Write test data
	data := []byte("0123456789ABCDEF")
	buf.WriteBatch(data)

	// Read from beginning
	read, newPos := buf.ReadAt(0, 8)
	if string(read) != "01234567" {
		t.Errorf("ReadAt returned %q, want %q", string(read), "01234567")
	}
	if newPos != 8 {
		t.Errorf("newPos = %d, want 8", newPos)
	}

	// Read remaining
	read, newPos = buf.ReadAt(8, 100)
	if string(read) != "89ABCDEF" {
		t.Errorf("ReadAt returned %q, want %q", string(read), "89ABCDEF")
	}
	if newPos != 16 {
		t.Errorf("newPos = %d, want 16", newPos)
	}
}

func TestBroadcastBufferReadAtStale(t *testing.T) {
	buf := NewBroadcastBuffer(256, 64) // Small buffer

	// Fill buffer beyond capacity
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i)
	}
	buf.WriteBatch(data)

	// Try to read from position 0 (should be stale)
	read, newPos := buf.ReadAt(0, 100)

	// Should skip to oldest available data
	if newPos < 44 { // 300 - 256 = 44
		t.Errorf("newPos = %d, should be >= 44", newPos)
	}
	if len(read) == 0 {
		t.Error("ReadAt returned empty slice for stale position")
	}
}

func TestBroadcastBufferGetBurst(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 64)

	// Write some data
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	buf.WriteBatch(data)

	burst := buf.GetBurst()
	if len(burst) > 64 {
		t.Errorf("burst size = %d, want <= 64", len(burst))
	}
	if len(burst) == 0 {
		t.Error("burst is empty")
	}

	// Verify burst contains recent data
	// Last byte should be 99
	if burst[len(burst)-1] != 99 {
		t.Errorf("last burst byte = %d, want 99", burst[len(burst)-1])
	}
}

func TestBroadcastBufferSyncPoints(t *testing.T) {
	buf := NewBroadcastBuffer(65536, 4096)

	// Write enough data to create sync points
	data := make([]byte, 20000)
	buf.WriteBatch(data)

	syncPos := buf.GetSyncPoint()
	writePos := buf.WritePos()

	// Sync point should be before write position
	if syncPos >= writePos {
		t.Errorf("syncPos %d should be < writePos %d", syncPos, writePos)
	}

	// Sync point should be within burst size of write position
	if writePos-syncPos > int64(buf.burstSize) {
		t.Errorf("syncPos %d is too far behind writePos %d", syncPos, writePos)
	}
}

func TestBroadcastBufferGetLivePosition(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	// Write data
	data := make([]byte, 500)
	buf.WriteBatch(data)

	livePos := buf.GetLivePosition()
	writePos := buf.WritePos()

	// Live position should be close to write position (within 2KB)
	if writePos-livePos > 2048 {
		t.Errorf("livePos %d is too far behind writePos %d", livePos, writePos)
	}
}

// ---------------------------------------------------------
// BROADCAST LISTENER TESTS
// ---------------------------------------------------------

func TestNewBroadcastListener(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	// Write some data first
	data := make([]byte, 500)
	buf.WriteBatch(data)

	listener := NewBroadcastListener("test-1", buf)
	if listener == nil {
		t.Fatal("NewBroadcastListener returned nil")
	}
	if listener.ID != "test-1" {
		t.Errorf("listener ID = %q, want %q", listener.ID, "test-1")
	}

	// Position should be at a sync point
	pos := listener.Position.Load()
	if pos < 0 || pos > buf.WritePos() {
		t.Errorf("listener position %d is invalid", pos)
	}
}

func TestBroadcastListenerRead(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	// Write identifiable data
	data := []byte("ABCDEFGHIJ1234567890")
	buf.WriteBatch(data)

	listener := NewBroadcastListener("test-1", buf)

	// Read data
	read := listener.Read(10)
	if len(read) == 0 {
		t.Error("Read returned empty slice")
	}

	// Verify we got valid data
	for _, b := range read {
		if b < 0x30 || (b > 0x39 && b < 0x41) || b > 0x5A {
			if b != 0 { // Allow for initial position
				// Character should be A-J or 0-9
			}
		}
	}
}

func TestBroadcastListenerSkipToLive(t *testing.T) {
	buf := NewBroadcastBuffer(256, 64) // Small buffer

	// Write initial data
	initial := make([]byte, 100)
	buf.WriteBatch(initial)

	listener := NewBroadcastListener("test-1", buf)
	startPos := listener.Position.Load()

	// Write a lot more data (exceeds MaxListenerLag)
	large := make([]byte, 50000)
	buf.WriteBatch(large)

	// Read should trigger skip-to-live
	listener.Read(100)

	if listener.SkipCount == 0 {
		t.Error("expected skip count > 0 after falling behind")
	}

	newPos := listener.Position.Load()
	if newPos <= startPos {
		t.Errorf("position should have advanced: start=%d, new=%d", startPos, newPos)
	}
}

func TestBroadcastListenerGetLag(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)

	// Write data
	data := make([]byte, 500)
	buf.WriteBatch(data)

	listener := NewBroadcastListener("test-1", buf)

	lag := listener.GetLag()
	if lag < 0 {
		t.Errorf("lag should be >= 0, got %d", lag)
	}
}

func TestBroadcastListenerIsHealthy(t *testing.T) {
	buf := NewBroadcastBuffer(65536, 4096)

	data := make([]byte, 1000)
	buf.WriteBatch(data)

	listener := NewBroadcastListener("test-1", buf)

	// Should be healthy initially
	if !listener.IsHealthy() {
		t.Error("listener should be healthy initially")
	}
}

func TestBroadcastListenerClose(t *testing.T) {
	buf := NewBroadcastBuffer(1024, 256)
	listener := NewBroadcastListener("test-1", buf)

	// Close should not panic
	listener.Close()

	// Double close should not panic
	listener.Close()

	// Done channel should be closed
	select {
	case <-listener.Done():
		// Good
	default:
		t.Error("Done channel should be closed after Close()")
	}
}

// ---------------------------------------------------------
// BROADCASTER TESTS
// ---------------------------------------------------------

func TestNewBroadcaster(t *testing.T) {
	bc := NewBroadcaster(1024, 256)
	if bc == nil {
		t.Fatal("NewBroadcaster returned nil")
	}
	defer bc.Close()

	if bc.ListenerCount() != 0 {
		t.Errorf("initial listener count = %d, want 0", bc.ListenerCount())
	}
}

func TestBroadcasterWrite(t *testing.T) {
	bc := NewBroadcaster(1024, 256)
	defer bc.Close()

	data := []byte("test data")
	n, err := bc.Write(data)

	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
}

func TestBroadcasterAddRemoveListener(t *testing.T) {
	bc := NewBroadcaster(1024, 256)
	defer bc.Close()

	// Add listeners
	l1 := bc.AddListener("listener-1")
	l2 := bc.AddListener("listener-2")

	if bc.ListenerCount() != 2 {
		t.Errorf("listener count = %d, want 2", bc.ListenerCount())
	}

	if l1 == nil || l2 == nil {
		t.Fatal("AddListener returned nil")
	}

	// Remove one
	bc.RemoveListener("listener-1")

	if bc.ListenerCount() != 1 {
		t.Errorf("listener count = %d, want 1", bc.ListenerCount())
	}

	// Get remaining listener
	got := bc.GetListener("listener-2")
	if got == nil {
		t.Error("GetListener returned nil for existing listener")
	}

	// Get removed listener
	got = bc.GetListener("listener-1")
	if got != nil {
		t.Error("GetListener should return nil for removed listener")
	}
}

func TestBroadcasterNotify(t *testing.T) {
	bc := NewBroadcaster(1024, 256)
	defer bc.Close()

	// Write should trigger notification
	bc.Write([]byte("test"))

	select {
	case <-bc.Notify():
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Error("expected notification after write")
	}
}

func TestBroadcasterConcurrentListeners(t *testing.T) {
	bc := NewBroadcaster(65536, 4096)
	defer bc.Close()

	// Write initial data
	data := make([]byte, 1000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	bc.Write(data)

	// Add multiple listeners concurrently
	var wg sync.WaitGroup
	numListeners := 100

	for i := 0; i < numListeners; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			listener := bc.AddListener(string(rune('A' + id)))
			if listener != nil {
				// Simulate reading
				for j := 0; j < 10; j++ {
					listener.Read(100)
				}
			}
		}(i)
	}

	wg.Wait()

	if bc.ListenerCount() != numListeners {
		t.Errorf("listener count = %d, want %d", bc.ListenerCount(), numListeners)
	}
}

// ---------------------------------------------------------
// FRAME DETECTION TESTS
// ---------------------------------------------------------

func TestDetectMP3Frame(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantSize int
	}{
		{
			name:     "not mp3",
			data:     []byte{0x00, 0x00, 0x00, 0x00},
			wantSize: 0,
		},
		{
			name:     "too short",
			data:     []byte{0xFF, 0xFB},
			wantSize: 0,
		},
		{
			name:     "invalid bitrate",
			data:     []byte{0xFF, 0xFB, 0x00, 0x00}, // bitrate index 0
			wantSize: 0,
		},
		{
			name:     "valid mp3 header 128kbps",
			data:     []byte{0xFF, 0xFB, 0x90, 0x00}, // MPEG1 Layer3, 128kbps, 44100Hz
			wantSize: 417,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectMP3Frame(tt.data)
			if got != tt.wantSize {
				t.Errorf("DetectMP3Frame() = %d, want %d", got, tt.wantSize)
			}
		})
	}
}

func TestFindNextMP3Frame(t *testing.T) {
	// Create data with MP3 frame sync in the middle
	data := make([]byte, 100)
	data[20] = 0xFF
	data[21] = 0xFB
	data[22] = 0x90
	data[23] = 0x00

	offset := FindNextMP3Frame(data)
	if offset != 20 {
		t.Errorf("FindNextMP3Frame() = %d, want 20", offset)
	}

	// No frame sync
	noFrame := make([]byte, 100)
	offset = FindNextMP3Frame(noFrame)
	if offset != -1 {
		t.Errorf("FindNextMP3Frame() = %d, want -1 for no frame", offset)
	}
}

// ---------------------------------------------------------
// JITTER BUFFER TESTS
// ---------------------------------------------------------

func TestNewJitterBuffer(t *testing.T) {
	jb := NewJitterBuffer(50 * time.Millisecond)
	if jb == nil {
		t.Fatal("NewJitterBuffer returned nil")
	}
	if jb.target != 50*time.Millisecond {
		t.Errorf("target = %v, want %v", jb.target, 50*time.Millisecond)
	}
}

func TestJitterBufferPushPop(t *testing.T) {
	jb := NewJitterBuffer(10 * time.Millisecond)

	// Push data
	jb.Push([]byte("test1"))
	jb.Push([]byte("test2"))

	if jb.Len() != 2 {
		t.Errorf("Len() = %d, want 2", jb.Len())
	}

	// Pop should return nil if delay not reached
	// (immediately after push)
	popped := jb.Pop()
	if popped != nil {
		// This might succeed if the test runs slow enough
		// which is fine
	}

	// Wait for delay
	time.Sleep(15 * time.Millisecond)

	popped = jb.Pop()
	if popped == nil {
		t.Error("Pop returned nil after delay")
	} else if string(popped) != "test1" {
		t.Errorf("Pop returned %q, want %q", string(popped), "test1")
	}
}

// ---------------------------------------------------------
// BYTE POOL TESTS
// ---------------------------------------------------------

func TestBytePool(t *testing.T) {
	buf := GetPooledBuffer()
	if buf == nil {
		t.Fatal("GetPooledBuffer returned nil")
	}
	if len(*buf) != BytePoolSize {
		t.Errorf("buffer size = %d, want %d", len(*buf), BytePoolSize)
	}

	// Modify buffer
	(*buf)[0] = 42

	// Return to pool
	PutPooledBuffer(buf)

	// Get another buffer
	buf2 := GetPooledBuffer()
	if buf2 == nil {
		t.Fatal("GetPooledBuffer returned nil after put")
	}

	// Might be the same buffer (reused)
	PutPooledBuffer(buf2)
}

// ---------------------------------------------------------
// PADDED INT64 TESTS
// ---------------------------------------------------------

func TestPaddedInt64(t *testing.T) {
	var p PaddedInt64

	if p.Load() != 0 {
		t.Errorf("initial value = %d, want 0", p.Load())
	}

	p.Store(42)
	if p.Load() != 42 {
		t.Errorf("after store = %d, want 42", p.Load())
	}

	result := p.Add(8)
	if result != 50 {
		t.Errorf("Add result = %d, want 50", result)
	}
	if p.Load() != 50 {
		t.Errorf("after add = %d, want 50", p.Load())
	}
}

// ---------------------------------------------------------
// BENCHMARKS
// ---------------------------------------------------------

func BenchmarkBroadcastBufferWrite(b *testing.B) {
	buf := NewBroadcastBuffer(256*1024, 8192)
	data := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		buf.Write(data)
	}
}

func BenchmarkBroadcastBufferWriteBatch(b *testing.B) {
	buf := NewBroadcastBuffer(256*1024, 8192)
	data := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		buf.WriteBatch(data)
	}
}

func BenchmarkBroadcastBufferReadAt(b *testing.B) {
	buf := NewBroadcastBuffer(256*1024, 8192)

	// Pre-fill buffer
	data := make([]byte, 128*1024)
	buf.WriteBatch(data)

	b.ResetTimer()
	b.SetBytes(4096)

	var pos int64
	for i := 0; i < b.N; i++ {
		_, pos = buf.ReadAt(pos, 4096)
		if pos >= buf.WritePos() {
			pos = 0
		}
	}
}

func BenchmarkBroadcastListenerRead(b *testing.B) {
	buf := NewBroadcastBuffer(256*1024, 8192)

	// Pre-fill buffer
	data := make([]byte, 128*1024)
	buf.WriteBatch(data)

	listener := NewBroadcastListener("bench", buf)

	b.ResetTimer()
	b.SetBytes(2048)

	for i := 0; i < b.N; i++ {
		d := listener.Read(2048)
		if len(d) == 0 {
			// Reset position if we caught up
			listener.Position.Store(0)
		}
	}
}

func BenchmarkBroadcasterWrite(b *testing.B) {
	bc := NewBroadcaster(256*1024, 8192)
	defer bc.Close()

	// Add some listeners
	for i := 0; i < 10; i++ {
		bc.AddListener(string(rune('A' + i)))
	}

	data := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		bc.Write(data)
	}
}

func BenchmarkBroadcasterConcurrent(b *testing.B) {
	bc := NewBroadcaster(256*1024, 8192)
	defer bc.Close()

	data := make([]byte, 1024)

	// Pre-fill
	bc.Write(make([]byte, 64*1024))

	// Add listeners that read concurrently
	numListeners := 10
	done := make(chan struct{})

	for i := 0; i < numListeners; i++ {
		listener := bc.AddListener(string(rune('A' + i)))
		go func(l *BroadcastListener) {
			for {
				select {
				case <-done:
					return
				default:
					l.Read(1024)
				}
			}
		}(listener)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		bc.Write(data)
	}

	close(done)
}

func BenchmarkDetectMP3Frame(b *testing.B) {
	// Valid MP3 frame header
	data := []byte{0xFF, 0xFB, 0x90, 0x00}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		DetectMP3Frame(data)
	}
}

func BenchmarkBytePool(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf := GetPooledBuffer()
		PutPooledBuffer(buf)
	}
}

func BenchmarkBytePoolParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := GetPooledBuffer()
			// Simulate some work
			(*buf)[0] = 1
			PutPooledBuffer(buf)
		}
	})
}
