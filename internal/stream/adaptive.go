// Package stream provides adaptive streaming with quality of service
// This file implements intelligent bandwidth management and listener quality tracking

package stream

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------
// QUALITY OF SERVICE LEVELS
// ---------------------------------------------------------

// QoSLevel represents quality of service tier
type QoSLevel int

const (
	// QoSUltraLow - Minimal buffering, for local/fast connections
	QoSUltraLow QoSLevel = iota
	// QoSLow - Low latency, some buffering
	QoSLow
	// QoSMedium - Balanced latency and stability
	QoSMedium
	// QoSHigh - Prioritize stability over latency
	QoSHigh
	// QoSAdaptive - Automatically adjust based on conditions
	QoSAdaptive
)

// QoSConfig contains QoS-specific settings
type QoSConfig struct {
	Level         QoSLevel
	BurstSize     int
	MaxLag        int64
	ChunkSize     int
	PollInterval  time.Duration
	SkipThreshold int64
}

// DefaultQoSConfigs maps QoS levels to configurations
var DefaultQoSConfigs = map[QoSLevel]QoSConfig{
	QoSUltraLow: {
		Level:         QoSUltraLow,
		BurstSize:     2048, // 2KB burst
		MaxLag:        8192, // 8KB max lag before skip
		ChunkSize:     512,  // 512B chunks
		PollInterval:  2 * time.Millisecond,
		SkipThreshold: 4096,
	},
	QoSLow: {
		Level:         QoSLow,
		BurstSize:     4096,  // 4KB burst
		MaxLag:        16384, // 16KB max lag
		ChunkSize:     1024,  // 1KB chunks
		PollInterval:  5 * time.Millisecond,
		SkipThreshold: 8192,
	},
	QoSMedium: {
		Level:         QoSMedium,
		BurstSize:     8192,  // 8KB burst
		MaxLag:        32768, // 32KB max lag
		ChunkSize:     2048,  // 2KB chunks
		PollInterval:  10 * time.Millisecond,
		SkipThreshold: 16384,
	},
	QoSHigh: {
		Level:         QoSHigh,
		BurstSize:     16384, // 16KB burst
		MaxLag:        65536, // 64KB max lag
		ChunkSize:     4096,  // 4KB chunks
		PollInterval:  20 * time.Millisecond,
		SkipThreshold: 32768,
	},
}

// ---------------------------------------------------------
// LISTENER STATISTICS
// ---------------------------------------------------------

// ListenerStats tracks performance metrics for a listener
type ListenerStats struct {
	// Bandwidth metrics
	BytesSent   int64
	BytesPerSec float64
	PeakBytesPS float64
	AvgBytesPS  float64

	// Latency metrics
	CurrentLag int64
	AvgLag     float64
	MaxLag     int64
	MinLag     int64

	// Quality metrics
	SkipCount    int32
	BufferUnder  int32 // Times we ran out of data
	BufferOver   int32 // Times we had to skip ahead
	QualityScore float64

	// Timing
	ConnectedAt   time.Time
	LastUpdate    time.Time
	ConnectionAge time.Duration

	// Computed
	QoSLevel QoSLevel
}

// CalculateQualityScore computes overall quality (0.0 to 1.0)
func (s *ListenerStats) CalculateQualityScore() float64 {
	// Factors:
	// - Low lag is good (weight: 40%)
	// - Few skips is good (weight: 30%)
	// - Stable bandwidth is good (weight: 30%)

	// Lag score (0-1, lower is better)
	lagScore := 1.0 - math.Min(float64(s.CurrentLag)/float64(65536), 1.0)

	// Skip score (0-1, fewer is better)
	skipScore := 1.0 - math.Min(float64(s.SkipCount)/10.0, 1.0)

	// Stability score (how consistent is bandwidth)
	stabilityScore := 1.0
	if s.PeakBytesPS > 0 && s.AvgBytesPS > 0 {
		variance := (s.PeakBytesPS - s.AvgBytesPS) / s.PeakBytesPS
		stabilityScore = 1.0 - math.Min(variance, 1.0)
	}

	s.QualityScore = lagScore*0.4 + skipScore*0.3 + stabilityScore*0.3
	return s.QualityScore
}

// ---------------------------------------------------------
// ADAPTIVE LISTENER
// ---------------------------------------------------------

