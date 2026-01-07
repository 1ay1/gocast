// Package stream tests for buffer and streaming components
package stream

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------
// BUFFER TESTS
// ---------------------------------------------------------

func TestNewBuffer(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		burstSize int
		wantMin   int
	}{
		{"default", 0, 0, 1024 * 1024}, // Default is 10MB, rounded to power of 2
		{"small", 1000, 100, 1024},     // rounds up to power of 2
		{"exact power of 2", 4096, 512, 4096},
		{"large", 1024 * 1024, 8192, 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewBuffer(tt.size, tt.burstSize)
			if buf == nil {
				t.Fatal("NewBuffer returned nil")
			}
			if buf.Size() < tt.wantMin {
				t.Errorf("buffer size = %d, want >= %d", buf.Size(), tt.wantMin)
			}
			// Verify power of 2
			size := buf.Size()
			if size&(size-1) != 0 {
				t.Errorf("buffer size %d is not a power of 2", size)
			}
		})
	}
}

func TestBufferWrite(t *testing.T) {
	buf := NewBuffer(1024, 256)

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

func TestBufferWrapAround(t *testing.T) {
	buf := NewBuffer(256, 64)

	// Write more than buffer size to test wrap-around
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	// Write 5 times (500 bytes) to wrap around multiple times
	for i := 0; i < 5; i++ {
		_, err := buf.Write(data)
		if err != nil {
			t.Fatalf("Write error on iteration %d: %v", i, err)
		}
	}

	if buf.WritePos() != 500 {
		t.Errorf("WritePos = %d, want 500", buf.WritePos())
	}
}

func TestBufferReadFromInto(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write test data
	data := []byte("0123456789ABCDEF")
	buf.Write(data)

	// Read from beginning
	readBuf := make([]byte, 8)
	n, newPos := buf.ReadFromInto(0, readBuf)
	if n != 8 {
		t.Errorf("Read returned %d bytes, want 8", n)
	}
	if string(readBuf[:n]) != "01234567" {
		t.Errorf("ReadFromInto returned %q, want %q", string(readBuf[:n]), "01234567")
	}
	if newPos != 8 {
		t.Errorf("newPos = %d, want 8", newPos)
	}

	// Read remaining
	readBuf = make([]byte, 100)
	n, newPos = buf.ReadFromInto(8, readBuf)
	if string(readBuf[:n]) != "89ABCDEF" {
		t.Errorf("ReadFromInto returned %q, want %q", string(readBuf[:n]), "89ABCDEF")
	}
	if newPos != 16 {
		t.Errorf("newPos = %d, want 16", newPos)
	}
}

func TestBufferReadFromStale(t *testing.T) {
	buf := NewBuffer(256, 64) // Small buffer

	// Fill buffer beyond capacity
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i)
	}
	buf.Write(data)

	// Try to read from position 0 (should be stale)
	readBuf := make([]byte, 100)
	n, newPos := buf.ReadFromInto(0, readBuf)

	// Should skip to oldest available data
	if newPos < 44 { // 300 - 256 = 44
		t.Errorf("newPos = %d, should be >= 44", newPos)
	}
	if n == 0 {
		t.Error("ReadFromInto returned 0 bytes for stale position")
	}
}

func TestBufferSafeReadFromInto(t *testing.T) {
	buf := NewBuffer(256, 64) // Small buffer

	// Fill buffer beyond capacity
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i)
	}
	buf.Write(data)

	// Read from stale position - should report skipped bytes
	readBuf := make([]byte, 100)
	n, newPos, skipped := buf.SafeReadFromInto(0, readBuf)

	if skipped == 0 {
		t.Error("SafeReadFromInto should report skipped bytes")
	}
	if n == 0 {
		t.Error("SafeReadFromInto returned 0 bytes")
	}
	if newPos <= 0 {
		t.Error("newPos should be > 0")
	}
}

