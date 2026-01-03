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
const (
	// streamChunkSize: Read buffer size (not configurable - internal optimization)
	streamChunkSize = 16384

	// sourceReconnectWait: How long listeners wait for source to reconnect
	sourceReconnectWait = 30 * time.Second

	// icyMetaInterval: Standard Icecast metadata interval
	icyMetaInterval = 16000

	// defaultClientTimeout: Fallback if not in config
	defaultClientTimeout = 120 * time.Second

	// defaultBurstSize: Fallback burst size if not in config
	// 64KB = ~1.6 seconds at 320kbps
	defaultBurstSize = 65536

	// defaultBitrate: Fallback bitrate if not in config (bits per second)
	defaultBitrate = 320000
)

// botUserAgents contains patterns for known bots/preview fetchers
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

	// Buffer pool for streaming
	bufPool sync.Pool
}

// NewListenerHandler creates a new listener handler
func NewListenerHandler(mm *stream.MountManager, cfg *config.Config, logger *log.Logger) *ListenerHandler {
	return NewListenerHandlerWithActivity(mm, cfg, logger, nil)
}

// NewListenerHandlerWithActivity creates a new listener handler with activity tracking
func NewListenerHandlerWithActivity(mm *stream.MountManager, cfg *config.Config, logger *log.Logger, activityBuffer *ActivityBuffer) *ListenerHandler {
	return &ListenerHandler{
		mountManager:   mm,
		config:         cfg,
		logger:         logger,
		activityBuffer: activityBuffer,
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
}

// getConfig returns the current config with proper locking
func (h *ListenerHandler) getConfig() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// ServeHTTP handles incoming listener requests
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

	// Handle HEAD requests separately
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
func (h *ListenerHandler) HandleHead(w http.ResponseWriter, r *http.Request, mount *stream.Mount) {
	meta := mount.GetMetadata()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Server", "GoCast/"+Version)
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("X-Content-Type-Options", "nosniff")

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
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData, Range")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Type, icy-br, icy-name, icy-genre")

	w.WriteHeader(http.StatusOK)
}

// setHeaders sets HTTP response headers for streaming
func (h *ListenerHandler) setHeaders(w http.ResponseWriter, mount *stream.Mount, metaInterval int) {
	meta := mount.GetMetadata()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Server", "GoCast/"+Version)
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("X-Content-Type-Options", "nosniff")

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
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData, Range")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Type, icy-br, icy-name, icy-genre, icy-metaint")

	w.WriteHeader(http.StatusOK)
}

