// Package source handles source client connections (stream senders)
package source

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/stream"
)

// Source streaming constants - optimized for consistent behavior
const (
	// Read buffer size for source connections
	// 8KB is a good balance for audio streaming
	sourceReadBufferSize = 8192

	// TCP buffer sizes for source connections
	// 64KB provides smooth buffering for up to 320kbps streams
	sourceTCPBufferSize = 65536
)

// Handler handles source client connections
type Handler struct {
	mountManager *stream.MountManager
	config       *config.Config
	logger       *log.Logger
	mu           sync.RWMutex
}

// NewHandler creates a new source handler
func NewHandler(mm *stream.MountManager, cfg *config.Config, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{
		mountManager: mm,
		config:       cfg,
		logger:       logger,
	}
}

// SetConfig updates the handler's configuration (for hot-reload support)
func (h *Handler) SetConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
	h.logger.Println("Source handler configuration updated")
}

// getConfig returns the current config with proper locking
func (h *Handler) getConfig() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// HandleSource handles incoming source connections
// This supports both HTTP PUT and the legacy Icecast SOURCE method
func (h *Handler) HandleSource(w http.ResponseWriter, r *http.Request) {
	// Extract mount path
	mountPath := r.URL.Path
	if mountPath == "" {
		mountPath = "/"
	}

	h.logger.Printf("Source connection attempt: %s from %s", mountPath, r.RemoteAddr)

	// Authenticate source
	if !h.authenticate(r) {
		h.logger.Printf("Source authentication failed for %s from %s", mountPath, r.RemoteAddr)
		w.Header().Set("WWW-Authenticate", `Basic realm="GoCast Source"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get or create mount
	mount, err := h.mountManager.GetOrCreateMount(mountPath)
	if err != nil {
		h.logger.Printf("Failed to create mount %s: %v", mountPath, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Check if source is already connected
	if mount.IsActive() {
		h.logger.Printf("Source already connected to %s", mountPath)
		http.Error(w, "Source already connected", http.StatusConflict)
		return
	}

	// Start source
	clientIP := getClientIP(r)
	if err := mount.StartSource(clientIP); err != nil {
		h.logger.Printf("Failed to start source for %s: %v", mountPath, err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Parse and set metadata from headers
	h.parseMetadata(r, mount)

	h.logger.Printf("Source connected: %s from %s", mountPath, clientIP)

	// For PUT requests, we need to hijack the connection to send an immediate
	// response and then continue reading the stream data. This is required
	// because Icecast clients (like ffmpeg) expect an immediate 200 OK before
	// they start sending audio data.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		h.logger.Printf("Hijacking not supported for %s", mountPath)
		mount.StopSource()
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		h.logger.Printf("Failed to hijack connection for %s: %v", mountPath, err)
		mount.StopSource()
		http.Error(w, "Streaming error", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Apply TCP optimizations for source streaming
	optimizeTCPConnection(conn)

	// Send HTTP 200 OK response immediately - this is what Icecast does
	// and what clients expect before they start streaming
	bufrw.WriteString("HTTP/1.0 200 OK\r\n")
	bufrw.WriteString("\r\n")
	bufrw.Flush()

	// Now stream from the connection - the client will send audio data
	h.logger.Printf("DEBUG: Starting to stream from source connection for %s", mountPath)
	h.streamFromConnection(conn, bufrw.Reader, mount, mountPath)

	// Cleanup
	mount.StopSource()
	h.logger.Printf("Source disconnected: %s", mountPath)
}

// HandleSourceMethod handles the legacy Icecast SOURCE method
func (h *Handler) HandleSourceMethod(w http.ResponseWriter, r *http.Request) {
	// For SOURCE method, we need to send an HTTP/1.0 OK response first
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Apply TCP optimizations for source streaming
	optimizeTCPConnection(conn)

	// Extract mount path
	mountPath := r.URL.Path
	if mountPath == "" {
		mountPath = "/"
	}

	h.logger.Printf("SOURCE method connection: %s from %s", mountPath, r.RemoteAddr)

	// Authenticate
	if !h.authenticate(r) {
		h.logger.Printf("SOURCE authentication failed for %s", mountPath)
		bufrw.WriteString("HTTP/1.0 401 Unauthorized\r\n")
		bufrw.WriteString("WWW-Authenticate: Basic realm=\"GoCast Source\"\r\n")
		bufrw.WriteString("\r\n")
		bufrw.Flush()
		return
	}

	// Get or create mount
	mount, err := h.mountManager.GetOrCreateMount(mountPath)
	if err != nil {
		h.logger.Printf("Failed to create mount %s: %v", mountPath, err)
		bufrw.WriteString("HTTP/1.0 503 Service Unavailable\r\n\r\n")
		bufrw.Flush()
		return
	}

	// Check if source already connected
	if mount.IsActive() {
		h.logger.Printf("Source already connected to %s", mountPath)
		bufrw.WriteString("HTTP/1.0 409 Conflict\r\n\r\n")
		bufrw.Flush()
		return
	}

	// Start source
	clientIP := getClientIP(r)
	if err := mount.StartSource(clientIP); err != nil {
		bufrw.WriteString("HTTP/1.0 409 Conflict\r\n\r\n")
		bufrw.Flush()
		return
	}

	// Parse metadata
	h.parseMetadata(r, mount)

	// Send OK response
	bufrw.WriteString("HTTP/1.0 200 OK\r\n\r\n")
	bufrw.Flush()

	h.logger.Printf("SOURCE connected: %s from %s", mountPath, clientIP)

	// Stream data from the connection
	h.streamFromReader(bufrw.Reader, mount, mountPath)

	mount.StopSource()
	h.logger.Printf("SOURCE disconnected: %s", mountPath)
}

// authenticate checks source credentials
func (h *Handler) authenticate(r *http.Request) bool {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if auth == "" {
		// Check for ice-* headers (legacy Icecast authentication)
		iceUser := r.Header.Get("ice-username")
		icePass := r.Header.Get("ice-password")
		if icePass != "" {
			return h.checkCredentials(iceUser, icePass, r.URL.Path)
		}
		return false
	}

	// Parse Basic auth
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[6:])
	if err != nil {
		return false
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}

	return h.checkCredentials(parts[0], parts[1], r.URL.Path)
}

// checkCredentials verifies username and password
func (h *Handler) checkCredentials(username, password, mountPath string) bool {
	cfg := h.getConfig()

	// Check mount-specific password first
	if mount, exists := cfg.Mounts[mountPath]; exists {
		if mount.Password != "" && password == mount.Password {
			return true
		}
	}

	// Check global source password
	// Username can be "source" or empty for Icecast compatibility
	if username == "" || username == "source" {
		return password == cfg.Auth.SourcePassword
	}

	// Check admin credentials
	if username == cfg.Auth.AdminUser {
		return password == cfg.Auth.AdminPassword
	}

	return false
}

// parseMetadata extracts metadata from request headers
// Falls back to mount config defaults if headers not provided
func (h *Handler) parseMetadata(r *http.Request, mount *stream.Mount) {
	meta := &stream.Metadata{}

	// Start with mount config defaults
	if mount.Config != nil {
		meta.Name = mount.Config.StreamName
		meta.Description = mount.Config.Description
		meta.Genre = mount.Config.Genre
		meta.URL = mount.Config.URL
		meta.Bitrate = mount.Config.Bitrate
		meta.Public = mount.Config.Public
		meta.ContentType = mount.Config.Type
		// Set default stream title from config
		if mount.Config.StreamName != "" {
			meta.StreamTitle = mount.Config.StreamName
		}
	}

	// ICY headers override defaults
	if v := r.Header.Get("ice-name"); v != "" {
		meta.Name = v
		meta.StreamTitle = v // Also use as initial stream title
		// Try to parse "Song - Artist" format from ice-name
		if parts := strings.SplitN(v, " - ", 2); len(parts) == 2 {
			meta.Title = strings.TrimSpace(parts[0])
			meta.Artist = strings.TrimSpace(parts[1])
			meta.StreamTitle = v
		}
	}
	// Explicit artist header (ice-artist is non-standard but useful)
	if v := r.Header.Get("ice-artist"); v != "" {
		meta.Artist = v
	}
	// Explicit title header
	if v := r.Header.Get("ice-title"); v != "" {
		meta.Title = v
		if meta.StreamTitle == "" || meta.StreamTitle == meta.Name {
			if meta.Artist != "" {
				meta.StreamTitle = meta.Title + " - " + meta.Artist
			} else {
				meta.StreamTitle = meta.Title
			}
		}
	}
	if v := r.Header.Get("ice-description"); v != "" {
		meta.Description = v
	}
	if v := r.Header.Get("ice-genre"); v != "" {
		meta.Genre = v
	}
	if v := r.Header.Get("ice-url"); v != "" {
		meta.URL = v
	}
	if v := r.Header.Get("ice-bitrate"); v != "" {
		if bitrate, err := strconv.Atoi(v); err == nil && bitrate > 0 {
			meta.Bitrate = bitrate
		}
	}
	// Also check Audio-Info header for bitrate (ffmpeg sends this)
	if v := r.Header.Get("Audio-Info"); v != "" {
		for _, part := range strings.Split(v, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 && strings.ToLower(kv[0]) == "bitrate" {
				if bitrate, err := strconv.Atoi(kv[1]); err == nil && bitrate > 0 {
					meta.Bitrate = bitrate
				}
			}
		}
	}
	if v := r.Header.Get("ice-public"); v != "" {
		meta.Public = v == "1" || v == "true"
	}
	if v := r.Header.Get("ice-audio-info"); v != "" {
		// Parse audio info for bitrate
		// Format: ice-audio-info=bitrate=320;samplerate=44100
		for _, part := range strings.Split(v, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 && strings.ToLower(kv[0]) == "bitrate" {
				if bitrate, err := strconv.Atoi(kv[1]); err == nil && bitrate > 0 {
					meta.Bitrate = bitrate
				}
			}
		}
	}
	// Default to 320 if still using config default of 128 (common misconfiguration)
	if meta.Bitrate == 128 || meta.Bitrate == 0 {
		// Check Content-Type for hints
		ct := strings.ToLower(meta.ContentType)
		if strings.Contains(ct, "mp3") || strings.Contains(ct, "mpeg") {
			meta.Bitrate = 320 // Assume high quality for MP3
		}
	}

	// Content-Type from header or default
	if v := r.Header.Get("Content-Type"); v != "" {
		meta.ContentType = v
	}
	if meta.ContentType == "" {
		meta.ContentType = "audio/mpeg"
	}

	// If still no stream title, use mount path
	if meta.StreamTitle == "" {
		meta.StreamTitle = "Live Stream on " + mount.Path
	}

	// Log all received headers for debugging
	h.logger.Printf("Source headers for %s:", mount.Path)
	for key, values := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "ice") ||
			strings.HasPrefix(strings.ToLower(key), "audio") ||
			strings.ToLower(key) == "content-type" {
			h.logger.Printf("  %s: %v", key, values)
		}
	}

	mount.UpdateMetadata(meta)
	h.logger.Printf("Mount %s metadata: name=%s, title=%s, bitrate=%d",
		mount.Path, meta.Name, meta.StreamTitle, meta.Bitrate)
}

// streamSource reads data from the request body and writes to the mount
func (h *Handler) streamSource(r *http.Request, mount *stream.Mount, mountPath string) {
	buf := make([]byte, 8192)
	var totalBytes int64

	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			written, writeErr := mount.WriteData(buf[:n])
			if writeErr != nil {
				h.logger.Printf("Error writing to mount %s: %v", mountPath, writeErr)
				return
			}
			totalBytes += int64(written)
		}

		if err != nil {
			if err != io.EOF {
				h.logger.Printf("Error reading from source %s: %v", mountPath, err)
			}
			return
		}
	}
}

// streamFromReader reads data from a buffered reader and writes to the mount
func (h *Handler) streamFromReader(reader *bufio.Reader, mount *stream.Mount, mountPath string) {
	buf := make([]byte, 8192)
	totalBytes := int64(0)
	readCount := 0

	h.logger.Printf("DEBUG: streamFromReader started for %s", mountPath)

	for mount.IsActive() {
		n, err := reader.Read(buf)
		readCount++

		if readCount <= 5 || readCount%1000 == 0 {
			h.logger.Printf("DEBUG: Source %s read #%d: %d bytes, err=%v", mountPath, readCount, n, err)
		}

		if n > 0 {
			_, writeErr := mount.WriteData(buf[:n])
			if writeErr != nil {
				h.logger.Printf("Error writing to mount %s: %v", mountPath, writeErr)
				return
			}
			totalBytes += int64(n)
		}

		if err != nil {
			if err != io.EOF {
				h.logger.Printf("Error reading from SOURCE %s: %v", mountPath, err)
			}
			h.logger.Printf("DEBUG: Source %s ended after %d reads, %d total bytes", mountPath, readCount, totalBytes)
			return
		}
	}

	h.logger.Printf("DEBUG: Source %s loop ended (mount inactive), %d total bytes", mountPath, totalBytes)
}

// streamFromConnection reads data from a hijacked connection and writes to the mount
// It first drains any buffered data from the bufio.Reader, then reads directly from the connection
func (h *Handler) streamFromConnection(conn net.Conn, bufReader *bufio.Reader, mount *stream.Mount, mountPath string) {
	// BULLETPROOF: Use 16KB buffer for efficient reads
	// This matches typical network MTU multiples and reduces syscall overhead
	buf := make([]byte, 16384)
	totalBytes := int64(0)
	readCount := 0

	h.logger.Printf("DEBUG: streamFromConnection started for %s", mountPath)

	// Timing debug: track gaps in source data
	var lastReadTime time.Time
	var maxGapMs int64
	var gapCount int

	// Set a generous read deadline to detect dead connections
	// We'll reset this after each successful read
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// First, drain any buffered data from the bufio.Reader
	for mount.IsActive() {
		buffered := bufReader.Buffered()
		if buffered == 0 {
			h.logger.Printf("DEBUG: Source %s no more buffered data, switching to direct connection read", mountPath)
			break
		}

		toRead := buffered
		if toRead > len(buf) {
			toRead = len(buf)
		}

		n, err := bufReader.Read(buf[:toRead])
		readCount++

		if readCount <= 5 {
			h.logger.Printf("DEBUG: Source %s buffered read #%d: %d bytes, err=%v", mountPath, readCount, n, err)
		}

		if n > 0 {
			_, writeErr := mount.WriteData(buf[:n])
			if writeErr != nil {
				h.logger.Printf("Error writing to mount %s: %v", mountPath, writeErr)
				return
			}
			totalBytes += int64(n)
		}

		if err != nil {
			if err != io.EOF {
				h.logger.Printf("Error reading buffered data from SOURCE %s: %v", mountPath, err)
			}
			return
		}
	}

	// Now read directly from the connection - BULLETPROOF version
	lastReadTime = time.Now()
	for mount.IsActive() {
		// Reset read deadline before each read
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := conn.Read(buf)
		readCount++
		now := time.Now()

		// Track timing gaps in source data
		if !lastReadTime.IsZero() && n > 0 {
			gapMs := now.Sub(lastReadTime).Milliseconds()
			if gapMs > maxGapMs {
				maxGapMs = gapMs
			}
			// Log significant gaps (>100ms could indicate source issues)
			if gapMs > 100 {
				gapCount++
				h.logger.Printf("WARNING: Source %s gap detected: %dms between reads (read #%d, %d bytes, total gaps: %d, max gap: %dms)",
					mountPath, gapMs, readCount, n, gapCount, maxGapMs)
			}
		}

		if readCount <= 10 || readCount%1000 == 0 {
			h.logger.Printf("DEBUG: Source %s direct read #%d: %d bytes, err=%v, maxGap=%dms, gapCount=%d",
				mountPath, readCount, n, err, maxGapMs, gapCount)
		}

		if n > 0 {
			lastReadTime = now
			// Write immediately to buffer - this triggers instant broadcast to all listeners
			_, writeErr := mount.WriteData(buf[:n])
			if writeErr != nil {
				h.logger.Printf("Error writing to mount %s: %v", mountPath, writeErr)
				return
			}
			totalBytes += int64(n)
		}

		if err != nil {
			// Check for timeout - this is expected, just continue
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is OK - source might be pausing between tracks
				// Just continue and try again
				continue
			}
			if err != io.EOF {
				h.logger.Printf("Error reading from SOURCE %s: %v", mountPath, err)
			}
			h.logger.Printf("DEBUG: Source %s ended after %d reads, %d total bytes", mountPath, readCount, totalBytes)
			return
		}
	}

	h.logger.Printf("DEBUG: Source %s loop ended (mount inactive), %d total bytes, maxGap=%dms, totalGaps=%d", mountPath, totalBytes, maxGapMs, gapCount)
}

// getClientIP extracts the client IP from the request
// optimizeTCPConnection applies TCP optimizations for streaming connections
// This ensures consistent behavior for both HTTP and HTTPS source connections
func optimizeTCPConnection(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Disable Nagle's algorithm for low latency
		// This is critical for real-time audio streaming
		tcpConn.SetNoDelay(true)

		// Enable TCP keep-alive to detect dead connections
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)

		// Set buffer sizes for smooth streaming
		tcpConn.SetReadBuffer(sourceTCPBufferSize)
		tcpConn.SetWriteBuffer(sourceTCPBufferSize)
	}
}

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

// MetadataHandler handles metadata update requests
type MetadataHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
	logger       *log.Logger
	mu           sync.RWMutex
}

// NewMetadataHandler creates a new metadata handler
func NewMetadataHandler(mm *stream.MountManager, cfg *config.Config, logger *log.Logger) *MetadataHandler {
	if logger == nil {
		logger = log.Default()
	}
	return &MetadataHandler{
		mountManager: mm,
		config:       cfg,
		logger:       logger,
	}
}

// SetConfig updates the handler's configuration (for hot-reload support)
func (h *MetadataHandler) SetConfig(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = cfg
	h.logger.Println("Metadata handler configuration updated")
}

// getConfig returns the current config with proper locking
func (h *MetadataHandler) getConfig() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// HandleMetadataUpdate handles /admin/metadata requests
// Compatible with Icecast clients (RadioBOSS, BUTT, etc.)
// Accepts source password, mount-specific password, or admin credentials
func (h *MetadataHandler) HandleMetadataUpdate(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	mount := r.URL.Query().Get("mount")
	if mount == "" {
		http.Error(w, "Missing mount parameter", http.StatusBadRequest)
		return
	}

	// Authenticate
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="GoCast"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check credentials (source or admin)
	if !h.checkCredentials(username, password, mount) {
		w.Header().Set("WWW-Authenticate", `Basic realm="GoCast"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get mount
	m := h.mountManager.GetMount(mount)
	if m == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	// Update metadata
	mode := r.URL.Query().Get("mode")
	if mode != "updinfo" {
		http.Error(w, "Invalid mode", http.StatusBadRequest)
		return
	}

	song := r.URL.Query().Get("song")
	if song == "" {
		song = r.URL.Query().Get("title")
	}

	if song != "" {
		m.SetMetadata(song)
		h.logger.Printf("Metadata updated for %s: %s", mount, song)
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, "<?xml version=\"1.0\"?>\n<iceresponse><message>Metadata update successful</message><return>1</return></iceresponse>")
}

// checkCredentials verifies credentials for metadata updates
// Accepts: admin credentials, source password, or mount-specific password
func (h *MetadataHandler) checkCredentials(username, password, mountPath string) bool {
	cfg := h.getConfig()

	// Check admin credentials first
	if username == cfg.Auth.AdminUser && password == cfg.Auth.AdminPassword {
		return true
	}

	// Check mount-specific password (any username)
	if mount, exists := cfg.Mounts[mountPath]; exists {
		if mount.Password != "" && password == mount.Password {
			return true
		}
	}

	// Check global source password (any username)
	if password == cfg.Auth.SourcePassword {
		return true
	}

	return false
}

// ICYMetadataWriter wraps a writer to inject ICY metadata
type ICYMetadataWriter struct {
	writer        io.Writer
	interval      int
	byteCount     int
	mount         *stream.Mount
	lastMetadata  string
	metadataReady bool
}

// NewICYMetadataWriter creates a new ICY metadata writer
func NewICYMetadataWriter(w io.Writer, interval int, mount *stream.Mount) *ICYMetadataWriter {
	return &ICYMetadataWriter{
		writer:   w,
		interval: interval,
		mount:    mount,
	}
}

// Write writes data with ICY metadata injection
func (w *ICYMetadataWriter) Write(p []byte) (int, error) {
	if w.interval <= 0 {
		return w.writer.Write(p)
	}

	written := 0
	for len(p) > 0 {
		// Calculate bytes until next metadata point
		bytesUntilMeta := w.interval - (w.byteCount % w.interval)

		if bytesUntilMeta > len(p) {
			// Write all data
			n, err := w.writer.Write(p)
			w.byteCount += n
			written += n
			return written, err
		}

		// Write data up to metadata point
		n, err := w.writer.Write(p[:bytesUntilMeta])
		w.byteCount += n
		written += n
		if err != nil {
			return written, err
		}
		p = p[bytesUntilMeta:]

		// Write metadata
		if err := w.writeMetadata(); err != nil {
			return written, err
		}
	}

	return written, nil
}

// writeMetadata writes ICY metadata block
func (w *ICYMetadataWriter) writeMetadata() error {
	meta := w.mount.GetMetadata()
	streamTitle := meta.StreamTitle

	// Check if metadata has changed
	if streamTitle == w.lastMetadata {
		// No change, write empty metadata block
		_, err := w.writer.Write([]byte{0})
		return err
	}

	w.lastMetadata = streamTitle

	// Format: StreamTitle='Song Title';
	metaStr := fmt.Sprintf("StreamTitle='%s';", streamTitle)

	// Calculate block size (16-byte blocks)
	blockSize := (len(metaStr) + 15) / 16
	if blockSize > 255 {
		blockSize = 255
		metaStr = metaStr[:255*16]
	}

	// Pad to full block
	padding := blockSize*16 - len(metaStr)
	metaStr += strings.Repeat("\x00", padding)

	// Write block size and metadata
	if _, err := w.writer.Write([]byte{byte(blockSize)}); err != nil {
		return err
	}
	_, err := w.writer.Write([]byte(metaStr))
	return err
}

// KeepAlive sends periodic keepalive for sources
func KeepAlive(mount *stream.Mount, interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Just check if mount is still active
			if !mount.IsActive() {
				return
			}
		}
	}
}