func TestBufferGetBurst(t *testing.T) {
	buf := NewBuffer(1024, 64)

	// Write some data
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	buf.Write(data)

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

func TestBufferSyncPoints(t *testing.T) {
	buf := NewBuffer(65536, 4096)

	// Write enough data to create sync points
	data := make([]byte, 20000)
	buf.Write(data)

	syncPos := buf.GetSyncPoint()
	writePos := buf.WritePos()

	// Sync point should be before write position
	if syncPos >= writePos {
		t.Errorf("syncPos %d should be < writePos %d", syncPos, writePos)
	}

	// Sync point should be within burst size of write position
	if writePos-syncPos > int64(buf.BurstSize()) {
		t.Errorf("syncPos %d is too far behind writePos %d", syncPos, writePos)
	}
}

func TestBufferGetLivePosition(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write data
	data := make([]byte, 500)
	buf.Write(data)

	livePos := buf.GetLivePosition()
	writePos := buf.WritePos()

	// Live position should be close to write position (within 2KB)
	if writePos-livePos > 2048 {
		t.Errorf("livePos %d is too far behind writePos %d", livePos, writePos)
	}
}

func TestBufferWaitForData(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write initial data
	buf.Write([]byte("initial"))
	initialPos := buf.WritePos()

	// Start a writer goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		buf.Write([]byte("more data"))
	}()

	// Wait for data - should return true when data arrives
	result := buf.WaitForData(initialPos, 200*time.Millisecond)
	if !result {
		t.Error("WaitForData should return true when data is written")
	}
}

func TestBufferWaitForDataTimeout(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write initial data
	buf.Write([]byte("initial"))
	pos := buf.WritePos()

	// Wait for more data that never comes - should timeout
	start := time.Now()
	result := buf.WaitForData(pos, 50*time.Millisecond)
	elapsed := time.Since(start)

	if result {
		t.Error("WaitForData should return false on timeout")
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("WaitForData returned too quickly: %v", elapsed)
	}
}

func TestBufferWaitForDataContext(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write initial data
	buf.Write([]byte("initial"))
	initialPos := buf.WritePos()

	// Create context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Wait for data with context - should return false when cancelled
	result := buf.WaitForDataContext(ctx, initialPos)
	if result {
		t.Error("WaitForDataContext should return false when context is cancelled")
	}
}

func TestBufferWaitForDataContextWithData(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write initial data
	buf.Write([]byte("initial"))
	initialPos := buf.WritePos()

	ctx := context.Background()

	// Start a writer goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		buf.Write([]byte("more data"))
	}()

	// Wait for data with context - should return true when data arrives
	result := buf.WaitForDataContext(ctx, initialPos)
	if !result {
		t.Error("WaitForDataContext should return true when data is written")
	}
}

func TestBufferWaitForDataChan(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write initial data
	buf.Write([]byte("initial"))
	initialPos := buf.WritePos()

	done := make(chan struct{})

	// Close done channel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()

	// Wait for data - should return false when done is closed
	result := buf.WaitForDataChan(initialPos, done)
	if result {
		t.Error("WaitForDataChan should return false when done channel is closed")
	}
}

// ---------------------------------------------------------
// LISTENER POSITION TESTS
// ---------------------------------------------------------

