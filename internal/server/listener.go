// Package server handles HTTP server and listener connections
// Ultra-low-latency implementation for instant listener connection
package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/stream"
)

// Ultra-low latency constants
const (
	// Tiny chunks for minimum latency
	chunkSize = 1024 // 1KB = ~25ms at 320kbps

	// Zero burst - start at live edge immediately
	maxBurstBytes = 1024 // Only 1KB burst for instant start

	// Fast poll for responsive streaming
	pollInterval = 1 * time.Millisecond

	// ICY metadata interval
	icyMetadataInterval = 16000
)

// ListenerHandler handles listener connections
type ListenerHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
	logger       *log.Logger
}

// NewListenerHandler creates a new listener handler
func NewListenerHandler(mm *stream.MountManager, cfg *config.Config, logger *log.Logger) *ListenerHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &ListenerHandler{
		mountManager: mm,
		config:       cfg,
		logger:       logger,
	}
}

// ServeHTTP handles listener requests
func (h *ListenerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Path
	if mountPath == "" {
		mountPath = "/"
	}

	// Get mount
	mount := h.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	// Check if source is active
	if !mount.IsActive() {
		if mount.Config.FallbackMount != "" {
			fallback := h.mountManager.GetMount(mount.Config.FallbackMount)
			if fallback != nil && fallback.IsActive() {
				mount = fallback
			} else {
				http.Error(w, "Stream not available", http.StatusServiceUnavailable)
				return
			}
		} else {
			http.Error(w, "Stream not available", http.StatusServiceUnavailable)
			return
		}
	}

	// Quick capacity checks
	if mount.ListenerCount() >= mount.Config.MaxListeners {
		http.Error(w, "Maximum listeners reached", http.StatusServiceUnavailable)
		return
	}
	if h.mountManager.TotalListeners() >= h.config.Limits.MaxClients {
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	// Get client info
	clientIP := getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Check IP restrictions
	if !h.checkIPAllowed(mount, clientIP) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// ICY metadata
	wantsMetadata := r.Header.Get("Icy-MetaData") == "1"
	metadataInterval := 0
	if wantsMetadata {
		metadataInterval = icyMetadataInterval
	}

	h.logger.Printf("Listener connected: %s from %s", mountPath, clientIP)

	// CRITICAL: Set headers and flush IMMEDIATELY
	h.setResponseHeaders(w, mount, metadataInterval)

	// Get flusher and flush headers NOW
	flusher, hasFlusher := w.(http.Flusher)
	if hasFlusher {
		flusher.Flush()
	}

	// Create listener
	listener := stream.NewListener(w, clientIP, userAgent, mount)

	if err := mount.AddListener(listener); err != nil {
		h.logger.Printf("Failed to add listener: %v", err)
		return
	}

	defer func() {
		mount.RemoveListener(listener.ID)
		h.logger.Printf("Listener disconnected: %s (sent: %d bytes)", mountPath, listener.BytesSent)
	}()

	// Stream with ultra-low latency
	h.streamUltraLowLatency(w, flusher, hasFlusher, listener, mount, metadataInterval)
}

// setResponseHeaders sets HTTP headers for streaming response
func (h *ListenerHandler) setResponseHeaders(w http.ResponseWriter, mount *stream.Mount, metadataInterval int) {
	meta := mount.GetMetadata()

	// Essential headers only - minimize header processing time
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")

	// ICY headers
	if meta.Name != "" {
		w.Header().Set("icy-name", meta.Name)
	}
	if meta.Bitrate > 0 {
		w.Header().Set("icy-br", strconv.Itoa(meta.Bitrate))
	}
	if metadataInterval > 0 {
		w.Header().Set("icy-metaint", strconv.Itoa(metadataInterval))
	}

	// Server
	w.Header().Set("Server", "GoCast/1.0")

	// CORS for web players
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send 200 OK immediately
	w.WriteHeader(http.StatusOK)
}

// streamUltraLowLatency streams with minimum possible latency
func (h *ListenerHandler) streamUltraLowLatency(w http.ResponseWriter, flusher http.Flusher, hasFlusher bool, listener *stream.Listener, mount *stream.Mount, metadataInterval int) {
	buffer := mount.Buffer()

	// Start at LIVE EDGE - not burst position
	// This is the key to instant playback
	writePos := buffer.WritePos()
	readPos := writePos // Start exactly where we are NOW

	// Send tiny burst just to prime the player's buffer (optional)
	// Most players need SOME data to start, but minimal
	if writePos > int64(maxBurstBytes) {
		burstStart := writePos - int64(maxBurstBytes)
		burstData, _ := buffer.ReadFrom(burstStart, maxBurstBytes)
		if len(burstData) > 0 {
			w.Write(burstData)
			if hasFlusher {
				flusher.Flush()
			}
			listener.BytesSent += int64(len(burstData))
		}
		readPos = writePos
	}

	// Metadata tracking
	var metaByteCount int
	var lastMetadata string

	// Use notify channel for instant response to new data
	notifyChan := buffer.NotifyChan()

	// Timeout
	timeout := h.config.Limits.ClientTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	lastActivity := time.Now()

	// Tight loop for ultra-low latency
	for {
		// Check if we should exit
		select {
		case <-listener.Done():
			return
		default:
		}

		// Check source active
		if !mount.IsActive() {
			return
		}

		// Check timeout
		if time.Since(lastActivity) > timeout {
			return
		}

		// Try to read available data
		data, newPos := buffer.ReadFrom(readPos, chunkSize)

		if len(data) == 0 {
			// No data - wait briefly for notification or poll
			select {
			case <-listener.Done():
				return
			case <-notifyChan:
				// Data available, loop immediately
				continue
			case <-time.After(pollInterval):
				// Quick poll, try again
				continue
			}
		}

		readPos = newPos
		lastActivity = time.Now()

		// Write data
		var err error
		if metadataInterval > 0 {
			err = writeWithMetadata(w, data, mount, &metaByteCount, &lastMetadata, metadataInterval)
		} else {
			_, err = w.Write(data)
		}

		if err != nil {
			return
		}

		listener.BytesSent += int64(len(data))
		listener.LastActive = lastActivity

		// Flush EVERY write for lowest latency
		if hasFlusher {
			flusher.Flush()
		}

		// Check listener duration limit
		if mount.Config.MaxListenerDuration > 0 {
			if time.Since(listener.ConnectedAt) > mount.Config.MaxListenerDuration {
				return
			}
		}
	}
}

// writeWithMetadata writes data with ICY metadata injection
func writeWithMetadata(w io.Writer, data []byte, mount *stream.Mount, byteCount *int, lastMetadata *string, interval int) error {
	for len(data) > 0 {
		bytesUntilMeta := interval - (*byteCount % interval)

		if bytesUntilMeta > len(data) {
			n, err := w.Write(data)
			*byteCount += n
			return err
		}

		// Write up to metadata point
		n, err := w.Write(data[:bytesUntilMeta])
		*byteCount += n
		if err != nil {
			return err
		}
		data = data[bytesUntilMeta:]

		// Write metadata
		if err := writeMetadataBlock(w, mount, lastMetadata); err != nil {
			return err
		}
	}
	return nil
}

// writeMetadataBlock writes ICY metadata
func writeMetadataBlock(w io.Writer, mount *stream.Mount, lastMetadata *string) error {
	meta := mount.GetMetadata()
	streamTitle := meta.StreamTitle

	if streamTitle == *lastMetadata {
		// No change - empty block
		_, err := w.Write([]byte{0})
		return err
	}

	*lastMetadata = streamTitle

	metaStr := fmt.Sprintf("StreamTitle='%s';", escapeMetadata(streamTitle))

	blockSize := (len(metaStr) + 15) / 16
	if blockSize > 255 {
		blockSize = 255
		metaStr = metaStr[:255*16]
	}

	// Pad to block boundary
	padding := blockSize*16 - len(metaStr)
	metaStr += strings.Repeat("\x00", padding)

	if _, err := w.Write([]byte{byte(blockSize)}); err != nil {
		return err
	}
	_, err := w.Write([]byte(metaStr))
	return err
}

// escapeMetadata escapes special characters
func escapeMetadata(s string) string {
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}

// checkIPAllowed checks if IP is allowed
func (h *ListenerHandler) checkIPAllowed(mount *stream.Mount, clientIP string) bool {
	for _, denied := range mount.Config.DeniedIPs {
		if matchIP(clientIP, denied) {
			return false
		}
	}
	if len(mount.Config.AllowedIPs) == 0 {
		return true
	}
	for _, allowed := range mount.Config.AllowedIPs {
		if matchIP(clientIP, allowed) {
			return true
		}
	}
	return false
}

// matchIP matches IP against pattern
func matchIP(ip, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(ip, pattern[:len(pattern)-1])
	}
	return ip == pattern
}

