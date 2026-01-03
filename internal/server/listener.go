// Package server handles HTTP server and listener connections
// Robust, high-performance streaming with automatic recovery
package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/stream"
)

// Version of GoCast server
var Version = "dev"

// =============================================================================
// STREAMING CONSTANTS - Defaults that can be overridden by config
// =============================================================================
//
// These are fallback values. The actual values come from:
// - Global config: config.Limits.BurstSize, config.Limits.QueueSize
// - Mount config: mount.Config.BurstSize, mount.Config.Bitrate
//
// All values are hot-reloadable via the admin panel.
const (
	// streamChunkSize: Read buffer size (not configurable - internal optimization)
	// 16KB allows efficient reads from the ring buffer
	streamChunkSize = 16384

	// sourceReconnectWait: How long listeners wait for source to reconnect
	sourceReconnectWait = 30 * time.Second

	// icyMetaInterval: Standard Icecast metadata interval
	icyMetaInterval = 16000

	// defaultClientTimeout: Fallback if not in config
	defaultClientTimeout = 120 * time.Second

	// defaultBurstSize: Fallback burst size if not in config
	// 128KB = ~3.2 seconds at 320kbps - bulletproof!
	defaultBurstSize = 131072

	// defaultBitrate: Fallback bitrate if not in config (bits per second)
	defaultBitrate = 320000
)

// botUserAgents contains patterns for known bots/preview fetchers
// These connections will be tracked but marked as bots
var botUserAgents = []string{
	"WhatsApp",
	"facebookexternalhit",
	"Facebot",
	"Twitterbot",
	"LinkedInBot",
	"Slackbot",
	"TelegramBot",
	"Discordbot",
	"Googlebot",
	"bingbot",
	"YandexBot",
	"DuckDuckBot",
	"Baiduspider",
	"curl",
	"wget",
	"python-requests",
	"Go-http-client",
	"Apache-HttpClient",
	"Java/",
	"okhttp",
}

// isBotUserAgent checks if the user agent belongs to a known bot/preview fetcher
func isBotUserAgent(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	for _, bot := range botUserAgents {
		if strings.Contains(ua, strings.ToLower(bot)) {
			return true
		}
	}
	return false
}

// ListenerHandler handles listener HTTP requests
type ListenerHandler struct {
	mountManager   *stream.MountManager
	config         *config.Config
	logger         *log.Logger
	activityBuffer *ActivityBuffer
	mu             sync.RWMutex

	// Buffer pool to reduce allocations
	bufPool sync.Pool
}

// NewListenerHandler creates a new listener handler
func NewListenerHandler(mm *stream.MountManager, cfg *config.Config, logger *log.Logger) *ListenerHandler {
	return NewListenerHandlerWithActivity(mm, cfg, logger, nil)
}

// NewListenerHandlerWithActivity creates a listener handler with activity logging
func NewListenerHandlerWithActivity(mm *stream.MountManager, cfg *config.Config, logger *log.Logger, ab *ActivityBuffer) *ListenerHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &ListenerHandler{
		mountManager:   mm,
		config:         cfg,
		logger:         logger,
		activityBuffer: ab,
		bufPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, streamChunkSize)
				return &buf
			},
		},
	}
}

// SetConfig updates the handler's configuration (for hot-reload support)
func (h *ListenerHandler) SetConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
	h.logger.Println("Listener handler configuration updated")
}

