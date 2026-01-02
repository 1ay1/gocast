// Package source handles source client connections (stream senders)
package source

import (
	"bufio"
	"encoding/base64"
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

// Handler handles source client connections
type Handler struct {
	mountManager *stream.MountManager
	config       *config.Config
	logger       *log.Logger
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

	// Send HTTP 200 OK response immediately - this is what Icecast does
	// and what clients expect before they start streaming
	bufrw.WriteString("HTTP/1.0 200 OK\r\n")
	bufrw.WriteString("\r\n")
	bufrw.Flush()

	// Now stream from the connection - the client will send audio data
	h.streamFromReader(bufrw.Reader, mount, mountPath)

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
	// Check mount-specific password first
	if mount, exists := h.config.Mounts[mountPath]; exists {
		if mount.Password != "" && password == mount.Password {
			return true
		}
	}

	// Check global source password
	// Username can be "source" or empty for Icecast compatibility
	if username == "" || username == "source" {
		return password == h.config.Auth.SourcePassword
	}

	// Check admin credentials
	if username == h.config.Auth.AdminUser {
		return password == h.config.Auth.AdminPassword
	}

	return false
}

// parseMetadata extracts metadata from request headers
func (h *Handler) parseMetadata(r *http.Request, mount *stream.Mount) {
	meta := &stream.Metadata{}

	// ICY headers
	if v := r.Header.Get("ice-name"); v != "" {
		meta.Name = v
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
		if bitrate, err := strconv.Atoi(v); err == nil {
			meta.Bitrate = bitrate
		}
	}
	if v := r.Header.Get("ice-public"); v != "" {
		meta.Public = v == "1" || v == "true"
	}

	// Content-Type
	if v := r.Header.Get("Content-Type"); v != "" {
		meta.ContentType = v
	} else {
		// Default based on common formats
		meta.ContentType = mount.Config.Type
	}

	mount.UpdateMetadata(meta)
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

	for mount.IsActive() {
		n, err := reader.Read(buf)
		if n > 0 {
			_, writeErr := mount.WriteData(buf[:n])
			if writeErr != nil {
				h.logger.Printf("Error writing to mount %s: %v", mountPath, writeErr)
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				h.logger.Printf("Error reading from SOURCE %s: %v", mountPath, err)
			}
			return
		}
	}
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

// MetadataHandler handles metadata update requests
type MetadataHandler struct {
	mountManager *stream.MountManager
	config       *config.Config
	logger       *log.Logger
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

// HandleMetadataUpdate handles /admin/metadata requests
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
		w.Header().Set("WWW-Authenticate", `Basic realm="GoCast Admin"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check credentials (source or admin)
	if !h.checkCredentials(username, password, mount) {
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
func (h *MetadataHandler) checkCredentials(username, password, mountPath string) bool {
	// Check source credentials
	if username == "source" || username == "" {
		// Check mount-specific password
		if mount, exists := h.config.Mounts[mountPath]; exists {
			if mount.Password != "" && password == mount.Password {
				return true
			}
		}
		return password == h.config.Auth.SourcePassword
	}

	// Check admin credentials
	if username == h.config.Auth.AdminUser {
		return password == h.config.Auth.AdminPassword
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
