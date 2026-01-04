// Package stream handles audio stream management and distribution
package stream

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/google/uuid"
)

// STREAMING HOT PATH DESIGN:
// The following functions are called thousands of times per second and MUST be lock-free:
// - WriteData() - source pushing audio data
// - IsActive() - listener checking if source is connected
// - Buffer.ReadFromInto() - listener reading audio data
//
// We achieve this by using atomic operations instead of mutexes for the hot path.

var (
	ErrMountNotFound      = errors.New("mount point not found")
	ErrMountAlreadyExists = errors.New("mount point already exists")
	ErrNoSource           = errors.New("no source connected")
	ErrMaxListeners       = errors.New("maximum listeners reached")
	ErrSourceConnected    = errors.New("source already connected")
)

// Metadata represents stream metadata (ICY metadata)
type Metadata struct {
	Title       string
	Artist      string
	Album       string
	StreamTitle string
	URL         string
	Genre       string
	Bitrate     int
	ContentType string
	Description string
	Name        string
	Public      bool
	mu          sync.RWMutex
}

// GetStreamTitle returns the current stream title
func (m *Metadata) GetStreamTitle() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.StreamTitle
}

// SetStreamTitle sets the current stream title
func (m *Metadata) SetStreamTitle(title string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StreamTitle = title
}

// Clone returns a copy of the metadata
func (m *Metadata) Clone() *Metadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &Metadata{
		Title:       m.Title,
		Artist:      m.Artist,
		Album:       m.Album,
		StreamTitle: m.StreamTitle,
		URL:         m.URL,
		Genre:       m.Genre,
		Bitrate:     m.Bitrate,
		ContentType: m.ContentType,
		Description: m.Description,
		Name:        m.Name,
		Public:      m.Public,
	}
}

// Listener represents a connected listener
type Listener struct {
	ID          string
	IP          string
	UserAgent   string
	ConnectedAt time.Time
	BytesSent   int64
	LastActive  time.Time
	IsBot       bool // True if this is a known bot/preview fetcher
	done        chan struct{}
}

// NewListener creates a new listener with minimal info
func NewListener(ip, userAgent string) *Listener {
	return &Listener{
		ID:          uuid.New().String(),
		IP:          ip,
		UserAgent:   userAgent,
		ConnectedAt: time.Now(),
		LastActive:  time.Now(),
		IsBot:       false,
		done:        make(chan struct{}),
	}
}

// NewListenerWithBot creates a new listener with bot flag
func NewListenerWithBot(ip, userAgent string, isBot bool) *Listener {
	return &Listener{
		ID:          uuid.New().String(),
		IP:          ip,
		UserAgent:   userAgent,
		ConnectedAt: time.Now(),
		LastActive:  time.Now(),
		IsBot:       isBot,
		done:        make(chan struct{}),
	}
}

// Close closes the listener connection
func (l *Listener) Close() {
	select {
	case <-l.done:
		// Already closed
	default:
		close(l.done)
	}
}

// Done returns the done channel
func (l *Listener) Done() <-chan struct{} {
	return l.done
}

// Mount represents a stream mount point
type Mount struct {
	Path   string
	Config *config.MountConfig
	buffer *Buffer

	// HOT PATH: sourceActive is atomic for lock-free streaming
	// This is checked on EVERY audio chunk write and read - must be fast
	sourceActive atomic.Bool

	metadata            *Metadata
	listeners           map[string]*Listener
	listenerCount       int32
	sourceIP            string
	sourceID            string
	startTime           time.Time
	bytesReceived       int64
	peakListeners       int32        // Deprecated: raw connection peak
	peakUniqueListeners int32        // Peak unique listeners (by IP+UserAgent)
	mu                  sync.RWMutex // Protects sourceIP, sourceID, startTime (NOT sourceActive)
	listenerMu          sync.RWMutex // Protects listeners map
	configMu            sync.RWMutex // Protects Config
	fallbackMount       string
}

// NewMount creates a new mount point
func NewMount(path string, cfg *config.MountConfig, bufferSize, burstSize int) *Mount {
	if cfg == nil {
		cfg = &config.MountConfig{
			Name:         path,
			MaxListeners: 100,
			Type:         "audio/mpeg",
			Public:       true,
			BurstSize:    burstSize,
		}
	}

	return &Mount{
		Path:      path,
		Config:    cfg,
		buffer:    NewBuffer(bufferSize, cfg.BurstSize),
		metadata:  &Metadata{ContentType: cfg.Type},
		listeners: make(map[string]*Listener),
	}
}

