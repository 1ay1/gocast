// Package server handles HTTP server and listener connections
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
		// Check for fallback mount
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

	// Check listener limits
	if mount.ListenerCount() >= mount.Config.MaxListeners {
		http.Error(w, "Maximum listeners reached", http.StatusServiceUnavailable)
		return
	}

	// Check total client limit
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

	// Check if client wants ICY metadata
	wantsMetadata := r.Header.Get("Icy-MetaData") == "1"
	metadataInterval := 0
	if wantsMetadata {
		metadataInterval = 16000 // Standard interval
	}

	h.logger.Printf("Listener connected: %s from %s (UA: %s, metadata: %v)",
		mountPath, clientIP, userAgent, wantsMetadata)

	// Set response headers
	h.setResponseHeaders(w, mount, metadataInterval)

	// Create listener
	listener := stream.NewListener(w, clientIP, userAgent, mount)

	// Add listener to mount
	if err := mount.AddListener(listener); err != nil {
		h.logger.Printf("Failed to add listener: %v", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	defer func() {
		mount.RemoveListener(listener.ID)
		h.logger.Printf("Listener disconnected: %s from %s (sent: %d bytes)",
			mountPath, clientIP, listener.BytesSent)
	}()

	// Stream data to listener
	h.streamToListener(w, listener, mount, metadataInterval)
}

// setResponseHeaders sets HTTP headers for streaming response
func (h *ListenerHandler) setResponseHeaders(w http.ResponseWriter, mount *stream.Mount, metadataInterval int) {
	meta := mount.GetMetadata()

	// Core streaming headers
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "Mon, 26 Jul 1997 05:00:00 GMT")

	// ICY headers
	w.Header().Set("icy-name", meta.Name)
	w.Header().Set("icy-description", meta.Description)
	w.Header().Set("icy-genre", meta.Genre)
	w.Header().Set("icy-url", meta.URL)
	w.Header().Set("icy-pub", boolToICY(meta.Public))

	if meta.Bitrate > 0 {
		w.Header().Set("icy-br", strconv.Itoa(meta.Bitrate))
	}

	if metadataInterval > 0 {
		w.Header().Set("icy-metaint", strconv.Itoa(metadataInterval))
	}

	// Server identification
	w.Header().Set("Server", fmt.Sprintf("GoCast/%s", "1.0.0"))

	// CORS headers for web players
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Expose-Headers", "icy-metaint, icy-name, icy-description, icy-genre, icy-url, icy-br")

	w.WriteHeader(http.StatusOK)
}

// streamToListener streams audio data to the listener
func (h *ListenerHandler) streamToListener(w http.ResponseWriter, listener *stream.Listener, mount *stream.Mount, metadataInterval int) {
	// Get flusher for immediate writes
	flusher, hasFlusher := w.(http.Flusher)

	buffer := mount.Buffer()
	readPos := buffer.WritePos() - int64(buffer.BurstSize())
	if readPos < 0 {
		readPos = 0
	}

	// Track metadata for ICY injection
	var metaByteCount int
	var lastMetadata string

	// Timeout for waiting on new data
	dataTimeout := time.NewTimer(h.config.Limits.ClientTimeout)
	defer dataTimeout.Stop()

	for {
		select {
		case <-listener.Done():
			return
		case <-mount.Notify():
			// New data available
		case <-dataTimeout.C:
			// Timeout waiting for data
			h.logger.Printf("Listener timeout: %s", listener.ID)
			return
		}

		// Check if source is still active
		if !mount.IsActive() {
			return
		}

		// Read available data
		data, newPos := buffer.ReadFrom(readPos, 8192)
		if len(data) == 0 {
			// No new data, wait for notification
			dataTimeout.Reset(h.config.Limits.ClientTimeout)
			continue
		}

		readPos = newPos

		// Write data (with optional metadata injection)
		var err error
		if metadataInterval > 0 {
			err = h.writeWithMetadata(w, data, mount, &metaByteCount, &lastMetadata, metadataInterval)
		} else {
			_, err = w.Write(data)
		}

		if err != nil {
			return
		}

		listener.BytesSent += int64(len(data))
		listener.LastActive = time.Now()

		if hasFlusher {
			flusher.Flush()
		}

		// Reset timeout
		dataTimeout.Reset(h.config.Limits.ClientTimeout)

		// Check max listener duration
		if mount.Config.MaxListenerDuration > 0 {
			if time.Since(listener.ConnectedAt) > mount.Config.MaxListenerDuration {
				h.logger.Printf("Listener max duration reached: %s", listener.ID)
				return
			}
		}
	}
}

