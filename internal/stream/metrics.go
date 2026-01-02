// Package stream provides comprehensive metrics and benchmarking for audio streaming
// This file implements real-time performance monitoring, latency tracking, and throughput analysis

package stream

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------
// HISTOGRAM - For latency distribution tracking
// ---------------------------------------------------------

// Histogram tracks distribution of values with configurable buckets
type Histogram struct {
	buckets     []int64   // Counts per bucket
	boundaries  []float64 // Upper bounds for each bucket
	sum         float64
	count       int64
	min         float64
	max         float64
	mu          sync.RWMutex
	initialized bool
}

// NewHistogram creates a histogram with exponential buckets
// Suitable for latency tracking (microseconds to seconds)
func NewHistogram(bucketCount int, minValue, maxValue float64) *Histogram {
	if bucketCount < 2 {
		bucketCount = 20
	}

	h := &Histogram{
		buckets:    make([]int64, bucketCount+1), // +1 for overflow bucket
		boundaries: make([]float64, bucketCount),
		min:        math.MaxFloat64,
		max:        0,
	}

	// Create exponential buckets
	factor := math.Pow(maxValue/minValue, 1.0/float64(bucketCount-1))
	current := minValue
	for i := 0; i < bucketCount; i++ {
		h.boundaries[i] = current
		current *= factor
	}

	h.initialized = true
	return h
}

// NewLatencyHistogram creates a histogram optimized for latency tracking
// Buckets: 10µs to 10s with 20 exponential buckets
func NewLatencyHistogram() *Histogram {
	return NewHistogram(20, 0.00001, 10.0) // 10µs to 10s
}

// Observe records a value in the histogram
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += value
	h.count++

	if value < h.min {
		h.min = value
	}
	if value > h.max {
		h.max = value
	}

	// Find bucket
	bucket := len(h.buckets) - 1 // Overflow bucket by default
	for i, boundary := range h.boundaries {
		if value <= boundary {
			bucket = i
			break
		}
	}
	h.buckets[bucket]++
}

// Percentile calculates the approximate percentile value (0-100)
func (h *Histogram) Percentile(p float64) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return 0
	}

	target := int64(float64(h.count) * p / 100.0)
	var cumulative int64

	for i, count := range h.buckets {
		cumulative += count
		if cumulative >= target {
			if i == 0 {
				return h.boundaries[0] / 2
			}
			if i >= len(h.boundaries) {
				return h.max
			}
			// Linear interpolation within bucket
			prevBoundary := float64(0)
			if i > 0 {
				prevBoundary = h.boundaries[i-1]
			}
			return (prevBoundary + h.boundaries[i]) / 2
		}
	}

	return h.max
}

// Stats returns histogram statistics
func (h *Histogram) Stats() HistogramStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := HistogramStats{
		Count: h.count,
		Sum:   h.sum,
		Min:   h.min,
		Max:   h.max,
	}

	if h.count > 0 {
		stats.Avg = h.sum / float64(h.count)
	}

	return stats
}

// Reset clears the histogram
func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.buckets {
		h.buckets[i] = 0
	}
	h.sum = 0
	h.count = 0
	h.min = math.MaxFloat64
	h.max = 0
}

