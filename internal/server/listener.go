// Package server handles HTTP server and listener connections
// Ultra-low-latency implementation with robust MP3 frame alignment
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

// Streaming constants optimized for low latency and smooth playback
const (
	// Chunk size for reading from buffer - balance between efficiency and latency
	chunkSize = 8192 // 8KB chunks - ~200ms at 320kbps

	// Initial burst to prime player buffer - must contain complete MP3 frames
	initialBurstSize = 16384 // 16KB ensures smooth start (~400ms at 320kbps)

	// Poll interval when no data available - tight loop for responsiveness
	pollInterval = 2 * time.Millisecond

	// ICY metadata interval (bytes between metadata blocks)
	icyMetadataInterval = 16000

	// MP3 frame sizes at 320kbps: ~1044 bytes at 44.1kHz, ~960 bytes at 48kHz
	maxMP3FrameSize = 1152 // Maximum possible MP3 frame size
)

// MP3FrameScanner handles finding and aligning to MP3 frame boundaries
type MP3FrameScanner struct {
	synced    bool
	remainder []byte // Partial frame data waiting for more
}

// FindFrameStart finds the first valid MP3 frame sync in data
// Returns offset to frame start, or -1 if not found
func (s *MP3FrameScanner) FindFrameStart(data []byte) int {
	if len(data) < 4 {
		return -1
	}

	for i := 0; i < len(data)-3; i++ {
		if isValidMP3FrameHeader(data[i:]) {
			return i
		}
	}
	return -1
}

// isValidMP3FrameHeader checks if bytes form a valid MP3 frame header
func isValidMP3FrameHeader(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// First byte must be 0xFF (frame sync)
	if data[0] != 0xFF {
		return false
	}

	b1 := data[1]
	// Second byte: top 3 bits must be set (0xE0), but not 0xFF
	if b1 == 0xFF || (b1&0xE0) != 0xE0 {
		return false
	}

	// Extract header fields
	version := (b1 >> 3) & 0x03 // MPEG version
	layer := (b1 >> 1) & 0x03   // Layer
	// protection := b1 & 0x01     // CRC protection (not needed for validation)

	// Version 01 is reserved
	if version == 0x01 {
		return false
	}

	// Layer 00 is reserved
	if layer == 0x00 {
		return false
	}

	b2 := data[2]
	bitrate := (b2 >> 4) & 0x0F    // Bitrate index
	sampleRate := (b2 >> 2) & 0x03 // Sample rate index

	// Bitrate 1111 is invalid
	if bitrate == 0x0F {
		return false
	}

	// Bitrate 0000 is "free" format - technically valid but rare
	// We'll accept it

	// Sample rate 11 is reserved
	if sampleRate == 0x03 {
		return false
	}

	// Valid MP3 frame header!
	return true
}

// getMP3FrameSize calculates the frame size from header bytes
func getMP3FrameSize(data []byte) int {
	if len(data) < 4 || !isValidMP3FrameHeader(data) {
		return 0
	}

	b1 := data[1]
	version := (b1 >> 3) & 0x03
	layer := (b1 >> 1) & 0x03

	b2 := data[2]
	bitrateIdx := (b2 >> 4) & 0x0F
	sampleRateIdx := (b2 >> 2) & 0x03
	padding := (b2 >> 1) & 0x01

	// Bitrate table for MPEG1 Layer 3 (most common)
	bitratesV1L3 := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	// Bitrate table for MPEG2/2.5 Layer 3
	bitratesV2L3 := []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}

	// Sample rate tables
	sampleRatesV1 := []int{44100, 48000, 32000, 0}
	sampleRatesV2 := []int{22050, 24000, 16000, 0}
	sampleRatesV25 := []int{11025, 12000, 8000, 0}

	var bitrate, sampleRate, samplesPerFrame int

	switch version {
	case 0x03: // MPEG1
		if layer == 0x01 { // Layer 3
			bitrate = bitratesV1L3[bitrateIdx] * 1000
			samplesPerFrame = 1152
		} else {
			return 0 // Unsupported layer
		}
		sampleRate = sampleRatesV1[sampleRateIdx]
	case 0x02: // MPEG2
		if layer == 0x01 { // Layer 3
			bitrate = bitratesV2L3[bitrateIdx] * 1000
			samplesPerFrame = 576
		} else {
			return 0
		}
		sampleRate = sampleRatesV2[sampleRateIdx]
	case 0x00: // MPEG2.5
		if layer == 0x01 {
			bitrate = bitratesV2L3[bitrateIdx] * 1000
			samplesPerFrame = 576
		} else {
			return 0
		}
		sampleRate = sampleRatesV25[sampleRateIdx]
	default:
		return 0
	}

	if bitrate == 0 || sampleRate == 0 {
		return 0
	}

	// Frame size formula: (samples_per_frame / 8 * bitrate) / sample_rate + padding
	frameSize := (samplesPerFrame * bitrate / 8) / sampleRate
	if padding == 1 {
		frameSize++
	}

	return frameSize
}

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

	h.logger.Printf("DEBUG: Listener connecting: %s from %s (method: %s, user-agent: %s)",
		mountPath, getClientIP(r), r.Method, r.UserAgent())

	// Get mount
	mount := h.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	// Check if source is active
	if !mount.IsActive() {
		http.Error(w, "Stream offline", http.StatusServiceUnavailable)
		return
	}

	// Check listener limit
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
	listener := stream.NewListener(getClientIP(r), r.UserAgent())
	mount.AddListener(listener)
	defer mount.RemoveListener(listener)

	// Check for ICY metadata support
	metadataInterval := 0
	if r.Header.Get("Icy-MetaData") == "1" {
		metadataInterval = icyMetadataInterval
	}

	// Set response headers
	h.setResponseHeaders(w, mount, metadataInterval)

	// Get flusher for streaming
	flusher, hasFlusher := w.(http.Flusher)
	if hasFlusher {
		flusher.Flush()
	}

	// Stream with frame-aligned data
	h.streamWithFrameAlignment(w, flusher, hasFlusher, listener, mount, metadataInterval)
}