// AdaptiveListener wraps ListenerPosition with QoS awareness
type AdaptiveListener struct {
	*ListenerPosition

	// QoS
	qosLevel  QoSLevel
	qosConfig QoSConfig

	// Stats tracking
	stats   ListenerStats
	statsMu sync.RWMutex

	// Bandwidth measurement
	bytesWindow []int64     // Sliding window of bytes sent
	timesWindow []time.Time // Corresponding timestamps
	windowSize  int
	windowPos   int

	// Adaptive control
	adaptiveEnabled bool
	lastAdapt       time.Time
	adaptInterval   time.Duration

	// Back-pressure detection
	writeLatencies []time.Duration
	latencyPos     int
}

// NewAdaptiveListener creates a new QoS-aware listener
func NewAdaptiveListener(base *ListenerPosition, level QoSLevel) *AdaptiveListener {
	config, ok := DefaultQoSConfigs[level]
	if !ok {
		config = DefaultQoSConfigs[QoSMedium]
		level = QoSMedium
	}

	al := &AdaptiveListener{
		ListenerPosition: base,
		qosLevel:         level,
		qosConfig:        config,
		windowSize:       30, // 30 samples for averaging
		bytesWindow:      make([]int64, 30),
		timesWindow:      make([]time.Time, 30),
		writeLatencies:   make([]time.Duration, 10),
		adaptiveEnabled:  level == QoSAdaptive,
		adaptInterval:    time.Second,
	}

	al.stats.ConnectedAt = time.Now()
	al.stats.MinLag = math.MaxInt64

	return al
}

// Read reads data with QoS awareness
func (al *AdaptiveListener) Read(buf []byte) (int, bool) {
	// Limit read size to QoS chunk size
	maxRead := al.qosConfig.ChunkSize
	if len(buf) > maxRead {
		buf = buf[:maxRead]
	}

	n, ok := al.ListenerPosition.Read(buf)

	if n > 0 {
		al.recordMetrics(n)
	}

	// Periodically adapt QoS level
	if al.adaptiveEnabled && time.Since(al.lastAdapt) > al.adaptInterval {
		al.adapt()
	}

	return n, ok
}

// GetLag returns the current lag in bytes
func (al *AdaptiveListener) GetLag() int64 {
	return al.ListenerPosition.GetLag()
}

// GetSkipCount returns the number of skip-to-live events
func (al *AdaptiveListener) GetSkipCount() int32 {
	return al.ListenerPosition.SkipCount.Load()
}

// recordMetrics updates statistics
func (al *AdaptiveListener) recordMetrics(bytesRead int) {
	al.statsMu.Lock()
	defer al.statsMu.Unlock()

	now := time.Now()

	// Update sliding window
	al.bytesWindow[al.windowPos] = int64(bytesRead)
	al.timesWindow[al.windowPos] = now
	al.windowPos = (al.windowPos + 1) % al.windowSize

	// Update stats
	al.stats.BytesSent += int64(bytesRead)
	al.stats.LastUpdate = now
	al.stats.ConnectionAge = now.Sub(al.stats.ConnectedAt)

	// Calculate current bandwidth
	var totalBytes int64
	var oldestTime time.Time
	validSamples := 0

	for i := 0; i < al.windowSize; i++ {
		if !al.timesWindow[i].IsZero() {
			totalBytes += al.bytesWindow[i]
			if oldestTime.IsZero() || al.timesWindow[i].Before(oldestTime) {
				oldestTime = al.timesWindow[i]
			}
			validSamples++
		}
	}

	if validSamples > 1 && !oldestTime.IsZero() {
		duration := now.Sub(oldestTime).Seconds()
		if duration > 0 {
			al.stats.BytesPerSec = float64(totalBytes) / duration
			if al.stats.BytesPerSec > al.stats.PeakBytesPS {
				al.stats.PeakBytesPS = al.stats.BytesPerSec
			}
			// Exponential moving average
			if al.stats.AvgBytesPS == 0 {
				al.stats.AvgBytesPS = al.stats.BytesPerSec
			} else {
				al.stats.AvgBytesPS = al.stats.AvgBytesPS*0.9 + al.stats.BytesPerSec*0.1
			}
		}
	}

	// Update lag stats
	lag := al.GetLag()
	al.stats.CurrentLag = lag
	if lag > al.stats.MaxLag {
		al.stats.MaxLag = lag
	}
	if lag < al.stats.MinLag {
		al.stats.MinLag = lag
	}
	// Exponential moving average for lag
	if al.stats.AvgLag == 0 {
		al.stats.AvgLag = float64(lag)
	} else {
		al.stats.AvgLag = al.stats.AvgLag*0.95 + float64(lag)*0.05
	}

	// Update skip count
	al.stats.SkipCount = al.GetSkipCount()
}

