// Package stats provides server statistics and metrics collection
package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// ServerStats holds global server statistics
type ServerStats struct {
	StartTime        time.Time
	TotalConnections int64
	TotalBytes       int64
	PeakListeners    int64
	CurrentListeners int64
	TotalSources     int64
	mu               sync.RWMutex
}

// Global server stats instance
var global = &ServerStats{
	StartTime: time.Now(),
}

// Global returns the global stats instance
func Global() *ServerStats {
	return global
}

// Init initializes the global stats
func Init() {
	global = &ServerStats{
		StartTime: time.Now(),
	}
}

// IncrementConnections increments the total connection count
func (s *ServerStats) IncrementConnections() {
	atomic.AddInt64(&s.TotalConnections, 1)
}

// AddBytes adds to the total bytes transferred
func (s *ServerStats) AddBytes(n int64) {
	atomic.AddInt64(&s.TotalBytes, n)
}

// GetTotalBytes returns the total bytes transferred
func (s *ServerStats) GetTotalBytes() int64 {
	return atomic.LoadInt64(&s.TotalBytes)
}

// GetTotalConnections returns the total connection count
func (s *ServerStats) GetTotalConnections() int64 {
	return atomic.LoadInt64(&s.TotalConnections)
}

// SetCurrentListeners sets the current listener count
func (s *ServerStats) SetCurrentListeners(n int64) {
	atomic.StoreInt64(&s.CurrentListeners, n)

	// Update peak if necessary
	for {
		peak := atomic.LoadInt64(&s.PeakListeners)
		if n <= peak {
			break
		}
		if atomic.CompareAndSwapInt64(&s.PeakListeners, peak, n) {
			break
		}
	}
}

// GetCurrentListeners returns the current listener count
func (s *ServerStats) GetCurrentListeners() int64 {
	return atomic.LoadInt64(&s.CurrentListeners)
}

// GetPeakListeners returns the peak listener count
func (s *ServerStats) GetPeakListeners() int64 {
	return atomic.LoadInt64(&s.PeakListeners)
}

// IncrementSources increments the source count
func (s *ServerStats) IncrementSources() {
	atomic.AddInt64(&s.TotalSources, 1)
}

// GetTotalSources returns the total source count
func (s *ServerStats) GetTotalSources() int64 {
	return atomic.LoadInt64(&s.TotalSources)
}

// Uptime returns the server uptime
func (s *ServerStats) Uptime() time.Duration {
	return time.Since(s.StartTime)
}

// Snapshot represents a point-in-time snapshot of stats
type Snapshot struct {
	Timestamp        time.Time
	Uptime           time.Duration
	TotalConnections int64
	TotalBytes       int64
	CurrentListeners int64
	PeakListeners    int64
	TotalSources     int64
}

// Snapshot returns a point-in-time snapshot of the stats
func (s *ServerStats) Snapshot() Snapshot {
	return Snapshot{
		Timestamp:        time.Now(),
		Uptime:           s.Uptime(),
		TotalConnections: s.GetTotalConnections(),
		TotalBytes:       s.GetTotalBytes(),
		CurrentListeners: s.GetCurrentListeners(),
		PeakListeners:    s.GetPeakListeners(),
		TotalSources:     s.GetTotalSources(),
	}
}

// MountStats holds per-mount statistics
type MountStats struct {
	Path             string
	BytesSent        int64
	BytesReceived    int64
	Listeners        int64
	PeakListeners    int64
	ConnectionsTotal int64
	SourceConnected  bool
	SourceIP         string
	StreamStart      time.Time
	ContentType      string
	Bitrate          int
	mu               sync.RWMutex
}

// NewMountStats creates new mount statistics
func NewMountStats(path string) *MountStats {
	return &MountStats{
		Path: path,
	}
}

// AddBytesSent adds to bytes sent
func (m *MountStats) AddBytesSent(n int64) {
	atomic.AddInt64(&m.BytesSent, n)
}

// AddBytesReceived adds to bytes received
func (m *MountStats) AddBytesReceived(n int64) {
	atomic.AddInt64(&m.BytesReceived, n)
}

// IncrementListeners increments the listener count
func (m *MountStats) IncrementListeners() {
	n := atomic.AddInt64(&m.Listeners, 1)
	atomic.AddInt64(&m.ConnectionsTotal, 1)

	// Update peak if necessary
	for {
		peak := atomic.LoadInt64(&m.PeakListeners)
		if n <= peak {
			break
		}
		if atomic.CompareAndSwapInt64(&m.PeakListeners, peak, n) {
			break
		}
	}
}