// setResponseHeaders sets HTTP response headers for streaming
func (h *ListenerHandler) setResponseHeaders(w http.ResponseWriter, mount *stream.Mount, metadataInterval int) {
	meta := mount.GetMetadata()

	// Essential streaming headers
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Pragma", "no-cache")

	// ICY headers for compatibility
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
	if metadataInterval > 0 {
		w.Header().Set("icy-metaint", strconv.Itoa(metadataInterval))
	}

	// Server identification
	w.Header().Set("Server", "GoCast/"+Version)

	// CORS for web players
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")

	// Send headers immediately
	w.WriteHeader(http.StatusOK)
}

// streamWithFrameAlignment streams audio data with MP3 frame alignment
func (h *ListenerHandler) streamWithFrameAlignment(w http.ResponseWriter, flusher http.Flusher, hasFlusher bool, listener *stream.Listener, mount *stream.Mount, metadataInterval int) {
	buffer := mount.Buffer()
	if buffer == nil {
		return
	}

	// Get current write position - this is where live data is being written
	writePos := buffer.WritePos()

	// Start reading from live position (no burst, just catch up to live edge)
	// This avoids the complexity of frame alignment on old data
	readPos := writePos

	// For the first chunk, we need to find an MP3 frame boundary
	needsFrameSync := true

	// Metadata state
	var metaByteCount int
	var lastMetadata string

	// Notification channel for new data
	notifyChan := buffer.NotifyChan()

	// Timeout handling
	timeout := h.config.Limits.ClientTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	lastActivity := time.Now()

	// Pre-allocate buffer for metadata operations
	metaBuf := make([]byte, chunkSize)

	// MP3 frame scanner for initial sync
	scanner := &MP3FrameScanner{}

	// Main streaming loop - optimized for throughput
	for {
		// Non-blocking check for disconnect
		select {
		case <-listener.Done():
			return
		default:
		}

		// Check mount still active
		if !mount.IsActive() {
			return
		}

		// Read all available data up to chunk size
		data, newPos := buffer.ReadFrom(readPos, chunkSize)

		if len(data) == 0 {
			// Check timeout before waiting
			if time.Since(lastActivity) > timeout {
				return
			}

			// Wait for new data - use select with short timeout
			select {
			case <-listener.Done():
				return
			case <-notifyChan:
				// New data signaled, immediately try to read
			case <-time.After(pollInterval):
				// Poll timeout, check again
			}
			continue
		}

		// Update read position BEFORE writing (prevents re-reading same data)
		readPos = newPos
		lastActivity = time.Now()

		// On first chunk, find MP3 frame boundary to avoid initial noise
		if needsFrameSync && len(data) >= 4 {
			frameStart := scanner.FindFrameStart(data)
			if frameStart > 0 && frameStart < len(data)-4 {
				// Skip bytes before first valid frame
				data = data[frameStart:]
			}
			needsFrameSync = false // Only do this once
		}

		// Skip if we have no data after frame sync
		if len(data) == 0 {
			continue
		}

		// Write data to client
		var err error
		if metadataInterval > 0 {
			err = h.writeWithMetadata(w, data, mount, &metaByteCount, &lastMetadata, metadataInterval, metaBuf)
		} else {
			_, err = w.Write(data)
		}

		if err != nil {
			return
		}

		listener.BytesSent += int64(len(data))
		listener.LastActive = lastActivity

		// Flush after each write for low latency
		if hasFlusher {
			flusher.Flush()
		}

		// Check listener duration limit (less frequently)
		if mount.Config.MaxListenerDuration > 0 {
			if time.Since(listener.ConnectedAt) > mount.Config.MaxListenerDuration {
				return
			}
		}
	}
}

