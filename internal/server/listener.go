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
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/stream"
)

// Version of GoCast server
var Version = "dev"

// Streaming constants - OPTIMIZED FOR BULLETPROOF STREAMING
const (
	// Read chunk size - larger chunks = more efficient, less syscalls
	// 16KB = ~400ms at 320kbps, efficient while still responsive
	streamChunkSize = 16384

	// Poll interval when waiting for data - fast polling for low latency
	dataPollInterval = 2 * time.Millisecond

	// How long to wait for source reconnection (between songs/reconnects)
	// 30 seconds is generous - allows for source restarts
	sourceReconnectWait = 30 * time.Second

	// ICY metadata interval - standard Icecast interval
	icyMetaInterval = 16000

	// Client timeout for inactive connections - generous for slow networks
	defaultClientTimeout = 120 * time.Second
)

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

	// Check if we can add listener
	if !mount.CanAddListener() {
		http.Error(w, "Listener limit reached", http.StatusServiceUnavailable)
		return
	}

	// Check IP restrictions
	if !h.checkIPAllowed(r, mount) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Create listener
	listener := stream.NewListener(clientIP, r.UserAgent())
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

// streamToClient streams audio data to client with BULLETPROOF error handling
// This implementation prioritizes data integrity - no bytes are ever skipped unless
// the listener is more than buffer-size behind (26+ seconds at 320kbps)
func (h *ListenerHandler) streamToClient(w http.ResponseWriter, flusher http.Flusher, hasFlusher bool, listener *stream.Listener, mount *stream.Mount, metaInterval int) {
	buffer := mount.Buffer()
	if buffer == nil {
		return
	}

	// Wait for source if not active (allows connecting before source starts)
	if !mount.IsActive() {
		waitStart := time.Now()
		for !mount.IsActive() {
			if time.Since(waitStart) > sourceReconnectWait {
				return // No source after waiting
			}
			select {
			case <-listener.Done():
				return
			case <-time.After(100 * time.Millisecond):
				// Keep waiting
			}
		}
	}

	// Get buffer from pool - use larger buffer for efficiency
	bufPtr := h.bufPool.Get().(*[]byte)
	readBuf := *bufPtr
	defer h.bufPool.Put(bufPtr)

	// Start at a position that gives us some buffer room
	// Don't start at live edge - start with some burst data for smooth playback
	writePos := buffer.WritePos()
	burstSize := int64(buffer.BurstSize())
	if burstSize > writePos {
		burstSize = writePos
	}
	readPos := writePos - burstSize
	if readPos < 0 {
		readPos = 0
	}

	// Frame sync state - find MP3 frame on first read
	needsSync := true

	// Metadata state
	var metaByteCount int
	var lastMeta string

	// Timeout handling - generous timeout for slow connections
	timeout := h.config.Limits.ClientTimeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	lastActivity := time.Now()

	// Source disconnect handling
	var sourceDisconnectTime time.Time
	sourceWasActive := true // We know it's active now

	// Main streaming loop - BULLETPROOF version
	for {
		// Check client disconnect
		select {
		case <-listener.Done():
			return
		default:
		}

		// Check source status with reconnection support
		sourceActive := mount.IsActive()

		if !sourceActive {
			if sourceWasActive {
				// Source just disconnected, start waiting
				sourceDisconnectTime = time.Now()
				sourceWasActive = false
			}

			// Check if we've waited too long
			if time.Since(sourceDisconnectTime) > sourceReconnectWait {
				return // Give up, source not coming back
			}

			// Wait a bit for reconnection - but keep checking for data
			// There might still be buffered data to send
			select {
			case <-listener.Done():
				return
			case <-time.After(50 * time.Millisecond):
				// Continue to try reading any remaining buffered data
			}
		} else {
			if !sourceWasActive {
				// Source just reconnected - DON'T reset position!
				// Keep reading from where we were to avoid losing any data
				// The buffer is large enough to handle the gap
				needsSync = true // Need to find MP3 frame again
			}
			sourceWasActive = true
		}

		// Check client timeout
		if time.Since(lastActivity) > timeout {
			return
		}

		// Try to read data - this is the bulletproof read
		n, newPos := buffer.ReadFromInto(readPos, readBuf)

		if n == 0 {
			// No data available, wait efficiently with short timeout
			// This prevents busy-waiting while being responsive
			if buffer.WaitForData(readPos, dataPollInterval) {
				continue
			}
			continue
		}

		data := readBuf[:n]
		readPos = newPos
		lastActivity = time.Now()

		// Find MP3 frame sync on first chunk for clean audio start
		if needsSync && n >= 4 {
			syncOffset := findMP3Sync(data)
			if syncOffset > 0 && syncOffset < n-4 {
				data = data[syncOffset:]
			}
			needsSync = false
		}

		if len(data) == 0 {
			continue
		}

		// Write to client - handle partial writes
		var err error
		if metaInterval > 0 {
			err = writeDataWithMeta(w, data, mount, &metaByteCount, &lastMeta, metaInterval)
		} else {
			_, err = w.Write(data)
		}

		if err != nil {
			return // Client disconnected
		}

		listener.BytesSent += int64(len(data))
		listener.LastActive = lastActivity

		// Flush for low latency - do this after every write for smooth streaming
		if hasFlusher {
			flusher.Flush()
		}

		// Check duration limit
		if mount.Config.MaxListenerDuration > 0 {
			if time.Since(listener.ConnectedAt) > mount.Config.MaxListenerDuration {
				return
			}
		}
	}
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
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		h.serveJSON(w)
	} else if strings.Contains(accept, "text/xml") || strings.Contains(accept, "application/xml") {
		h.serveXML(w)
	} else {
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

	sb.WriteString(fmt.Sprintf(`{"server_id":%q,"version":%q,"started":%q,"uptime":%d,"host":%q,"mounts":[`,
		serverID, h.version, h.startTime.Format(time.RFC3339), uptime, cfg.Server.Hostname))

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

		// Include metadata object with stream_title for admin panel
		sb.WriteString(fmt.Sprintf(`{"path":%q,"active":%t,"listeners":%d,"peak":%d,"name":%q,"genre":%q,"bitrate":%d,"metadata":{"stream_title":%q,"artist":%q,"title":%q}}`,
			mountPath, mount.IsActive(), stats.Listeners, stats.PeakListeners,
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