// SetConfig updates the mount's configuration (for hot-reload support)
func (m *Mount) SetConfig(cfg *config.MountConfig) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.Config = cfg
}

// GetConfig returns the mount's current configuration with proper locking
func (m *Mount) GetConfig() *config.MountConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.Config
}

// StartSource starts a source connection
func (m *Mount) StartSource(sourceIP string) error {
	// Try to atomically set sourceActive from false to true
	if !m.sourceActive.CompareAndSwap(false, true) {
		return ErrSourceConnected
	}

	// Source is now active, set up the rest under lock
	m.mu.Lock()
	m.sourceIP = sourceIP
	m.sourceID = uuid.New().String()
	m.startTime = time.Now()
	atomic.StoreInt64(&m.bytesReceived, 0)
	m.mu.Unlock()

	m.buffer.Reset()
	return nil
}

// StopSource stops the source connection
func (m *Mount) StopSource() {
	// Atomically mark as inactive first (lock-free for hot path)
	m.sourceActive.Store(false)

	// Clear metadata under lock
	m.mu.Lock()
	m.sourceIP = ""
	m.sourceID = ""
	m.mu.Unlock()
}

// IsActive returns true if a source is connected
// HOT PATH: Lock-free atomic read - called on every streaming iteration
func (m *Mount) IsActive() bool {
	return m.sourceActive.Load()
}

// WriteData writes data from the source to the buffer
// HOT PATH: Completely lock-free - called ~40 times/second at 320kbps
// This is the most critical function for streaming performance
func (m *Mount) WriteData(data []byte) (int, error) {
	// Lock-free check using atomic
	if !m.sourceActive.Load() {
		return 0, ErrNoSource
	}

	n, err := m.buffer.Write(data)
	if err != nil {
		return n, err
	}

	atomic.AddInt64(&m.bytesReceived, int64(n))

	return n, nil
}

// CanAddListener checks if a new listener can be added
func (m *Mount) CanAddListener() bool {
	count := atomic.LoadInt32(&m.listenerCount)
	cfg := m.GetConfig()
	return int(count) < cfg.MaxListeners
}

// AddListener adds a new listener
func (m *Mount) AddListener(l *Listener) {
	m.listenerMu.Lock()
	defer m.listenerMu.Unlock()

	m.listeners[l.ID] = l
	atomic.AddInt32(&m.listenerCount, 1)

	// Update peak unique listeners (count unique IP+UserAgent combinations)
	m.updatePeakUnique()
}

// updatePeakUnique updates peak based on current unique listener count
// Must be called with listenerMu held
func (m *Mount) updatePeakUnique() {
	unique := make(map[string]struct{})
	for _, l := range m.listeners {
		key := l.IP + "|" + l.UserAgent
		unique[key] = struct{}{}
	}
	uniqueCount := int32(len(unique))

	for {
		peak := atomic.LoadInt32(&m.peakUniqueListeners)
		if uniqueCount <= peak {
			break
		}
		if atomic.CompareAndSwapInt32(&m.peakUniqueListeners, peak, uniqueCount) {
			break
		}
	}
}

// RemoveListener removes a listener by reference
func (m *Mount) RemoveListener(l *Listener) {
	m.listenerMu.Lock()
	defer m.listenerMu.Unlock()

	if _, exists := m.listeners[l.ID]; exists {
		l.Close()
		delete(m.listeners, l.ID)
		atomic.AddInt32(&m.listenerCount, -1)
	}
}

// RemoveListenerByID removes a listener by ID string (for admin API)
func (m *Mount) RemoveListenerByID(id string) {
	m.listenerMu.Lock()
	defer m.listenerMu.Unlock()

	if l, exists := m.listeners[id]; exists {
		l.Close()
		delete(m.listeners, id)
		atomic.AddInt32(&m.listenerCount, -1)
	}
}

// GetListener returns a listener by ID
func (m *Mount) GetListener(id string) *Listener {
	m.listenerMu.RLock()
	defer m.listenerMu.RUnlock()
	return m.listeners[id]
}

// ListenerCount returns the current number of listeners
func (m *Mount) ListenerCount() int {
	return int(atomic.LoadInt32(&m.listenerCount))
}

// PeakListeners returns the peak unique listener count
func (m *Mount) PeakListeners() int {
	return int(atomic.LoadInt32(&m.peakUniqueListeners))
}

// GetListeners returns a copy of all listeners
func (m *Mount) GetListeners() []*Listener {
	m.listenerMu.RLock()
	defer m.listenerMu.RUnlock()

	result := make([]*Listener, 0, len(m.listeners))
	for _, l := range m.listeners {
		result = append(result, l)
	}
	return result
}