func TestNewListenerPosition(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write some data first
	data := make([]byte, 500)
	buf.Write(data)

	listener := NewListenerPosition("test-1", buf)
	if listener == nil {
		t.Fatal("NewListenerPosition returned nil")
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

func TestListenerPositionRead(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write identifiable data
	data := []byte("ABCDEFGHIJ1234567890")
	buf.Write(data)

	listener := NewListenerPosition("test-1", buf)

	// Read data
	readBuf := make([]byte, 10)
	n, ok := listener.Read(readBuf)
	if !ok {
		t.Error("Read returned not ok")
	}
	if n == 0 {
		t.Error("Read returned 0 bytes")
	}
}

func TestListenerPositionSkipToLive(t *testing.T) {
	buf := NewBuffer(256, 64) // Small buffer

	// Write initial data
	initial := make([]byte, 100)
	buf.Write(initial)

	listener := NewListenerPosition("test-1", buf)
	startPos := listener.Position.Load()

	// Write a lot more data (exceeds MaxListenerLag)
	large := make([]byte, MaxListenerLag+10000)
	buf.Write(large)

	// Read should trigger skip-to-live
	readBuf := make([]byte, 100)
	listener.Read(readBuf)

	if listener.SkipCount.Load() == 0 {
		t.Error("expected skip count > 0 after falling behind")
	}

	newPos := listener.Position.Load()
	if newPos <= startPos {
		t.Errorf("position should have advanced: start=%d, new=%d", startPos, newPos)
	}
}

func TestListenerPositionGetLag(t *testing.T) {
	buf := NewBuffer(1024, 256)

	// Write data
	data := make([]byte, 500)
	buf.Write(data)

	listener := NewListenerPosition("test-1", buf)

	lag := listener.GetLag()
	if lag < 0 {
		t.Errorf("lag should be >= 0, got %d", lag)
	}
}

func TestListenerPositionIsHealthy(t *testing.T) {
	buf := NewBuffer(65536, 4096)

	data := make([]byte, 1000)
	buf.Write(data)

	listener := NewListenerPosition("test-1", buf)

	// Should be healthy initially
	if !listener.IsHealthy() {
		t.Error("listener should be healthy initially")
	}
}

func TestListenerPositionClose(t *testing.T) {
	buf := NewBuffer(1024, 256)
	listener := NewListenerPosition("test-1", buf)

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

	if bc.Buffer() == nil {
		t.Error("Buffer() returned nil")
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

func TestBroadcasterClose(t *testing.T) {
	bc := NewBroadcaster(1024, 256)

	// Close should not panic
	bc.Close()

	// Double close should not panic
	bc.Close()

	// IsClosed should return true
	if !bc.IsClosed() {
		t.Error("IsClosed should return true after Close()")
	}
}

func TestBroadcasterConcurrentWrites(t *testing.T) {
	bc := NewBroadcaster(65536, 4096)
	defer bc.Close()

	var wg sync.WaitGroup
	numWriters := 10
	writesPerWriter := 100

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i % 256)
	}

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				bc.Write(data)
			}
		}()
	}

	wg.Wait()

	expectedBytes := int64(numWriters * writesPerWriter * len(data))
	if bc.Buffer().BytesTotal() != expectedBytes {
		t.Errorf("total bytes = %d, want %d", bc.Buffer().BytesTotal(), expectedBytes)
	}
}

// ---------------------------------------------------------
// BUFFER POOL TESTS
// ---------------------------------------------------------

func TestSmallBufferPool(t *testing.T) {
	buf := GetSmallBuffer()
	if buf == nil {
		t.Fatal("GetSmallBuffer returned nil")
	}
	if len(*buf) != SmallBufferSize {
		t.Errorf("buffer size = %d, want %d", len(*buf), SmallBufferSize)
	}

	// Modify buffer
	(*buf)[0] = 42

	// Return to pool
	PutSmallBuffer(buf)

	// Get another buffer
	buf2 := GetSmallBuffer()
	if buf2 == nil {
		t.Fatal("GetSmallBuffer returned nil after put")
	}

	PutSmallBuffer(buf2)
}

func TestLargeBufferPool(t *testing.T) {
	buf := GetLargeBuffer()
	if buf == nil {
		t.Fatal("GetLargeBuffer returned nil")
	}
	if len(*buf) != LargeBufferSize {
		t.Errorf("buffer size = %d, want %d", len(*buf), LargeBufferSize)
	}

	PutLargeBuffer(buf)
}