// DecrementListeners decrements the listener count
func (m *MountStats) DecrementListeners() {
	atomic.AddInt64(&m.Listeners, -1)
}

// GetListeners returns the current listener count
func (m *MountStats) GetListeners() int64 {
	return atomic.LoadInt64(&m.Listeners)
}

// GetPeakListeners returns the peak listener count
func (m *MountStats) GetPeakListeners() int64 {
	return atomic.LoadInt64(&m.PeakListeners)
}

// SetSourceConnected sets the source connection status
func (m *MountStats) SetSourceConnected(connected bool, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SourceConnected = connected
	m.SourceIP = ip
	if connected {
		m.StreamStart = time.Now()
	}
}

// IsSourceConnected returns the source connection status
func (m *MountStats) IsSourceConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.SourceConnected
}

// StreamDuration returns how long the stream has been active
func (m *MountStats) StreamDuration() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.SourceConnected {
		return 0
	}
	return time.Since(m.StreamStart)
}

// Collector collects and aggregates statistics
type Collector struct {
	mounts   map[string]*MountStats
	mu       sync.RWMutex
	interval time.Duration
	done     chan struct{}
}

// NewCollector creates a new stats collector
func NewCollector(interval time.Duration) *Collector {
	return &Collector{
		mounts:   make(map[string]*MountStats),
		interval: interval,
		done:     make(chan struct{}),
	}
}

// GetMountStats returns stats for a specific mount
func (c *Collector) GetMountStats(path string) *MountStats {
	c.mu.RLock()
	stats, exists := c.mounts[path]
	c.mu.RUnlock()

	if exists {
		return stats
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if stats, exists := c.mounts[path]; exists {
		return stats
	}

	stats = NewMountStats(path)
	c.mounts[path] = stats
	return stats
}

// RemoveMountStats removes stats for a mount
func (c *Collector) RemoveMountStats(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.mounts, path)
}

// AllMountStats returns a copy of all mount stats
func (c *Collector) AllMountStats() map[string]*MountStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*MountStats, len(c.mounts))
	for k, v := range c.mounts {
		result[k] = v
	}
	return result
}

// Start starts the stats collection
func (c *Collector) Start() {
	go c.collectLoop()
}

// Stop stops the stats collection
func (c *Collector) Stop() {
	close(c.done)
}

// collectLoop periodically collects stats
func (c *Collector) collectLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect aggregates current stats
func (c *Collector) collect() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalListeners int64
	for _, stats := range c.mounts {
		totalListeners += stats.GetListeners()
	}

	Global().SetCurrentListeners(totalListeners)
}

// FormatBytes formats bytes into a human-readable string
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return formatInt(bytes) + " B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatFloat(float64(bytes)/float64(div)) + " " + string("KMGTPE"[exp]) + "iB"
}

// formatInt formats an integer with thousands separators
func formatInt(n int64) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	if n < 1000 {
		return intToString(n)
	}
	return formatInt(n/1000) + "," + padLeft(intToString(n%1000), 3, '0')
}

// formatFloat formats a float to 2 decimal places
func formatFloat(f float64) string {
	return floatToString(f, 2)
}

// intToString converts an int64 to string
func intToString(n int64) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	return string(result)
}

// floatToString converts a float64 to string with specified precision
func floatToString(f float64, precision int) string {
	if f < 0 {
		return "-" + floatToString(-f, precision)
	}

	intPart := int64(f)
	result := intToString(intPart) + "."

	f -= float64(intPart)
	for i := 0; i < precision; i++ {
		f *= 10
		result += string(byte('0' + int(f)%10))
	}

	return result
}

// padLeft pads a string on the left
func padLeft(s string, length int, pad byte) string {
	for len(s) < length {
		s = string(pad) + s
	}
	return s
}

// FormatDuration formats a duration into a human-readable string
func FormatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return intToString(int64(days)) + "d " + intToString(int64(hours)) + "h " + intToString(int64(minutes)) + "m"
	}
	if hours > 0 {
		return intToString(int64(hours)) + "h " + intToString(int64(minutes)) + "m " + intToString(int64(seconds)) + "s"
	}
	if minutes > 0 {
		return intToString(int64(minutes)) + "m " + intToString(int64(seconds)) + "s"
	}
	return intToString(int64(seconds)) + "s"
}