// TotalBytesSent returns the total bytes sent to all current listeners
// OPTIMIZED: Uses atomic loads without holding lock for long
func (m *Mount) TotalBytesSent() int64 {
	m.listenerMu.RLock()
	// Get slice of pointers quickly, then release lock
	listeners := make([]*Listener, 0, len(m.listeners))
	for _, l := range m.listeners {
		listeners = append(listeners, l)
	}
	m.listenerMu.RUnlock()

	// Now sum bytes without holding any lock
	var total int64
	for _, l := range listeners {
		total += atomic.LoadInt64(&l.BytesSent)
	}
	return total
}

// UniqueListener represents a consolidated view of listeners from the same IP/UserAgent
type UniqueListener struct {
	IP          string
	UserAgent   string
	Connections int
	ConnectedAt time.Time // Earliest connection time
	BytesSent   int64     // Total bytes across all connections
	LastActive  time.Time // Most recent activity
	IDs         []string  // All listener IDs for this unique listener
	IsBot       bool      // True if this is a known bot/preview fetcher
}

// GetUniqueListeners returns listeners consolidated by IP+UserAgent
// This is useful for display purposes since browsers (especially Safari)
// often create multiple connections for a single user
// GetUniqueListeners returns consolidated view of listeners by IP+UserAgent
// OPTIMIZED: Minimizes lock hold time - copies data first, then processes
func (m *Mount) GetUniqueListeners() []*UniqueListener {
	// Snapshot listener data quickly under lock
	type listenerSnapshot struct {
		id          string
		ip          string
		userAgent   string
		connectedAt time.Time
		bytesSent   int64
		lastActive  time.Time
		isBot       bool
	}

	m.listenerMu.RLock()
	snapshots := make([]listenerSnapshot, 0, len(m.listeners))
	for _, l := range m.listeners {
		snapshots = append(snapshots, listenerSnapshot{
			id:          l.ID,
			ip:          l.IP,
			userAgent:   l.UserAgent,
			connectedAt: l.ConnectedAt,
			bytesSent:   atomic.LoadInt64(&l.BytesSent),
			lastActive:  l.LastActive,
			isBot:       l.IsBot,
		})
	}
	m.listenerMu.RUnlock()

	// Now do expensive consolidation without holding any lock
	unique := make(map[string]*UniqueListener)

	for _, l := range snapshots {
		key := l.ip + "|" + l.userAgent
		if ul, exists := unique[key]; exists {
			ul.Connections++
			ul.BytesSent += l.bytesSent
			ul.IDs = append(ul.IDs, l.id)
			if l.connectedAt.Before(ul.ConnectedAt) {
				ul.ConnectedAt = l.connectedAt
			}
			if l.lastActive.After(ul.LastActive) {
				ul.LastActive = l.lastActive
			}
		} else {
			unique[key] = &UniqueListener{
				IP:          l.ip,
				UserAgent:   l.userAgent,
				Connections: 1,
				ConnectedAt: l.connectedAt,
				BytesSent:   l.bytesSent,
				LastActive:  l.lastActive,
				IDs:         []string{l.id},
				IsBot:       l.isBot,
			}
		}
	}

	result := make([]*UniqueListener, 0, len(unique))
	for _, ul := range unique {
		result = append(result, ul)
	}
	return result
}

// UniqueListenerCount returns the count of unique IP+UserAgent combinations (excluding bots)
// OPTIMIZED: Minimizes lock hold time by copying data first
func (m *Mount) UniqueListenerCount() int {
	m.listenerMu.RLock()
	// Quick copy of essential data, then release lock
	type listenerKey struct {
		ip        string
		userAgent string
		isBot     bool
	}
	keys := make([]listenerKey, 0, len(m.listeners))
	for _, l := range m.listeners {
		keys = append(keys, listenerKey{l.IP, l.UserAgent, l.IsBot})
	}
	m.listenerMu.RUnlock()

	// Now do expensive deduplication without holding lock
	unique := make(map[string]struct{})
	for _, k := range keys {
		if k.isBot {
			continue
		}
		key := k.ip + "|" + k.userAgent
		unique[key] = struct{}{}
	}
	return len(unique)
}

// GetMetadata returns the current metadata
func (m *Mount) GetMetadata() *Metadata {
	return m.metadata.Clone()
}

// SetMetadata updates the stream metadata
func (m *Mount) SetMetadata(title string) {
	m.metadata.SetStreamTitle(title)
}