// writeWithMetadata writes data with ICY metadata injection
func (h *ListenerHandler) writeWithMetadata(w io.Writer, data []byte, mount *stream.Mount, byteCount *int, lastMetadata *string, interval int) error {
	for len(data) > 0 {
		// Calculate bytes until next metadata point
		bytesUntilMeta := interval - (*byteCount % interval)

		if bytesUntilMeta > len(data) {
			// Write all remaining data
			n, err := w.Write(data)
			*byteCount += n
			return err
		}

		// Write data up to metadata point
		n, err := w.Write(data[:bytesUntilMeta])
		*byteCount += n
		if err != nil {
			return err
		}
		data = data[bytesUntilMeta:]

		// Write metadata block
		if err := h.writeMetadataBlock(w, mount, lastMetadata); err != nil {
			return err
		}
	}

	return nil
}

// writeMetadataBlock writes an ICY metadata block
func (h *ListenerHandler) writeMetadataBlock(w io.Writer, mount *stream.Mount, lastMetadata *string) error {
	meta := mount.GetMetadata()
	streamTitle := meta.StreamTitle

	// Check if metadata has changed
	if streamTitle == *lastMetadata {
		// No change, write empty metadata block (size = 0)
		_, err := w.Write([]byte{0})
		return err
	}

	*lastMetadata = streamTitle

	// Format: StreamTitle='Song Title';
	metaStr := fmt.Sprintf("StreamTitle='%s';", escapeMetadata(streamTitle))

	// Calculate block size (16-byte blocks)
	blockSize := (len(metaStr) + 15) / 16
	if blockSize > 255 {
		blockSize = 255
		metaStr = metaStr[:255*16]
	}

	// Pad to full block
	padding := blockSize*16 - len(metaStr)
	metaStr += strings.Repeat("\x00", padding)

	// Write block size byte and metadata
	if _, err := w.Write([]byte{byte(blockSize)}); err != nil {
		return err
	}
	_, err := w.Write([]byte(metaStr))
	return err
}

// escapeMetadata escapes special characters in metadata
func escapeMetadata(s string) string {
	// Replace single quotes with escaped version
	s = strings.ReplaceAll(s, "'", "\\'")
	// Remove any null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}

// checkIPAllowed checks if the client IP is allowed
func (h *ListenerHandler) checkIPAllowed(mount *stream.Mount, clientIP string) bool {
	// Check denied IPs first
	for _, denied := range mount.Config.DeniedIPs {
		if matchIP(clientIP, denied) {
			return false
		}
	}

	// If no allowed IPs specified, allow all
	if len(mount.Config.AllowedIPs) == 0 {
		return true
	}

	// Check allowed IPs
	for _, allowed := range mount.Config.AllowedIPs {
		if matchIP(clientIP, allowed) {
			return true
		}
	}

	return false
}

// matchIP checks if an IP matches a pattern (simple prefix matching)
func matchIP(ip, pattern string) bool {
	// Simple matching - can be extended for CIDR notation
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(ip, pattern[:len(pattern)-1])
	}
	return ip == pattern
}

// boolToICY converts a boolean to ICY format (1 or 0)
func boolToICY(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// HandleOptions handles CORS preflight requests
func (h *ListenerHandler) HandleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// StatusHandler returns server status in various formats
type StatusHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(mm *stream.MountManager, cfg *config.Config) *StatusHandler {
	return &StatusHandler{
		mountManager: mm,
		config:       cfg,
	}
}

// ServeHTTP serves status information
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "html"
	}

	switch format {
	case "json":
		h.serveJSON(w, r)
	case "xml":
		h.serveXML(w, r)
	default:
		h.serveHTML(w, r)
	}
}