// streamToClient implements audio streaming to a listener
func (h *ListenerHandler) streamToClient(w http.ResponseWriter, flusher http.Flusher, hasFlusher bool, listener *stream.Listener, mount *stream.Mount, metaInterval int) {
	buffer := mount.Buffer()
	if buffer == nil {
		return
	}

	// Track start time for disconnect summary
	startTime := time.Now()

	// Create our stream writer - handles all writes and flushes
	sw := NewStreamWriter(w)
	defer sw.Close()

	// Initial flush to send headers immediately
	sw.Flush()

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

	// Get current config
	cfg := h.getConfig()

	// Get burst size: mount-specific > global > default
	burstSize := mount.Config.BurstSize
	if burstSize <= 0 {
		burstSize = cfg.Limits.BurstSize
	}
	if burstSize <= 0 {
		burstSize = defaultBurstSize
	}

	// ==========================================================================
	// PHASE 1: INITIAL BURST - Fill the player's buffer
	// ==========================================================================

	writePos := buffer.WritePos()
	burstAvailable := int64(burstSize)
	if burstAvailable > writePos {
		burstAvailable = writePos
	}

	readPos := writePos - burstAvailable
	if readPos < 0 {
		readPos = 0
	}

	// Send initial burst - use SafeReadFromInto to detect any skipped bytes
	// IMPORTANT: Must handle ICY metadata in burst phase too, otherwise protocol gets out of sync!
	burstSent := int64(0)
	needsSync := true
	totalSkipped := int64(0)

	// Initialize metadata tracking for burst phase (same variables used in real-time phase)
	var metaByteCount int
	var lastMeta string

	for burstSent < burstAvailable {
		select {
		case <-listener.Done():
			return
		default:
		}

		n, newPos, skipped := buffer.SafeReadFromInto(readPos, readBuf)
		if skipped > 0 {
			totalSkipped += skipped
			h.logger.Printf("WARNING: Listener %s skipped %d bytes during burst (total: %d)", listener.ID, skipped, totalSkipped)
		}
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

		// CRITICAL FIX: Handle ICY metadata in burst phase too!
		// Otherwise client expects metadata every 16KB but we send raw audio,
		// causing complete protocol desync and audio garbage/skipping
		var err error
		if metaInterval > 0 {
			err = writeDataWithMetaSW(sw, data, mount, &metaByteCount, &lastMeta, metaInterval)
		} else {
			_, err = sw.Write(data)
		}
		if err != nil {
			return
		}

		burstSent += int64(len(data))
		atomic.AddInt64(&listener.BytesSent, int64(len(data)))
	}

	// StreamWriter already flushes after each write, no need for extra flush

	// ==========================================================================
	// PHASE 2: REAL-TIME STREAMING - Use efficient waiting instead of polling
	// ==========================================================================

	var sourceDisconnectTime time.Time
	sourceWasActive := true
	// metaByteCount and lastMeta are now initialized in burst phase above
	// and continue to be used here for proper protocol continuity
	iterationCount := 0

	for {
		iterationCount++

		// Check for client disconnect
		select {
		case <-listener.Done():
			h.logger.Printf("INFO: Listener %s disconnected (client closed) after %v (sent: %d bytes, skipped: %d bytes, iterations: %d)",
				listener.ID, time.Since(startTime).Round(time.Second), sw.BytesWritten(), totalSkipped, iterationCount)
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
			h.logger.Printf("INFO: Listener %s disconnected (source timeout) after %v (sent: %d bytes, skipped: %d bytes, iterations: %d)",
				listener.ID, time.Since(startTime).Round(time.Second), sw.BytesWritten(), totalSkipped, iterationCount)
			return
		}

		// Read data from buffer using SafeReadFromInto to detect skipped bytes
		n, newPos, skipped := buffer.SafeReadFromInto(readPos, readBuf)
		if skipped > 0 {
			totalSkipped += skipped
			h.logger.Printf("WARNING: Listener %s skipped %d bytes at iteration %d (readPos: %d, total skipped: %d)",
				listener.ID, skipped, iterationCount, readPos, totalSkipped)
		}

		if n == 0 {
			// No data available - use efficient waiting with context
			// This wakes up INSTANTLY when new data arrives via sync.Cond.Broadcast()
			// Much better than polling with time.Sleep!
			if !buffer.WaitForDataContext(readPos, listener.Done()) {
				// Client disconnected while waiting
				h.logger.Printf("INFO: Listener %s disconnected (cancelled while waiting) after %v (sent: %d bytes, skipped: %d bytes, iterations: %d)",
					listener.ID, time.Since(startTime).Round(time.Second), sw.BytesWritten(), totalSkipped, iterationCount)
				return
			}
			continue
		}

		data := readBuf[:n]
		readPos = newPos

		// Write data through StreamWriter (handles flushing automatically)
		var err error
		if metaInterval > 0 {
			err = writeDataWithMetaSW(sw, data, mount, &metaByteCount, &lastMeta, metaInterval)
		} else {
			_, err = sw.Write(data)
		}

		if err != nil {
			h.logger.Printf("INFO: Listener %s disconnected after %v (sent: %d bytes, skipped: %d bytes, iterations: %d)",
				listener.ID, time.Since(startTime).Round(time.Second), sw.BytesWritten(), totalSkipped, iterationCount)
			return
		}

		atomic.AddInt64(&listener.BytesSent, int64(len(data)))
		// StreamWriter already flushes after each write
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
			continue
		}

		// Validate bitrate and sample rate
		b2 := data[i+2]
		bitrate := (b2 >> 4) & 0x0F
		sampleRate := (b2 >> 2) & 0x03
		if bitrate == 0x0F || sampleRate == 0x03 {
			continue
		}

		return i
	}

	return 0
}

// writeDataWithMeta writes data with ICY metadata interleaved (for io.Writer)
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
		remaining = remaining[n:] // Use actual bytes written, not requested
	}

	return nil
}