// getConfig returns the current config with proper locking
func (h *ListenerHandler) getConfig() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// ServeHTTP handles listener GET requests
func (h *ListenerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Path
	if mountPath == "" {
		mountPath = "/"
	}

	clientIP := getClientIP(r)
	userAgent := r.UserAgent()

	// Get mount
	mount := h.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	// Handle HEAD requests separately - just return headers, no listener creation
	if r.Method == http.MethodHead {
		h.HandleHead(w, r, mount)
		return
	}

	// Check if this is a bot/preview request
	isBot := isBotUserAgent(userAgent)

	// Check if we can add listener (bots don't count toward limit)
	if !isBot && !mount.CanAddListener() {
		http.Error(w, "Listener limit reached", http.StatusServiceUnavailable)
		return
	}

	// Check IP restrictions
	if !h.checkIPAllowed(r, mount) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Create listener with bot flag
	listener := stream.NewListenerWithBot(clientIP, userAgent, isBot)
	mount.AddListener(listener)
	connectTime := time.Now()

	// Log listener connect
	if h.activityBuffer != nil {
		h.activityBuffer.ListenerConnected(mountPath, clientIP, r.UserAgent())
	}

	defer func() {
		mount.RemoveListener(listener)
		// Log listener disconnect
		if h.activityBuffer != nil {
			h.activityBuffer.ListenerDisconnected(mountPath, clientIP, time.Since(connectTime))
		}
	}()

	// Check for ICY metadata request
	metadataInterval := 0
	if r.Header.Get("Icy-MetaData") == "1" {
		metadataInterval = icyMetaInterval
	}

	// Set response headers
	h.setHeaders(w, mount, metadataInterval)

	// Get flusher for streaming
	flusher, hasFlusher := w.(http.Flusher)
	if hasFlusher {
		flusher.Flush()
	}

	// Stream audio to client
	h.streamToClient(w, flusher, hasFlusher, listener, mount, metadataInterval)
}

// HandleHead handles HEAD requests - returns headers without creating a listener
// Browsers often send HEAD requests to probe the stream before connecting
func (h *ListenerHandler) HandleHead(w http.ResponseWriter, r *http.Request, mount *stream.Mount) {
	meta := mount.GetMetadata()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Server", "GoCast/"+Version)

	// ICY headers
	if meta.Name != "" {
		w.Header().Set("icy-name", meta.Name)
	}
	if meta.Genre != "" {
		w.Header().Set("icy-genre", meta.Genre)
	}
	if meta.Bitrate > 0 {
		w.Header().Set("icy-br", strconv.Itoa(meta.Bitrate))
	}
	w.Header().Set("icy-pub", "1")

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")

	w.WriteHeader(http.StatusOK)
}

// setHeaders sets HTTP response headers for streaming
func (h *ListenerHandler) setHeaders(w http.ResponseWriter, mount *stream.Mount, metaInterval int) {
	meta := mount.GetMetadata()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Server", "GoCast/"+Version)

	// ICY headers
	if meta.Name != "" {
		w.Header().Set("icy-name", meta.Name)
	}
	if meta.Genre != "" {
		w.Header().Set("icy-genre", meta.Genre)
	}
	if meta.Bitrate > 0 {
		w.Header().Set("icy-br", strconv.Itoa(meta.Bitrate))
	}
	w.Header().Set("icy-pub", "1")
	if metaInterval > 0 {
		w.Header().Set("icy-metaint", strconv.Itoa(metaInterval))
	}

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")

	w.WriteHeader(http.StatusOK)
}