// UpdateMetadata updates multiple metadata fields
func (m *Mount) UpdateMetadata(meta *Metadata) {
	m.metadata.mu.Lock()
	defer m.metadata.mu.Unlock()

	if meta.Title != "" {
		m.metadata.Title = meta.Title
	}
	if meta.Artist != "" {
		m.metadata.Artist = meta.Artist
	}
	if meta.StreamTitle != "" {
		m.metadata.StreamTitle = meta.StreamTitle
	}
	if meta.Genre != "" {
		m.metadata.Genre = meta.Genre
	}
	if meta.URL != "" {
		m.metadata.URL = meta.URL
	}
	if meta.Description != "" {
		m.metadata.Description = meta.Description
	}
	if meta.Name != "" {
		m.metadata.Name = meta.Name
	}
	if meta.Bitrate > 0 {
		m.metadata.Bitrate = meta.Bitrate
	}
	if meta.ContentType != "" {
		m.metadata.ContentType = meta.ContentType
	}
}

// Buffer returns the stream buffer
func (m *Mount) Buffer() *Buffer {
	return m.buffer
}

// SetBurstSize updates the burst size for hot-reload support
func (m *Mount) SetBurstSize(size int) {
	if m.buffer != nil && size > 0 {
		m.buffer.SetBurstSize(size)
	}
}

// UpdateFromConfig updates mount settings from config for hot-reload
func (m *Mount) UpdateFromConfig(cfg *config.MountConfig) {
	m.SetConfig(cfg)
	if cfg.BurstSize > 0 {
		m.SetBurstSize(cfg.BurstSize)
	}
}

// Notify returns the notification channel for new data
// Now delegated to buffer for more efficient notification
func (m *Mount) Notify() <-chan struct{} {
	return m.buffer.NotifyChan()
}

// Stats returns mount statistics
// OPTIMIZED: Avoids nested locks by collecting data separately
// This prevents lock contention that was causing streaming lag
func (m *Mount) Stats() MountStats {
	// First, get listener stats WITHOUT holding mu lock
	// This is the expensive operation that was blocking streaming
	bytesSent := m.TotalBytesSent()
	uniqueCount := m.UniqueListenerCount()
	totalConns := m.ListenerCount()
	peakListeners := m.PeakListeners()

	// Get sourceActive atomically (lock-free)
	isActive := m.sourceActive.Load()

	// Now get mount-level data with brief lock
	m.mu.RLock()
	stats := MountStats{
		Path:             m.Path,
		Active:           isActive,
		SourceIP:         m.sourceIP,
		StartTime:        m.startTime,
		BytesReceived:    atomic.LoadInt64(&m.bytesReceived),
		BytesSent:        bytesSent,
		Listeners:        uniqueCount,
		TotalConnections: totalConns,
		PeakListeners:    peakListeners,
		ContentType:      m.metadata.ContentType,
		Metadata:         m.metadata.Clone(),
	}
	m.mu.RUnlock()

	return stats
}

// MountStats contains mount point statistics
type MountStats struct {
	Path             string
	Active           bool
	SourceIP         string
	StartTime        time.Time
	BytesReceived    int64
	BytesSent        int64 // Total bytes sent to all listeners
	Listeners        int   // Unique listeners (by IP+UserAgent)
	TotalConnections int   // Raw TCP connection count
	PeakListeners    int
	ContentType      string
	Metadata         *Metadata
}

// MountManager manages all mount points
type MountManager struct {
	mounts    map[string]*Mount
	mu        sync.RWMutex
	config    *config.Config
	maxMounts int
	logger    func(format string, v ...interface{})
}

// NewMountManager creates a new mount manager
func NewMountManager(cfg *config.Config) *MountManager {
	mm := &MountManager{
		mounts:    make(map[string]*Mount),
		config:    cfg,
		maxMounts: cfg.Limits.MaxSources,
		logger:    func(format string, v ...interface{}) {}, // no-op by default
	}

	// Pre-create mounts from configuration
	for path, mountCfg := range cfg.Mounts {
		mm.mounts[path] = NewMount(path, mountCfg, cfg.Limits.QueueSize, cfg.Limits.BurstSize)
	}

	return mm
}