// writeDataWithMetaSW writes data with ICY metadata using StreamWriter (auto-flushes)
func writeDataWithMetaSW(sw *StreamWriter, data []byte, mount *stream.Mount, byteCount *int, lastMeta *string, interval int) error {
	remaining := data

	for len(remaining) > 0 {
		bytesUntilMeta := interval - *byteCount

		if bytesUntilMeta <= 0 {
			// Send metadata block
			meta := mount.GetMetadata()
			title := formatTitle(meta.Title, meta.Artist)

			if title != *lastMeta {
				if err := sendMetaBlockSW(sw, title); err != nil {
					return err
				}
				*lastMeta = title
			} else {
				// Empty metadata block
				if _, err := sw.Write([]byte{0}); err != nil {
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

		n, err := sw.Write(remaining[:toWrite])
		if err != nil {
			return err
		}
		*byteCount += n
		remaining = remaining[n:] // Use actual bytes written, not requested
	}

	return nil
}

// sendMetaBlockSW sends an ICY metadata block through StreamWriter
func sendMetaBlockSW(sw *StreamWriter, title string) error {
	if title == "" {
		_, err := sw.Write([]byte{0})
		return err
	}

	// Format: StreamTitle='title';
	metaStr := fmt.Sprintf("StreamTitle='%s';", escapeMeta(title))

	// Pad to 16-byte boundary
	metaLen := len(metaStr)
	blocks := (metaLen + 15) / 16
	paddedLen := blocks * 16

	buf := make([]byte, paddedLen+1)
	buf[0] = byte(blocks)
	copy(buf[1:], metaStr)

	_, err := sw.Write(buf)
	return err
}

// formatTitle formats metadata for streaming
func formatTitle(title, artist string) string {
	if artist != "" && title != "" {
		return artist + " - " + title
	}
	if title != "" {
		return title
	}
	if artist != "" {
		return artist
	}
	return ""
}

// sendMetaBlock sends an ICY metadata block
func sendMetaBlock(w io.Writer, title string) error {
	if title == "" {
		_, err := w.Write([]byte{0})
		return err
	}

	// Format: StreamTitle='title';
	metaStr := fmt.Sprintf("StreamTitle='%s';", escapeMeta(title))

	// Pad to 16-byte boundary
	metaLen := len(metaStr)
	blocks := (metaLen + 15) / 16
	paddedLen := blocks * 16

	buf := make([]byte, paddedLen+1)
	buf[0] = byte(blocks)
	copy(buf[1:], metaStr)

	_, err := w.Write(buf)
	return err
}

// escapeMeta escapes special characters in metadata
func escapeMeta(s string) string {
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// checkIPAllowed checks if the client IP is allowed
func (h *ListenerHandler) checkIPAllowed(r *http.Request, mount *stream.Mount) bool {
	if mount.Config == nil || len(mount.Config.AllowedIPs) == 0 {
		return true
	}

	clientIP := getClientIP(r)
	for _, pattern := range mount.Config.AllowedIPs {
		if matchIP(clientIP, pattern) {
			return true
		}
	}
	return false
}

// matchIP checks if an IP matches a pattern
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
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Type, icy-br, icy-name, icy-genre, icy-metaint")
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

	uptime := int64(time.Since(h.startTime).Seconds())
	serverID := cfg.Server.ServerID
	if serverID == "" {
		serverID = "GoCast"
	}

	totalBytesSent := h.mountManager.TotalBytesSent()

	sb.WriteString(`{"server":{"id":"`)
	sb.WriteString(escapeJSON(serverID))
	sb.WriteString(`","version":"`)
	sb.WriteString(escapeJSON(h.version))
	sb.WriteString(`","uptime":`)
	sb.WriteString(strconv.FormatInt(uptime, 10))
	sb.WriteString(`,"total_bytes_sent":`)
	sb.WriteString(strconv.FormatInt(totalBytesSent, 10))
	sb.WriteString(`},"mounts":[`)

	first := true
	for _, mountPath := range mounts {
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		if !first {
			sb.WriteString(",")
		}
		first = false

		sb.WriteString(`{"path":"`)
		sb.WriteString(escapeJSON(stats.Path))
		sb.WriteString(`","active":`)
		if stats.Active {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
		sb.WriteString(`,"listeners":`)
		sb.WriteString(strconv.Itoa(stats.Listeners))
		sb.WriteString(`,"bytes_sent":`)
		sb.WriteString(strconv.FormatInt(stats.BytesSent, 10))
		sb.WriteString(`,"content_type":"`)
		sb.WriteString(escapeJSON(stats.ContentType))
		sb.WriteString(`"}`)
	}

	sb.WriteString(`]}`)
	w.Write([]byte(sb.String()))
}

func (h *StatusHandler) serveXML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/xml")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cfg := h.getConfig()
	mounts := h.mountManager.ListMounts()
	var sb strings.Builder

	uptime := int64(time.Since(h.startTime).Seconds())
	serverID := cfg.Server.ServerID
	if serverID == "" {
		serverID = "GoCast"
	}

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<icestats><server_id>`)
	sb.WriteString(escapeXML(serverID))
	sb.WriteString(`</server_id><uptime>`)
	sb.WriteString(strconv.FormatInt(uptime, 10))
	sb.WriteString(`</uptime>`)

	for _, mountPath := range mounts {
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		sb.WriteString(`<source mount="`)
		sb.WriteString(escapeXML(stats.Path))
		sb.WriteString(`"><listeners>`)
		sb.WriteString(strconv.Itoa(stats.Listeners))
		sb.WriteString(`</listeners></source>`)
	}

	sb.WriteString(`</icestats>`)
	w.Write([]byte(sb.String()))
}

func (h *StatusHandler) serveHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cfg := h.getConfig()
	mounts := h.mountManager.ListMounts()
	var sb strings.Builder

	serverID := cfg.Server.ServerID
	if serverID == "" {
		serverID = "GoCast"
	}

	sb.WriteString(`<!DOCTYPE html><html><head><title>`)
	sb.WriteString(serverID)
	sb.WriteString(`</title></head><body><h1>`)
	sb.WriteString(serverID)
	sb.WriteString(`</h1><h2>Mounts</h2><ul>`)

	for _, mountPath := range mounts {
		mount := h.mountManager.GetMount(mountPath)
		if mount == nil {
			continue
		}
		stats := mount.Stats()
		sb.WriteString(`<li><a href="`)
		sb.WriteString(stats.Path)
		sb.WriteString(`">`)
		sb.WriteString(stats.Path)
		sb.WriteString(`</a> - `)
		sb.WriteString(strconv.Itoa(stats.Listeners))
		sb.WriteString(` listeners</li>`)
	}

	sb.WriteString(`</ul></body></html>`)
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
	return s
}