// streamToClient implements BULLETPROOF audio streaming.
//
// This is the heart of GoCast. It ensures smooth, uninterrupted audio by:
//
// 1. INITIAL BURST - Send available audio immediately to fill player buffer
// 2. IMMEDIATE PASS-THROUGH - Send data as soon as it arrives from source
// 3. AGGRESSIVE FLUSHING - Flush after every write to minimize latency
//
// The large initial burst (64KB = ~1.6 seconds) gives the player enough
// buffer to absorb any network jitter. After that, we pass data through
// as fast as it arrives - the source controls the rate.
func (h *ListenerHandler) streamToClient(w http.ResponseWriter, flusher http.Flusher, hasFlusher bool, listener *stream.Listener, mount *stream.Mount, metaInterval int) {
	buffer := mount.Buffer()
	if buffer == nil {
		return
	}

	// Create our stream writer
	sw := NewStreamWriter(w)
	defer sw.Close()

	// Wait for source if not active
	if !mount.IsActive() {
		if !h.waitForSource(mount, listener) {
			return
		}
	}

	// Get read buffer from pool
	bufPtr := h.bufPool.Get().(*[]byte)
	readBuf := *bufPtr
	defer h.bufPool.Put(bufPtr)

	// ==========================================================================
	// CONFIGURATION - All values from config, hot-reloadable
	// ==========================================================================

	// Get current config (supports hot-reload)
	cfg := h.getConfig()

	// Get burst size: mount-specific > global > default
	burstSize := mount.Config.BurstSize
	if burstSize <= 0 {
		burstSize = cfg.Limits.BurstSize
	}
	if burstSize <= 0 {
		burstSize = defaultBurstSize
	}

	// Get bitrate from mount config (kbps -> bps)
	bitrate := mount.Config.Bitrate * 1000
	if bitrate <= 0 {
		bitrate = defaultBitrate
	}
	_ = bitrate // Used for future rate control if needed

	// ==========================================================================
	// PHASE 1: INITIAL BURST - Fill the player's buffer
	// ==========================================================================

	// Start reading from behind the live position to get burst data
	writePos := buffer.WritePos()
	burstAvailable := int64(burstSize)
	if burstAvailable > writePos {
		burstAvailable = writePos
	}

	readPos := writePos - burstAvailable
	if readPos < 0 {
		readPos = 0
	}

	// Send initial burst as fast as possible
	burstSent := int64(0)
	needsSync := true

	for burstSent < burstAvailable {
		// Check for disconnect
		select {
		case <-listener.Done():
			return
		default:
		}

		n, newPos := buffer.ReadFromInto(readPos, readBuf)
		if n == 0 {
			break
		}

		data := readBuf[:n]
		readPos = newPos

		// Find MP3 frame sync on first chunk
		if needsSync && n >= 4 {
			if syncOffset := findMP3Sync(data); syncOffset > 0 && syncOffset < n-4 {
				data = data[syncOffset:]
			}
			needsSync = false
		}

		if len(data) == 0 {
			continue
		}

		// Write burst data (no metadata during burst - just raw audio)
		var err error
		_, err = sw.Write(data)

		if err != nil {
			return
		}

		burstSent += int64(len(data))
		atomic.AddInt64(&listener.BytesSent, int64(len(data)))
	}

	// Flush the burst
	if hasFlusher {
		flusher.Flush()
	}

	// ==========================================================================
	// PHASE 2: IMMEDIATE PASS-THROUGH STREAMING
	// ==========================================================================
	//
	// After the initial burst, we simply pass data through as it arrives.
	// The source (RadioBOSS) sends at the correct bitrate.
	// Our job is to forward it immediately with minimal latency.

	// Track source state
	var sourceDisconnectTime time.Time
	sourceWasActive := true

	var metaByteCount int
	var lastMeta string

	for {
		// Check for client disconnect
		select {
		case <-listener.Done():
			return
		default:
		}

		// Check source status
		sourceActive := mount.IsActive()
		if !sourceActive && sourceWasActive {
			sourceDisconnectTime = time.Now()
			sourceWasActive = false
		} else if sourceActive && !sourceWasActive {
			sourceWasActive = true
		}

		if !sourceActive && time.Since(sourceDisconnectTime) > sourceReconnectWait {
			return
		}

		// Read data from buffer
		n, newPos := buffer.ReadFromInto(readPos, readBuf)
		if n == 0 {
			// No data - wait briefly then try again
			// Use simple sleep instead of complex wait mechanisms
			time.Sleep(time.Millisecond)
			continue
		}

		data := readBuf[:n]
		readPos = newPos

		// Write data immediately
		var err error
		if metaInterval > 0 {
			err = writeDataWithMeta(w, data, mount, &metaByteCount, &lastMeta, metaInterval)
		} else {
			_, err = sw.Write(data)
		}

		if err != nil {
			return
		}

		atomic.AddInt64(&listener.BytesSent, int64(len(data)))

		// Flush immediately after every write
		if hasFlusher {
			flusher.Flush()
		}
	}
}