func TestMetaBufferPool(t *testing.T) {
	buf := GetMetaBuffer()
	if buf == nil {
		t.Fatal("GetMetaBuffer returned nil")
	}
	if cap(*buf) < MetaBufferSize {
		t.Errorf("buffer capacity = %d, want >= %d", cap(*buf), MetaBufferSize)
	}
	if len(*buf) != 0 {
		t.Errorf("buffer length = %d, want 0", len(*buf))
	}

	// Append some data
	*buf = append(*buf, []byte("test")...)

	PutMetaBuffer(buf)

	// Get another buffer - should be reset
	buf2 := GetMetaBuffer()
	if len(*buf2) != 0 {
		t.Errorf("reused buffer length = %d, want 0", len(*buf2))
	}

	PutMetaBuffer(buf2)
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

func TestValidateMP3Frame(t *testing.T) {
	validFrame := []byte{0xFF, 0xFB, 0x90, 0x00}
	if !ValidateMP3Frame(validFrame) {
		t.Error("ValidateMP3Frame should return true for valid frame")
	}

	invalidFrame := []byte{0x00, 0x00, 0x00, 0x00}
	if ValidateMP3Frame(invalidFrame) {
		t.Error("ValidateMP3Frame should return false for invalid frame")
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

	// Wait for delay
	time.Sleep(15 * time.Millisecond)

	popped := jb.Pop()
	if popped == nil {
		t.Error("Pop returned nil after delay")
	} else if string(popped) != "test1" {
		t.Errorf("Pop returned %q, want %q", string(popped), "test1")
	}
}

func TestJitterBufferFlush(t *testing.T) {
	jb := NewJitterBuffer(100 * time.Millisecond)

	jb.Push([]byte("test1"))
	jb.Push([]byte("test2"))

	flushed := jb.Flush()
	if len(flushed) != 2 {
		t.Errorf("Flush returned %d items, want 2", len(flushed))
	}

	if jb.Len() != 0 {
		t.Errorf("Len after flush = %d, want 0", jb.Len())
	}
}

func TestJitterBufferReset(t *testing.T) {
	jb := NewJitterBuffer(100 * time.Millisecond)

	jb.Push([]byte("test1"))
	jb.Push([]byte("test2"))

	jb.Reset()

	if jb.Len() != 0 {
		t.Errorf("Len after reset = %d, want 0", jb.Len())
	}
}

// ---------------------------------------------------------
// BENCHMARKS
// ---------------------------------------------------------

func BenchmarkBufferWrite(b *testing.B) {
	buf := NewBuffer(256*1024, 8192)
	data := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		buf.Write(data)
	}
}

func BenchmarkBufferReadFromInto(b *testing.B) {
	buf := NewBuffer(256*1024, 8192)

	// Pre-fill buffer
	data := make([]byte, 128*1024)
	buf.Write(data)

	readBuf := make([]byte, 4096)
	b.ResetTimer()
	b.SetBytes(4096)

	var pos int64
	for i := 0; i < b.N; i++ {
		_, pos = buf.ReadFromInto(pos, readBuf)
		if pos >= buf.WritePos() {
			pos = 0
		}
	}
}

func BenchmarkListenerPositionRead(b *testing.B) {
	buf := NewBuffer(256*1024, 8192)

	// Pre-fill buffer
	data := make([]byte, 128*1024)
	buf.Write(data)

	listener := NewListenerPosition("bench", buf)
	readBuf := make([]byte, 2048)

	b.ResetTimer()
	b.SetBytes(2048)

	for i := 0; i < b.N; i++ {
		n, _ := listener.Read(readBuf)
		if n == 0 {
			// Reset position if we caught up
			listener.Position.Store(0)
		}
	}
}

func BenchmarkBroadcasterWrite(b *testing.B) {
	bc := NewBroadcaster(256*1024, 8192)
	defer bc.Close()

	data := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		bc.Write(data)
	}
}

func BenchmarkBufferConcurrentReadWrite(b *testing.B) {
	buf := NewBuffer(256*1024, 8192)
	data := make([]byte, 1024)

	// Pre-fill
	buf.Write(make([]byte, 64*1024))

	// Add readers that read concurrently
	numReaders := 10
	done := make(chan struct{})

	for i := 0; i < numReaders; i++ {
		listener := NewListenerPosition(string(rune('A'+i)), buf)
		go func(lp *ListenerPosition) {
			readBuf := make([]byte, 1024)
			for {
				select {
				case <-done:
					return
				default:
					lp.Read(readBuf)
				}
			}
		}(listener)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		buf.Write(data)
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

func BenchmarkSmallBufferPool(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf := GetSmallBuffer()
		PutSmallBuffer(buf)
	}
}

func BenchmarkSmallBufferPoolParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := GetSmallBuffer()
			// Simulate some work
			(*buf)[0] = 1
			PutSmallBuffer(buf)
		}
	})
}

func BenchmarkWaitForData(b *testing.B) {
	buf := NewBuffer(256*1024, 8192)

	// Start a writer goroutine
	done := make(chan struct{})
	go func() {
		data := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				buf.Write(data)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	b.ResetTimer()

	pos := int64(0)
	for i := 0; i < b.N; i++ {
		buf.WaitForData(pos, 100*time.Millisecond)
		pos = buf.WritePos()
	}

	close(done)
}
