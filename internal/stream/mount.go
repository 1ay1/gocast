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
	Path          string
	Config        *config.MountConfig
	buffer        *Buffer
	metadata      *Metadata
	listeners     map[string]*Listener
	listenerCount int32
	sourceActive  bool
	sourceIP      string
	sourceID      string
	startTime     time.Time
	bytesReceived int64
	peakListeners int32
	mu            sync.RWMutex
	listenerMu    sync.RWMutex
	fallbackMount string
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

// StartSource starts a source connection
func (m *Mount) StartSource(sourceIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sourceActive {
		return ErrSourceConnected
	}

	m.sourceActive = true
	m.sourceIP = sourceIP
	m.sourceID = uuid.New().String()
	m.startTime = time.Now()
	m.bytesReceived = 0
	m.buffer.Reset()

	return nil
}

// StopSource stops the source connection
func (m *Mount) StopSource() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sourceActive = false
	m.sourceIP = ""
	m.sourceID = ""
}

// IsActive returns true if a source is connected
func (m *Mount) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sourceActive
}

// WriteData writes data from the source to the buffer
// Optimized: buffer handles its own notification now
func (m *Mount) WriteData(data []byte) (int, error) {
	m.mu.RLock()
	if !m.sourceActive {
		m.mu.RUnlock()
		return 0, ErrNoSource
	}
	m.mu.RUnlock()

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
	return int(count) < m.Config.MaxListeners
}

// AddListener adds a new listener
func (m *Mount) AddListener(l *Listener) {
	m.listenerMu.Lock()
	defer m.listenerMu.Unlock()

	m.listeners[l.ID] = l
	newCount := atomic.AddInt32(&m.listenerCount, 1)

	// Update peak listeners
	for {
		peak := atomic.LoadInt32(&m.peakListeners)
		if newCount <= peak {
			break
		}
		if atomic.CompareAndSwapInt32(&m.peakListeners, peak, newCount) {
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

// PeakListeners returns the peak listener count
func (m *Mount) PeakListeners() int {
	return int(atomic.LoadInt32(&m.peakListeners))
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

// Notify returns the notification channel for new data
// Now delegated to buffer for more efficient notification
func (m *Mount) Notify() <-chan struct{} {
	return m.buffer.NotifyChan()
}

// Stats returns mount statistics
func (m *Mount) Stats() MountStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MountStats{
		Path:          m.Path,
		Active:        m.sourceActive,
		SourceIP:      m.sourceIP,
		StartTime:     m.startTime,
		BytesReceived: atomic.LoadInt64(&m.bytesReceived),
		Listeners:     m.ListenerCount(),
		PeakListeners: m.PeakListeners(),
		ContentType:   m.metadata.ContentType,
		Metadata:      m.metadata.Clone(),
	}
}

// MountStats contains mount point statistics
type MountStats struct {
	Path          string
	Active        bool
	SourceIP      string
	StartTime     time.Time
	BytesReceived int64
	Listeners     int
	PeakListeners int
	ContentType   string
	Metadata      *Metadata
}

// MountManager manages all mount points
type MountManager struct {
	mounts    map[string]*Mount
	mu        sync.RWMutex
	config    *config.Config
	maxMounts int
}

// NewMountManager creates a new mount manager
func NewMountManager(cfg *config.Config) *MountManager {
	mm := &MountManager{
		mounts:    make(map[string]*Mount),
		config:    cfg,
		maxMounts: cfg.Limits.MaxSources,
	}

	// Pre-create mounts from configuration
	for path, mountCfg := range cfg.Mounts {
		mm.mounts[path] = NewMount(path, mountCfg, cfg.Limits.QueueSize, cfg.Limits.BurstSize)
	}

	return mm
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
func (mm *MountManager) Stats() []MountStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	stats := make([]MountStats, 0, len(mm.mounts))
	for _, mount := range mm.mounts {
		stats = append(stats, mount.Stats())
	}
	return stats
}

// TotalListeners returns the total number of listeners across all mounts
func (mm *MountManager) TotalListeners() int {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	total := 0
	for _, mount := range mm.mounts {
		total += mount.ListenerCount()
	}
	return total
}