// HistogramStats contains histogram statistics
type HistogramStats struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
	Avg   float64 `json:"avg"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// ---------------------------------------------------------
// RATE CALCULATOR - For throughput measurement
// ---------------------------------------------------------

// RateCalculator calculates rates over a sliding window
type RateCalculator struct {
	samples    []rateSample
	windowSize int
	position   int
	mu         sync.RWMutex
	total      int64
}

type rateSample struct {
	value int64
	time  time.Time
}

// NewRateCalculator creates a new rate calculator
func NewRateCalculator(windowSize int) *RateCalculator {
	if windowSize < 10 {
		windowSize = 60 // Default: 60 samples
	}
	return &RateCalculator{
		samples:    make([]rateSample, windowSize),
		windowSize: windowSize,
	}
}

// Add records a value
func (rc *RateCalculator) Add(value int64) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.samples[rc.position] = rateSample{
		value: value,
		time:  time.Now(),
	}
	rc.position = (rc.position + 1) % rc.windowSize
	atomic.AddInt64(&rc.total, value)
}

// Rate returns the current rate (values per second)
func (rc *RateCalculator) Rate() float64 {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	// Find oldest and newest samples
	var oldest, newest rateSample
	var totalValue int64
	var validCount int

	for _, s := range rc.samples {
		if s.time.IsZero() {
			continue
		}
		validCount++
		totalValue += s.value

		if oldest.time.IsZero() || s.time.Before(oldest.time) {
			oldest = s
		}
		if newest.time.IsZero() || s.time.After(newest.time) {
			newest = s
		}
	}

	if validCount < 2 {
		return 0
	}

	duration := newest.time.Sub(oldest.time).Seconds()
	if duration <= 0 {
		return 0
	}

	return float64(totalValue) / duration
}

// Total returns the cumulative total
func (rc *RateCalculator) Total() int64 {
	return atomic.LoadInt64(&rc.total)
}

// ---------------------------------------------------------
// STREAM METRICS - Comprehensive streaming metrics
// ---------------------------------------------------------

// StreamMetrics tracks all metrics for a stream
type StreamMetrics struct {
	// Identity
	MountPath string    `json:"mount_path"`
	StartTime time.Time `json:"start_time"`

	// Throughput
	BytesReceived int64 `json:"bytes_received"`
	BytesSent     int64 `json:"bytes_sent"`
	bytesRecvRate *RateCalculator
	bytesSentRate *RateCalculator

	// Listeners
	CurrentListeners int32 `json:"current_listeners"`
	PeakListeners    int32 `json:"peak_listeners"`
	TotalConnects    int64 `json:"total_connects"`
	TotalDisconnects int64 `json:"total_disconnects"`

	// Latency (in seconds)
	WriteLatency *Histogram `json:"-"`
	ReadLatency  *Histogram `json:"-"`

	// Quality
	SkipToLiveCount int64 `json:"skip_to_live_count"`
	BufferUnderruns int64 `json:"buffer_underruns"`
	BufferOverruns  int64 `json:"buffer_overruns"`

	// Sync tracking
	AvgListenerLag   float64 `json:"avg_listener_lag"`
	MaxListenerLag   int64   `json:"max_listener_lag"`
	ListenerLagStdev float64 `json:"listener_lag_stdev"`

	// Active source info
	SourceActive  bool      `json:"source_active"`
	SourceIP      string    `json:"source_ip"`
	SourceConnect time.Time `json:"source_connect"`

	mu sync.RWMutex
}

// NewStreamMetrics creates a new metrics tracker
func NewStreamMetrics(mountPath string) *StreamMetrics {
	return &StreamMetrics{
		MountPath:     mountPath,
		StartTime:     time.Now(),
		bytesRecvRate: NewRateCalculator(60),
		bytesSentRate: NewRateCalculator(60),
		WriteLatency:  NewLatencyHistogram(),
		ReadLatency:   NewLatencyHistogram(),
	}
}

// RecordBytesReceived records bytes received from source
func (m *StreamMetrics) RecordBytesReceived(n int) {
	atomic.AddInt64(&m.BytesReceived, int64(n))
	m.bytesRecvRate.Add(int64(n))
}

// RecordBytesSent records bytes sent to listeners
func (m *StreamMetrics) RecordBytesSent(n int) {
	atomic.AddInt64(&m.BytesSent, int64(n))
	m.bytesSentRate.Add(int64(n))
}

// RecordWriteLatency records source write latency
func (m *StreamMetrics) RecordWriteLatency(d time.Duration) {
	m.WriteLatency.Observe(d.Seconds())
}

// RecordReadLatency records listener read latency
func (m *StreamMetrics) RecordReadLatency(d time.Duration) {
	m.ReadLatency.Observe(d.Seconds())
}

// RecordListenerConnect records a new listener connection
func (m *StreamMetrics) RecordListenerConnect() {
	atomic.AddInt64(&m.TotalConnects, 1)
	count := atomic.AddInt32(&m.CurrentListeners, 1)

	// Update peak
	for {
		peak := atomic.LoadInt32(&m.PeakListeners)
		if count <= peak || atomic.CompareAndSwapInt32(&m.PeakListeners, peak, count) {
			break
		}
	}
}

// RecordListenerDisconnect records a listener disconnection
func (m *StreamMetrics) RecordListenerDisconnect() {
	atomic.AddInt64(&m.TotalDisconnects, 1)
	atomic.AddInt32(&m.CurrentListeners, -1)
}

// RecordSkipToLive records when a listener skips to live
func (m *StreamMetrics) RecordSkipToLive() {
	atomic.AddInt64(&m.SkipToLiveCount, 1)
}

// RecordBufferUnderrun records a buffer underrun event
func (m *StreamMetrics) RecordBufferUnderrun() {
	atomic.AddInt64(&m.BufferUnderruns, 1)
}

// RecordBufferOverrun records a buffer overrun event
func (m *StreamMetrics) RecordBufferOverrun() {
	atomic.AddInt64(&m.BufferOverruns, 1)
}

// UpdateListenerLagStats updates listener lag statistics
func (m *StreamMetrics) UpdateListenerLagStats(lags []int64) {
	if len(lags) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Calculate average
	var sum int64
	var max int64
	for _, lag := range lags {
		sum += lag
		if lag > max {
			max = lag
		}
	}
	avg := float64(sum) / float64(len(lags))

	// Calculate standard deviation
	var sqDiffSum float64
	for _, lag := range lags {
		diff := float64(lag) - avg
		sqDiffSum += diff * diff
	}
	stdev := math.Sqrt(sqDiffSum / float64(len(lags)))

	m.AvgListenerLag = avg
	m.MaxListenerLag = max
	m.ListenerLagStdev = stdev
}

// SetSourceActive sets source status
func (m *StreamMetrics) SetSourceActive(active bool, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SourceActive = active
	if active {
		m.SourceIP = ip
		m.SourceConnect = time.Now()
	} else {
		m.SourceIP = ""
	}
}

// Snapshot returns a point-in-time snapshot of metrics
func (m *StreamMetrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		MountPath:        m.MountPath,
		Uptime:           time.Since(m.StartTime),
		BytesReceived:    atomic.LoadInt64(&m.BytesReceived),
		BytesSent:        atomic.LoadInt64(&m.BytesSent),
		ReceiveRate:      m.bytesRecvRate.Rate(),
		SendRate:         m.bytesSentRate.Rate(),
		CurrentListeners: atomic.LoadInt32(&m.CurrentListeners),
		PeakListeners:    atomic.LoadInt32(&m.PeakListeners),
		TotalConnects:    atomic.LoadInt64(&m.TotalConnects),
		TotalDisconnects: atomic.LoadInt64(&m.TotalDisconnects),
		WriteLatencyP50:  m.WriteLatency.Percentile(50) * 1000, // ms
		WriteLatencyP99:  m.WriteLatency.Percentile(99) * 1000, // ms
		ReadLatencyP50:   m.ReadLatency.Percentile(50) * 1000,  // ms
		ReadLatencyP99:   m.ReadLatency.Percentile(99) * 1000,  // ms
		SkipToLiveCount:  atomic.LoadInt64(&m.SkipToLiveCount),
		BufferUnderruns:  atomic.LoadInt64(&m.BufferUnderruns),
		BufferOverruns:   atomic.LoadInt64(&m.BufferOverruns),
		AvgListenerLag:   m.AvgListenerLag,
		MaxListenerLag:   m.MaxListenerLag,
		ListenerLagStdev: m.ListenerLagStdev,
		SourceActive:     m.SourceActive,
		SourceIP:         m.SourceIP,
	}
}

// MetricsSnapshot is a point-in-time view of metrics
type MetricsSnapshot struct {
	MountPath        string        `json:"mount_path"`
	Uptime           time.Duration `json:"uptime"`
	BytesReceived    int64         `json:"bytes_received"`
	BytesSent        int64         `json:"bytes_sent"`
	ReceiveRate      float64       `json:"receive_rate_bps"`
	SendRate         float64       `json:"send_rate_bps"`
	CurrentListeners int32         `json:"current_listeners"`
	PeakListeners    int32         `json:"peak_listeners"`
	TotalConnects    int64         `json:"total_connects"`
	TotalDisconnects int64         `json:"total_disconnects"`
	WriteLatencyP50  float64       `json:"write_latency_p50_ms"`
	WriteLatencyP99  float64       `json:"write_latency_p99_ms"`
	ReadLatencyP50   float64       `json:"read_latency_p50_ms"`
	ReadLatencyP99   float64       `json:"read_latency_p99_ms"`
	SkipToLiveCount  int64         `json:"skip_to_live_count"`
	BufferUnderruns  int64         `json:"buffer_underruns"`
	BufferOverruns   int64         `json:"buffer_overruns"`
	AvgListenerLag   float64       `json:"avg_listener_lag_bytes"`
	MaxListenerLag   int64         `json:"max_listener_lag_bytes"`
	ListenerLagStdev float64       `json:"listener_lag_stdev"`
	SourceActive     bool          `json:"source_active"`
	SourceIP         string        `json:"source_ip,omitempty"`
}

// JSON returns JSON representation
func (s MetricsSnapshot) JSON() string {
	data, _ := json.MarshalIndent(s, "", "  ")
	return string(data)
}

// ---------------------------------------------------------
// GLOBAL METRICS REGISTRY
// ---------------------------------------------------------

// MetricsRegistry holds metrics for all streams
type MetricsRegistry struct {
	streams sync.Map // map[string]*StreamMetrics

	// Global metrics
	StartTime      time.Time
	TotalBytes     int64
	TotalListeners int64
}

// GlobalRegistry is the singleton metrics registry
var GlobalRegistry = &MetricsRegistry{
	StartTime: time.Now(),
}

// GetOrCreate returns metrics for a mount, creating if needed
func (r *MetricsRegistry) GetOrCreate(mountPath string) *StreamMetrics {
	if m, ok := r.streams.Load(mountPath); ok {
		return m.(*StreamMetrics)
	}

	metrics := NewStreamMetrics(mountPath)
	actual, _ := r.streams.LoadOrStore(mountPath, metrics)
	return actual.(*StreamMetrics)
}

// Get returns metrics for a mount
func (r *MetricsRegistry) Get(mountPath string) *StreamMetrics {
	if m, ok := r.streams.Load(mountPath); ok {
		return m.(*StreamMetrics)
	}
	return nil
}

// Remove removes metrics for a mount
func (r *MetricsRegistry) Remove(mountPath string) {
	r.streams.Delete(mountPath)
}

// All returns all stream metrics
func (r *MetricsRegistry) All() []*StreamMetrics {
	var result []*StreamMetrics
	r.streams.Range(func(key, value interface{}) bool {
		result = append(result, value.(*StreamMetrics))
		return true
	})
	return result
}

// GlobalSnapshot returns global metrics
func (r *MetricsRegistry) GlobalSnapshot() GlobalMetrics {
	var totalBytes, totalSent int64
	var totalListeners int32
	var peakListeners int32
	var mounts int

	r.streams.Range(func(key, value interface{}) bool {
		m := value.(*StreamMetrics)
		totalBytes += atomic.LoadInt64(&m.BytesReceived)
		totalSent += atomic.LoadInt64(&m.BytesSent)
		listeners := atomic.LoadInt32(&m.CurrentListeners)
		totalListeners += listeners
		peak := atomic.LoadInt32(&m.PeakListeners)
		if peak > peakListeners {
			peakListeners = peak
		}
		mounts++
		return true
	})

	return GlobalMetrics{
		Uptime:           time.Since(r.StartTime),
		ActiveMounts:     mounts,
		TotalListeners:   int(totalListeners),
		PeakListeners:    int(peakListeners),
		TotalBytesIn:     totalBytes,
		TotalBytesOut:    totalSent,
		MemoryUsageBytes: getMemoryUsage(),
	}
}

// GlobalMetrics contains server-wide metrics
type GlobalMetrics struct {
	Uptime           time.Duration `json:"uptime"`
	ActiveMounts     int           `json:"active_mounts"`
	TotalListeners   int           `json:"total_listeners"`
	PeakListeners    int           `json:"peak_listeners"`
	TotalBytesIn     int64         `json:"total_bytes_in"`
	TotalBytesOut    int64         `json:"total_bytes_out"`
	MemoryUsageBytes int64         `json:"memory_usage_bytes"`
}

// getMemoryUsage returns approximate memory usage (placeholder)
func getMemoryUsage() int64 {
	// In production, use runtime.ReadMemStats
	return 0
}

// ---------------------------------------------------------
// BENCHMARKING
// ---------------------------------------------------------

// Benchmark measures streaming performance
type Benchmark struct {
	Name       string
	Iterations int
	Duration   time.Duration
	Results    []BenchmarkResult
}

// BenchmarkResult contains results from a benchmark run
type BenchmarkResult struct {
	Iteration       int
	BytesWritten    int64
	BytesRead       int64
	WriteDuration   time.Duration
	ReadDuration    time.Duration
	WriteOps        int
	ReadOps         int
	WriteThroughput float64 // bytes/sec
	ReadThroughput  float64 // bytes/sec
	WriteLatencyAvg time.Duration
	ReadLatencyAvg  time.Duration
}

// RunBufferBenchmark benchmarks the broadcast buffer
func RunBufferBenchmark(bufSize, burstSize, dataSize, iterations int) *Benchmark {
	bench := &Benchmark{
		Name:       fmt.Sprintf("BroadcastBuffer-%d-%d", bufSize, dataSize),
		Iterations: iterations,
		Results:    make([]BenchmarkResult, iterations),
	}

	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	startTotal := time.Now()

	for iter := 0; iter < iterations; iter++ {
		buffer := NewBroadcastBuffer(bufSize, burstSize)
		result := BenchmarkResult{Iteration: iter}

		// Benchmark writes
		writeStart := time.Now()
		ops := 0
		var bytesWritten int64
		for bytesWritten < int64(bufSize*2) {
			n, _ := buffer.WriteBatch(data)
			bytesWritten += int64(n)
			ops++
		}
		result.WriteDuration = time.Since(writeStart)
		result.BytesWritten = bytesWritten
		result.WriteOps = ops
		result.WriteThroughput = float64(bytesWritten) / result.WriteDuration.Seconds()
		result.WriteLatencyAvg = result.WriteDuration / time.Duration(ops)

		// Benchmark reads
		listener := NewBroadcastListener("bench", buffer)
		readStart := time.Now()
		readOps := 0
		var bytesRead int64
		for bytesRead < bytesWritten/2 {
			d := listener.Read(dataSize)
			bytesRead += int64(len(d))
			readOps++
			if len(d) == 0 {
				break
			}
		}
		result.ReadDuration = time.Since(readStart)
		result.BytesRead = bytesRead
		result.ReadOps = readOps
		if readOps > 0 {
			result.ReadThroughput = float64(bytesRead) / result.ReadDuration.Seconds()
			result.ReadLatencyAvg = result.ReadDuration / time.Duration(readOps)
		}

		bench.Results[iter] = result
	}

	bench.Duration = time.Since(startTotal)
	return bench
}

// Summary returns benchmark summary statistics
func (b *Benchmark) Summary() BenchmarkSummary {
	var totalWriteThroughput, totalReadThroughput float64
	var writeThroughputs, readThroughputs []float64

	for _, r := range b.Results {
		totalWriteThroughput += r.WriteThroughput
		totalReadThroughput += r.ReadThroughput
		writeThroughputs = append(writeThroughputs, r.WriteThroughput)
		readThroughputs = append(readThroughputs, r.ReadThroughput)
	}

	n := float64(len(b.Results))

	sort.Float64s(writeThroughputs)
	sort.Float64s(readThroughputs)

	return BenchmarkSummary{
		Name:               b.Name,
		Iterations:         b.Iterations,
		TotalDuration:      b.Duration,
		AvgWriteThroughput: totalWriteThroughput / n,
		AvgReadThroughput:  totalReadThroughput / n,
		P50WriteThroughput: percentileFloat64(writeThroughputs, 50),
		P99WriteThroughput: percentileFloat64(writeThroughputs, 99),
		P50ReadThroughput:  percentileFloat64(readThroughputs, 50),
		P99ReadThroughput:  percentileFloat64(readThroughputs, 99),
	}
}

// BenchmarkSummary contains summary statistics
type BenchmarkSummary struct {
	Name               string        `json:"name"`
	Iterations         int           `json:"iterations"`
	TotalDuration      time.Duration `json:"total_duration"`
	AvgWriteThroughput float64       `json:"avg_write_throughput_bps"`
	AvgReadThroughput  float64       `json:"avg_read_throughput_bps"`
	P50WriteThroughput float64       `json:"p50_write_throughput_bps"`
	P99WriteThroughput float64       `json:"p99_write_throughput_bps"`
	P50ReadThroughput  float64       `json:"p50_read_throughput_bps"`
	P99ReadThroughput  float64       `json:"p99_read_throughput_bps"`
}

func percentileFloat64(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

// String returns human-readable summary
func (s BenchmarkSummary) String() string {
	return fmt.Sprintf(`Benchmark: %s
Iterations: %d
Total Duration: %v
Write Throughput: avg=%.2f MB/s, p50=%.2f MB/s, p99=%.2f MB/s
Read Throughput:  avg=%.2f MB/s, p50=%.2f MB/s, p99=%.2f MB/s`,
		s.Name,
		s.Iterations,
		s.TotalDuration,
		s.AvgWriteThroughput/1024/1024,
		s.P50WriteThroughput/1024/1024,
		s.P99WriteThroughput/1024/1024,
		s.AvgReadThroughput/1024/1024,
		s.P50ReadThroughput/1024/1024,
		s.P99ReadThroughput/1024/1024,
	)
}