// boolToICY converts bool to ICY format
func boolToICY(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// getClientIP extracts client IP
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// HandleOptions handles CORS preflight
func (h *ListenerHandler) HandleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// StatusHandler returns server status
type StatusHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
}

// NewStatusHandler creates a status handler
func NewStatusHandler(mm *stream.MountManager, cfg *config.Config) *StatusHandler {
	return &StatusHandler{mountManager: mm, config: cfg}
}

// ServeHTTP serves status
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	switch format {
	case "json":
		h.serveJSON(w, r)
	case "xml":
		h.serveXML(w, r)
	default:
		h.serveHTML(w, r)
	}
}

// serveJSON serves JSON status
func (h *StatusHandler) serveJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := h.mountManager.Stats()

	fmt.Fprintf(w, `{"icestats":{"server_id":"GoCast/1.0.0","source":[`)
	for i, stat := range stats {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"mount":"%s","listeners":%d,"peak":%d,"active":%v}`,
			stat.Path, stat.Listeners, stat.PeakListeners, stat.Active)
	}
	fmt.Fprint(w, "]}}")
}

// serveXML serves XML status
func (h *StatusHandler) serveXML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprint(w, `<?xml version="1.0"?><icestats>`)
	for _, stat := range h.mountManager.Stats() {
		fmt.Fprintf(w, `<source mount="%s"><listeners>%d</listeners></source>`,
			stat.Path, stat.Listeners)
	}
	fmt.Fprint(w, `</icestats>`)
}

// serveHTML serves HTML status
func (h *StatusHandler) serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><head><title>GoCast</title>
<style>body{font-family:system-ui;margin:40px;background:#111;color:#eee}
h1{color:#00ADD8}.mount{background:#222;padding:20px;margin:10px 0;border-radius:8px}
.live{color:#4f4}.offline{color:#f44}</style></head>
<body><h1>🎵 GoCast Status</h1>`)

	for _, stat := range h.mountManager.Stats() {
		status := `<span class="offline">● Offline</span>`
		if stat.Active {
			status = `<span class="live">● Live</span>`
		}
		fmt.Fprintf(w, `<div class="mount"><h2>%s %s</h2>
<p>Listeners: <strong>%d</strong> (peak: %d)</p>
<p>Listen: <a href="%s">%s</a></p></div>`,
			stat.Path, status, stat.Listeners, stat.PeakListeners,
			stat.Path, stat.Path)
	}
	fmt.Fprint(w, `</body></html>`)
}

// escapeXML escapes XML
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeJSON escapes JSON
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}