// writeWithMetadata writes data with ICY metadata interleaved
func (h *ListenerHandler) writeWithMetadata(w io.Writer, data []byte, mount *stream.Mount, byteCount *int, lastMeta *string, interval int, buf []byte) error {
	remaining := data
	for len(remaining) > 0 {
		// Bytes until next metadata block
		bytesUntilMeta := interval - *byteCount

		if bytesUntilMeta <= 0 {
			// Time to send metadata
			meta := mount.GetMetadata()
			currentMeta := formatStreamTitle(meta.Title, meta.Artist)

			// Only send if changed
			if currentMeta != *lastMeta {
				if err := writeMetadataBlock(w, currentMeta); err != nil {
					return err
				}
				*lastMeta = currentMeta
			} else {
				// Send empty metadata block
				if _, err := w.Write([]byte{0}); err != nil {
					return err
				}
			}
			*byteCount = 0
			bytesUntilMeta = interval
		}

		// Write audio data up to next metadata point
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

// formatStreamTitle formats metadata for ICY protocol
func formatStreamTitle(title, artist string) string {
	if artist != "" && title != "" {
		return fmt.Sprintf("%s - %s", artist, title)
	}
	if title != "" {
		return title
	}
	if artist != "" {
		return artist
	}
	return ""
}

// writeMetadataBlock writes an ICY metadata block
func writeMetadataBlock(w io.Writer, title string) error {
	if title == "" {
		_, err := w.Write([]byte{0})
		return err
	}

	// Format: StreamTitle='...';
	metaStr := fmt.Sprintf("StreamTitle='%s';", escapeMetadata(title))

	// Pad to 16-byte boundary
	metaLen := len(metaStr)
	blocks := (metaLen + 15) / 16
	if blocks > 255 {
		blocks = 255
		metaStr = metaStr[:255*16]
	}

	paddedLen := blocks * 16
	metaBytes := make([]byte, 1+paddedLen)
	metaBytes[0] = byte(blocks)
	copy(metaBytes[1:], metaStr)
	// Remaining bytes are already zero (padding)

	_, err := w.Write(metaBytes)
	return err
}

// escapeMetadata escapes special characters in metadata
func escapeMetadata(s string) string {
	s = strings.ReplaceAll(s, "'", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// checkIPAllowed verifies client IP against mount restrictions
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

// matchIP checks if client IP matches allowed pattern
func matchIP(clientIP, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// Simple prefix match for CIDR-like patterns
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(clientIP, prefix)
	}
	return clientIP == pattern
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// HandleOptions handles CORS preflight requests
func (h *ListenerHandler) HandleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Accept, X-Requested-With, Content-Type, Icy-MetaData")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// ========== Status Handler ==========

// StatusHandler handles status page requests
type StatusHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
}

// NewStatusHandler creates a status handler
func NewStatusHandler(mm *stream.MountManager, cfg *config.Config) *StatusHandler {
	return &StatusHandler{mountManager: mm, config: cfg}
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

	mounts := h.mountManager.ListMounts()
	var sb strings.Builder
	sb.WriteString(`{"mounts":[`)

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

		sb.WriteString(fmt.Sprintf(`{"path":%q,"active":%t,"listeners":%d,"peak":%d,"name":%q,"genre":%q,"bitrate":%d}`,
			mountPath, mount.IsActive(), stats.Listeners, stats.PeakListeners,
			escapeJSON(meta.Name), escapeJSON(meta.Genre), meta.Bitrate))
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

// ========== Connection Pool for efficiency ==========

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, chunkSize)
	},
}

func getBuffer() []byte {
	return bufferPool.Get().([]byte)
}

func putBuffer(buf []byte) {
	bufferPool.Put(buf)
}