// SetConfig updates the mount manager's configuration (for hot-reload support)
// This updates the config reference and syncs mount configurations
func (mm *MountManager) SetConfig(cfg *config.Config) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.config = cfg
	mm.maxMounts = cfg.Limits.MaxSources

	// Update existing mount configs and create new ones from config
	for path, mountCfg := range cfg.Mounts {
		if mount, exists := mm.mounts[path]; exists {
			// Update existing mount's config with full hot-reload support
			// This updates config AND burst size
			mount.UpdateFromConfig(mountCfg)
			mm.logger("[HotReload] Updated mount %s config", path)
		} else {
			// Create new mount from config
			mm.mounts[path] = NewMount(path, mountCfg, cfg.Limits.QueueSize, cfg.Limits.BurstSize)
			mm.logger("[HotReload] Created new mount %s", path)
		}
	}

	// Remove mounts that are no longer in config (but only if they're not active)
	for path, mount := range mm.mounts {
		if _, exists := cfg.Mounts[path]; !exists {
			// Only remove if no active source - don't interrupt live streams
			if !mount.IsActive() && mount.ListenerCount() == 0 {
				delete(mm.mounts, path)
			}
		}
	}
}

// SetLogger sets the logger function for the mount manager
func (mm *MountManager) SetLogger(logger func(format string, v ...interface{})) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.logger = logger
}

// GetMount returns a mount point by path
func (mm *MountManager) GetMount(path string) *Mount {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.mounts[path]
}

// GetOrCreateMount returns an existing mount or creates a new one
func (mm *MountManager) GetOrCreateMount(path string) (*Mount, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mount, exists := mm.mounts[path]; exists {
		return mount, nil
	}

	if len(mm.mounts) >= mm.maxMounts {
		return nil, fmt.Errorf("maximum number of mounts (%d) reached", mm.maxMounts)
	}

	// Get mount-specific config or use defaults
	mountCfg := mm.config.GetMountConfig(path)
	mount := NewMount(path, mountCfg, mm.config.Limits.QueueSize, mm.config.Limits.BurstSize)
	mm.mounts[path] = mount

	return mount, nil
}

// RemoveMount removes a mount point
func (mm *MountManager) RemoveMount(path string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mount, exists := mm.mounts[path]
	if !exists {
		return ErrMountNotFound
	}

	// Disconnect all listeners
	for _, l := range mount.GetListeners() {
		l.Close()
	}

	// Stop source if active
	mount.StopSource()

	delete(mm.mounts, path)
	return nil
}

// ListMounts returns all mount paths
func (mm *MountManager) ListMounts() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	paths := make([]string, 0, len(mm.mounts))
	for path := range mm.mounts {
		paths = append(paths, path)
	}
	return paths
}

// GetAllMounts returns all mounts
func (mm *MountManager) GetAllMounts() []*Mount {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	mounts := make([]*Mount, 0, len(mm.mounts))
	for _, mount := range mm.mounts {
		mounts = append(mounts, mount)
	}
	return mounts
}

// GetActiveMounts returns all active mounts (with connected sources)
func (mm *MountManager) GetActiveMounts() []*Mount {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	mounts := make([]*Mount, 0)
	for _, mount := range mm.mounts {
		if mount.IsActive() {
			mounts = append(mounts, mount)
		}
	}
	return mounts
}

// Stats returns statistics for all mounts
// OPTIMIZED: Minimizes lock hold time to prevent blocking streaming
// Previously held mm.mu.RLock while calling mount.Stats() for every mount,
// which caused cascading lock contention and streaming lag
func (mm *MountManager) Stats() []MountStats {
	// Quickly copy mount pointers under lock
	mm.mu.RLock()
	mounts := make([]*Mount, 0, len(mm.mounts))
	for _, mount := range mm.mounts {
		mounts = append(mounts, mount)
	}
	mm.mu.RUnlock()

	// Now collect stats WITHOUT holding MountManager lock
	// This allows streaming writes to proceed unblocked
	stats := make([]MountStats, 0, len(mounts))
	for _, mount := range mounts {
		stats = append(stats, mount.Stats())
	}
	return stats
}

// TotalListeners returns the total number of listeners across all mounts
// OPTIMIZED: Copies mount pointers first to minimize lock time
func (mm *MountManager) TotalListeners() int {
	mm.mu.RLock()
	mounts := make([]*Mount, 0, len(mm.mounts))
	for _, mount := range mm.mounts {
		mounts = append(mounts, mount)
	}
	mm.mu.RUnlock()

	total := 0
	for _, mount := range mounts {
		total += mount.ListenerCount()
	}
	return total
}

// TotalBytesSent returns the total bytes sent across all mounts
// OPTIMIZED: Copies mount pointers first to minimize lock time
func (mm *MountManager) TotalBytesSent() int64 {
	mm.mu.RLock()
	mounts := make([]*Mount, 0, len(mm.mounts))
	for _, mount := range mm.mounts {
		mounts = append(mounts, mount)
	}
	mm.mu.RUnlock()

	var total int64
	for _, mount := range mounts {
		total += mount.TotalBytesSent()
	}
	return total
}