// adapt adjusts QoS level based on performance
func (al *AdaptiveListener) adapt() {
	al.statsMu.Lock()
	defer al.statsMu.Unlock()

	al.lastAdapt = time.Now()
	al.stats.CalculateQualityScore()

	// Determine optimal QoS level
	var newLevel QoSLevel

	if al.stats.QualityScore > 0.9 {
		// Excellent quality - try lower latency
		newLevel = QoSLow
	} else if al.stats.QualityScore > 0.7 {
		// Good quality - stay balanced
		newLevel = QoSMedium
	} else if al.stats.QualityScore > 0.5 {
		// Okay quality - increase buffering
		newLevel = QoSHigh
	} else {
		// Poor quality - maximum buffering
		newLevel = QoSHigh
	}

	// Check for specific issues
	if al.stats.SkipCount > 5 {
		// Too many skips - increase buffer
		if newLevel < QoSHigh {
			newLevel++
		}
	}

	if al.stats.CurrentLag > int64(al.qosConfig.MaxLag)/2 {
		// Consistently behind - increase tolerance
		if newLevel < QoSHigh {
			newLevel++
		}
	}

	// Apply new level if changed
	if newLevel != al.qosLevel {
		if config, ok := DefaultQoSConfigs[newLevel]; ok {
			al.qosLevel = newLevel
			al.qosConfig = config
			al.stats.QoSLevel = newLevel
		}
	}
}

// Stats returns current statistics
func (al *AdaptiveListener) Stats() ListenerStats {
	al.statsMu.RLock()
	defer al.statsMu.RUnlock()
	return al.stats
}

// QoS returns current QoS level
func (al *AdaptiveListener) QoS() QoSLevel {
	al.statsMu.RLock()
	defer al.statsMu.RUnlock()
	return al.qosLevel
}

// SetQoS manually sets QoS level (disables adaptive)
func (al *AdaptiveListener) SetQoS(level QoSLevel) {
	al.statsMu.Lock()
	defer al.statsMu.Unlock()

	if config, ok := DefaultQoSConfigs[level]; ok {
		al.qosLevel = level
		al.qosConfig = config
		al.adaptiveEnabled = (level == QoSAdaptive)
	}
}

// Config returns current QoS config
func (al *AdaptiveListener) Config() QoSConfig {
	al.statsMu.RLock()
	defer al.statsMu.RUnlock()
	return al.qosConfig
}

// ---------------------------------------------------------
// ADAPTIVE STREAM MANAGER
// ---------------------------------------------------------

// AdaptiveStreamManager manages streams with QoS awareness
type AdaptiveStreamManager struct {
	buffer    *Buffer
	listeners sync.Map // map[string]*AdaptiveListener

	// Global stats
	totalListeners int32
	peakListeners  int32
	totalBytes     int64
	avgQuality     float64

	// Config
	defaultQoS QoSLevel
	autoAdapt  bool

	// Monitoring
	monitorTicker *time.Ticker
	monitorDone   chan struct{}
	mu            sync.RWMutex
}

// NewAdaptiveStreamManager creates a new adaptive stream manager
func NewAdaptiveStreamManager(bufSize, burstSize int, defaultQoS QoSLevel) *AdaptiveStreamManager {
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	if burstSize <= 0 {
		burstSize = DefaultBurstSize
	}

	asm := &AdaptiveStreamManager{
		buffer:      NewBuffer(bufSize, burstSize),
		defaultQoS:  defaultQoS,
		autoAdapt:   defaultQoS == QoSAdaptive,
		monitorDone: make(chan struct{}),
	}

	// Start monitoring goroutine
	asm.monitorTicker = time.NewTicker(5 * time.Second)
	go asm.monitor()

	return asm
}

// Write writes data to the buffer and notifies all listeners
func (asm *AdaptiveStreamManager) Write(data []byte) (int, error) {
	n, err := asm.buffer.Write(data)
	atomic.AddInt64(&asm.totalBytes, int64(n))
	return n, err
}