// serveJSON serves status as JSON
func (h *StatusHandler) serveJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := h.mountManager.Stats()

	fmt.Fprintf(w, `{"icestats":{"admin":"%s","host":"%s","location":"%s","server_id":"GoCast/1.0.0",`,
		h.config.Server.AdminRoot, h.config.Server.Hostname, h.config.Server.Location)
	fmt.Fprintf(w, `"server_start":"","source":[`)

	for i, stat := range stats {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"listenurl":"http://%s:%d%s",`,
			h.config.Server.Hostname, h.config.Server.Port, stat.Path)
		fmt.Fprintf(w, `"listeners":%d,"peak_listeners":%d,`,
			stat.Listeners, stat.PeakListeners)
		fmt.Fprintf(w, `"audio_info":"","genre":"%s","server_description":"%s",`,
			stat.Metadata.Genre, stat.Metadata.Description)
		fmt.Fprintf(w, `"server_name":"%s","server_type":"%s",`,
			stat.Metadata.Name, stat.ContentType)
		fmt.Fprintf(w, `"stream_start":"","title":"%s"}`,
			escapeJSON(stat.Metadata.StreamTitle))
	}

	fmt.Fprint(w, "]}}")
}

// serveXML serves status as XML (Icecast compatible)
func (h *StatusHandler) serveXML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, "\n<icestats>")
	fmt.Fprintf(w, "<admin>%s</admin>", h.config.Server.AdminRoot)
	fmt.Fprintf(w, "<host>%s</host>", h.config.Server.Hostname)
	fmt.Fprintf(w, "<location>%s</location>", h.config.Server.Location)
	fmt.Fprint(w, "<server_id>GoCast/1.0.0</server_id>")

	for _, stat := range h.mountManager.Stats() {
		fmt.Fprint(w, "<source>")
		fmt.Fprintf(w, "<mount>%s</mount>", stat.Path)
		fmt.Fprintf(w, "<listeners>%d</listeners>", stat.Listeners)
		fmt.Fprintf(w, "<peak_listeners>%d</peak_listeners>", stat.PeakListeners)
		fmt.Fprintf(w, "<genre>%s</genre>", escapeXML(stat.Metadata.Genre))
		fmt.Fprintf(w, "<server_name>%s</server_name>", escapeXML(stat.Metadata.Name))
		fmt.Fprintf(w, "<server_description>%s</server_description>", escapeXML(stat.Metadata.Description))
		fmt.Fprintf(w, "<server_type>%s</server_type>", stat.ContentType)
		fmt.Fprintf(w, "<server_url>%s</server_url>", escapeXML(stat.Metadata.URL))
		fmt.Fprintf(w, "<title>%s</title>", escapeXML(stat.Metadata.StreamTitle))
		fmt.Fprintf(w, "<listenurl>http://%s:%d%s</listenurl>",
			h.config.Server.Hostname, h.config.Server.Port, stat.Path)
		fmt.Fprint(w, "</source>")
	}

	fmt.Fprint(w, "</icestats>")
}

// serveHTML serves status as HTML
func (h *StatusHandler) serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<title>GoCast Server Status</title>
<style>
body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
h1 { color: #333; }
.mount { background: white; padding: 15px; margin: 10px 0; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.mount h2 { margin-top: 0; color: #2196F3; }
.info { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; }
.info-item { background: #f9f9f9; padding: 10px; border-radius: 4px; }
.info-item label { font-weight: bold; color: #666; }
.info-item span { display: block; color: #333; }
.listeners { font-size: 24px; color: #4CAF50; }
.offline { color: #999; }
</style>
</head>
<body>
<h1>🎵 GoCast Server Status</h1>
<p>Server: `+h.config.Server.Hostname+` | Location: `+h.config.Server.Location+`</p>
`)

	stats := h.mountManager.Stats()
	if len(stats) == 0 {
		fmt.Fprint(w, `<p class="offline">No active streams</p>`)
	}

	for _, stat := range stats {
		status := "🔴 Offline"
		if stat.Active {
			status = "🟢 Live"
		}

		fmt.Fprintf(w, `<div class="mount">
<h2>%s %s</h2>
<div class="info">
<div class="info-item"><label>Listeners</label><span class="listeners">%d</span></div>
<div class="info-item"><label>Peak</label><span>%d</span></div>
<div class="info-item"><label>Now Playing</label><span>%s</span></div>
<div class="info-item"><label>Genre</label><span>%s</span></div>
<div class="info-item"><label>Description</label><span>%s</span></div>
<div class="info-item"><label>Content Type</label><span>%s</span></div>
<div class="info-item"><label>Listen URL</label><span><a href="http://%s:%d%s">http://%s:%d%s</a></span></div>
</div>
</div>`,
			stat.Path, status,
			stat.Listeners,
			stat.PeakListeners,
			stat.Metadata.StreamTitle,
			stat.Metadata.Genre,
			stat.Metadata.Description,
			stat.ContentType,
			h.config.Server.Hostname, h.config.Server.Port, stat.Path,
			h.config.Server.Hostname, h.config.Server.Port, stat.Path,
		)
	}

	fmt.Fprint(w, `
<p style="margin-top: 20px; color: #999; font-size: 12px;">
Powered by GoCast - A modern Icecast replacement written in Go
</p>
</body>
</html>`)
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// escapeJSON escapes special JSON characters
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