// waitForSource waits for a source to connect, returns false if we should give up
func (h *ListenerHandler) waitForSource(mount *stream.Mount, listener *stream.Listener) bool {
	waitStart := time.Now()
	for !mount.IsActive() {
		if time.Since(waitStart) > sourceReconnectWait {
			return false
		}
		select {
		case <-listener.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return true
}

// findMP3Sync finds the first valid MP3 frame sync in data
// Returns offset to frame start, or 0 if not found
func findMP3Sync(data []byte) int {
	if len(data) < 4 {
		return 0
	}

	for i := 0; i < len(data)-3; i++ {
		if data[i] != 0xFF {
			continue
		}

		b1 := data[i+1]

		// Check frame sync: 0xFF followed by 0xE0-0xFE
		if b1 == 0xFF || (b1&0xE0) != 0xE0 {
			continue
		}

		// Validate MPEG version and layer
		version := (b1 >> 3) & 0x03
		layer := (b1 >> 1) & 0x03
		if version == 0x01 || layer == 0x00 {
			continue // Reserved values
		}

		// Validate bitrate and sample rate
		b2 := data[i+2]
		bitrate := (b2 >> 4) & 0x0F
		sampleRate := (b2 >> 2) & 0x03
		if bitrate == 0x0F || sampleRate == 0x03 {
			continue // Invalid values
		}

		return i // Valid frame found
	}

	return 0
}

// writeDataWithMeta writes data with ICY metadata interleaved
func writeDataWithMeta(w io.Writer, data []byte, mount *stream.Mount, byteCount *int, lastMeta *string, interval int) error {
	remaining := data

	for len(remaining) > 0 {
		bytesUntilMeta := interval - *byteCount

		if bytesUntilMeta <= 0 {
			// Send metadata block
			meta := mount.GetMetadata()
			title := formatTitle(meta.Title, meta.Artist)

			if title != *lastMeta {
				if err := sendMetaBlock(w, title); err != nil {
					return err
				}
				*lastMeta = title
			} else {
				// Empty metadata block
				if _, err := w.Write([]byte{0}); err != nil {
					return err
				}
			}
			*byteCount = 0
			bytesUntilMeta = interval
		}

		// Write audio data
		toWrite := len(remaining)
		if toWrite > bytesUntilMeta {
			toWrite = bytesUntilMeta
		}

		n, err := w.Write(remaining[:toWrite])
		if err != nil {
			return err
		}
		*byteCount += n
		remaining = remaining[toWrite:]
	}

	return nil
}

// formatTitle formats metadata for streaming
func formatTitle(title, artist string) string {
	if artist != "" && title != "" {
		return artist + " - " + title
	}
	if title != "" {
		return title
	}
	return artist
}

// sendMetaBlock sends an ICY metadata block
func sendMetaBlock(w io.Writer, title string) error {
	if title == "" {
		_, err := w.Write([]byte{0})
		return err
	}

	// Format: StreamTitle='...';
	meta := fmt.Sprintf("StreamTitle='%s';", escapeMeta(title))

	// Calculate block count (16-byte blocks)
	blocks := (len(meta) + 15) / 16
	if blocks > 255 {
		blocks = 255
		meta = meta[:255*16]
	}

	// Pad to block boundary
	padded := make([]byte, 1+blocks*16)
	padded[0] = byte(blocks)
	copy(padded[1:], meta)

	_, err := w.Write(padded)
	return err
}

// escapeMeta escapes metadata characters
func escapeMeta(s string) string {
	s = strings.ReplaceAll(s, "'", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// checkIPAllowed checks IP restrictions
func (h *ListenerHandler) checkIPAllowed(r *http.Request, mount *stream.Mount) bool {
	if len(mount.Config.AllowedIPs) == 0 {
		return true
	}

	clientIP := getClientIP(r)
	for _, allowed := range mount.Config.AllowedIPs {
		if matchIP(clientIP, allowed) {
			return true
		}
	}
	return false
}

// matchIP matches IP against pattern
func matchIP(clientIP, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(clientIP, prefix)
	}
	return clientIP == pattern
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// HandleOptions handles CORS preflight requests
func (h *ListenerHandler) HandleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// ========== Status Handler ==========

// StatusHandler handles status page requests
type StatusHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
	startTime    time.Time
	version      string
	mu           sync.RWMutex
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(mm *stream.MountManager, cfg *config.Config) *StatusHandler {
	return &StatusHandler{mountManager: mm, config: cfg, startTime: time.Now(), version: "1.0.0"}
}

// NewStatusHandlerWithInfo creates a new status handler with server info
func NewStatusHandlerWithInfo(mm *stream.MountManager, cfg *config.Config, startTime time.Time, version string) *StatusHandler {
	return &StatusHandler{mountManager: mm, config: cfg, startTime: startTime, version: version}
}

// SetConfig updates the handler's configuration (for hot-reload support)
func (h *StatusHandler) SetConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
}

// getConfig returns the current config with proper locking
func (h *StatusHandler) getConfig() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// ServeHTTP serves the status page
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check query parameter first, then Accept header
	format := r.URL.Query().Get("format")
	accept := r.Header.Get("Accept")

	switch {
	case format == "json" || strings.Contains(accept, "application/json"):
		h.serveJSON(w)
	case format == "xml" || strings.Contains(accept, "text/xml") || strings.Contains(accept, "application/xml"):
		h.serveXML(w)
	default:
		h.serveHTML(w)
	}
}

func (h *StatusHandler) serveJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cfg := h.getConfig()
	mounts := h.mountManager.ListMounts()
	var sb strings.Builder

	// Server info
	uptime := int64(time.Since(h.startTime).Seconds())
	serverID := cfg.Server.ServerID
	if serverID == "" {
		serverID = "GoCast"
	}

	// Calculate total bytes sent and bandwidth
	totalBytesSent := h.mountManager.TotalBytesSent()
	var bytesPerSec int64
	if uptime > 0 {
		bytesPerSec = totalBytesSent / uptime
	}

	sb.WriteString(fmt.Sprintf(`{"server_id":%q,"version":%q,"started":%q,"uptime":%d,"host":%q,"total_bytes_sent":%d,"bytes_per_sec":%d,"mounts":[`,
		serverID, h.version, h.startTime.Format(time.RFC3339), uptime, cfg.Server.Hostname, totalBytesSent, bytesPerSec))

	for i, mountPath := range mounts {
		if i > 0 {
			sb.WriteString(",")
		}
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		meta := mount.GetMetadata()

		// Include metadata object with stream_title for admin panel, plus bytes_sent
		sb.WriteString(fmt.Sprintf(`{"path":%q,"active":%t,"listeners":%d,"peak":%d,"bytes_sent":%d,"name":%q,"genre":%q,"bitrate":%d,"metadata":{"stream_title":%q,"artist":%q,"title":%q}}`,
			mountPath, mount.IsActive(), stats.Listeners, stats.PeakListeners, stats.BytesSent,
			escapeJSON(meta.Name), escapeJSON(meta.Genre), meta.Bitrate,
			escapeJSON(meta.StreamTitle), escapeJSON(meta.Artist), escapeJSON(meta.Title)))
	}

	sb.WriteString("]}")
	w.Write([]byte(sb.String()))
}

func (h *StatusHandler) serveXML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/xml")

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><icestats>`)

	for _, mountPath := range h.mountManager.ListMounts() {
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		meta := mount.GetMetadata()

		sb.WriteString(fmt.Sprintf(`<source mount="%s"><listeners>%d</listeners><peak>%d</peak><name>%s</name></source>`,
			escapeXML(mountPath), stats.Listeners, stats.PeakListeners, escapeXML(meta.Name)))
	}

	sb.WriteString("</icestats>")
	w.Write([]byte(sb.String()))
}

func (h *StatusHandler) serveHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><title>GoCast</title>
<style>body{font-family:system-ui;margin:40px;background:#111;color:#eee}
h1{color:#00ADD8}.mount{background:#222;padding:20px;margin:10px 0;border-radius:8px}
.live{color:#4f4}.offline{color:#f44}</style></head>
<body><h1>🎵 GoCast Status</h1>`)

	for _, mountPath := range h.mountManager.ListMounts() {
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		status := "Offline"
		statusClass := "offline"
		if mount.IsActive() {
			status = "Live"
			statusClass = "live"
		}

		sb.WriteString(fmt.Sprintf(`<div class="mount"><h2>%s <span class="%s">● %s</span></h2>
<p>Listeners: <strong>%d</strong> (peak: %d)</p>
<p>Listen: <a href="%s">%s</a></p></div>`,
			mountPath, statusClass, status, stats.Listeners, stats.PeakListeners, mountPath, mountPath))
	}

	sb.WriteString("</body></html>")
	w.Write([]byte(sb.String()))
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