// AddListener adds a new adaptive listener
func (asm *AdaptiveStreamManager) AddListener(id string) *AdaptiveListener {
	// Create the base listener position
	baseListener := NewListenerPosition(id, asm.buffer)
	adaptiveListener := NewAdaptiveListener(baseListener, asm.defaultQoS)

	asm.listeners.Store(id, adaptiveListener)
	count := atomic.AddInt32(&asm.totalListeners, 1)

	// Update peak
	for {
		peak := atomic.LoadInt32(&asm.peakListeners)
		if count <= peak || atomic.CompareAndSwapInt32(&asm.peakListeners, peak, count) {
			break
		}
	}

	return adaptiveListener
}

// RemoveListener removes a listener
func (asm *AdaptiveStreamManager) RemoveListener(id string) {
	if l, ok := asm.listeners.LoadAndDelete(id); ok {
		l.(*AdaptiveListener).Close()
		atomic.AddInt32(&asm.totalListeners, -1)
	}
}

// GetListener returns a listener by ID
func (asm *AdaptiveStreamManager) GetListener(id string) *AdaptiveListener {
	if l, ok := asm.listeners.Load(id); ok {
		return l.(*AdaptiveListener)
	}
	return nil
}

// monitor runs periodic health checks
func (asm *AdaptiveStreamManager) monitor() {
	for {
		select {
		case <-asm.monitorDone:
			return
		case <-asm.monitorTicker.C:
			asm.updateGlobalStats()
		}
	}
}

// updateGlobalStats calculates aggregate statistics
func (asm *AdaptiveStreamManager) updateGlobalStats() {
	var totalQuality float64
	var count int

	asm.listeners.Range(func(key, value interface{}) bool {
		listener := value.(*AdaptiveListener)
		stats := listener.Stats()
		stats.CalculateQualityScore()
		totalQuality += stats.QualityScore
		count++
		return true
	})

	asm.mu.Lock()
	if count > 0 {
		asm.avgQuality = totalQuality / float64(count)
	}
	asm.mu.Unlock()
}

// GlobalStats returns aggregate statistics
func (asm *AdaptiveStreamManager) GlobalStats() (listeners, peak int, totalBytes int64, avgQuality float64) {
	asm.mu.RLock()
	avgQuality = asm.avgQuality
	asm.mu.RUnlock()

	listeners = int(atomic.LoadInt32(&asm.totalListeners))
	peak = int(atomic.LoadInt32(&asm.peakListeners))
	totalBytes = atomic.LoadInt64(&asm.totalBytes)
	return
}

// Buffer returns the underlying buffer
func (asm *AdaptiveStreamManager) Buffer() *Buffer {
	return asm.buffer
}

// SetDefaultQoS sets the default QoS for new listeners
func (asm *AdaptiveStreamManager) SetDefaultQoS(level QoSLevel) {
	asm.mu.Lock()
	asm.defaultQoS = level
	asm.autoAdapt = (level == QoSAdaptive)
	asm.mu.Unlock()
}

// Close shuts down the manager
func (asm *AdaptiveStreamManager) Close() {
	close(asm.monitorDone)
	asm.monitorTicker.Stop()

	// Close all listeners
	asm.listeners.Range(func(key, value interface{}) bool {
		value.(*AdaptiveListener).Close()
		return true
	})
}

// ---------------------------------------------------------
// UTILITY FUNCTIONS
// ---------------------------------------------------------

// QoSLevelString returns string representation of QoS level
func QoSLevelString(level QoSLevel) string {
	switch level {
	case QoSUltraLow:
		return "ultra-low"
	case QoSLow:
		return "low"
	case QoSMedium:
		return "medium"
	case QoSHigh:
		return "high"
	case QoSAdaptive:
		return "adaptive"
	default:
		return "unknown"
	}
}

// ParseQoSLevel parses string to QoS level
func ParseQoSLevel(s string) QoSLevel {
	switch s {
	case "ultra-low", "ultralow", "ultra_low":
		return QoSUltraLow
	case "low":
		return QoSLow
	case "medium", "med":
		return QoSMedium
	case "high":
		return QoSHigh
	case "adaptive", "auto":
		return QoSAdaptive
	default:
		return QoSMedium
	}
}
