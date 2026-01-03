// Package server provides HTTP/HTTPS server functionality for GoCast
//
// This file implements BULLETPROOF audio streaming infrastructure.
// Every decision here prioritizes reliable, uninterrupted audio delivery.
//
// Key principles:
// 1. IMMEDIATE DELIVERY - Data goes to the client the moment it's available
// 2. NO BUFFERING - Buffering causes lag and skipping, we eliminate it everywhere
// 3. CONSISTENT BEHAVIOR - HTTP and HTTPS work identically
// 4. GRACEFUL DEGRADATION - Handle errors without disrupting other listeners
// 5. SINGLE SOURCE OF TRUTH - One configuration for all servers
package server

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gocast/gocast/internal/config"
)

// =============================================================================
// STREAMING CONSTANTS - Carefully tuned for audio streaming
// =============================================================================

const (
	// StreamWriteSize is the optimal write size for streaming
	// 1KB = ~25ms of audio at 320kbps - small enough for low latency,
	// large enough to be efficient. This aligns well with typical
	// audio frame sizes (MP3 frames are ~417 bytes at 320kbps)
	StreamWriteSize = 1024

	// StreamReadSize is how much we read from the buffer at once
	// 8KB = ~200ms at 320kbps - gives us efficient reads while
	// keeping memory usage reasonable
	StreamReadSize = 8192

	// TCPBufferSize for streaming connections
	// 64KB is optimal for most networks - large enough to handle
	// bursts without blocking, small enough to keep latency low
	TCPBufferSize = 65536

	// KeepAlivePeriod for TCP connections
	// 30 seconds is aggressive enough to detect dead connections
	// without causing unnecessary traffic
	KeepAlivePeriod = 30 * time.Second

	// FlushDeadline is the maximum time to wait for a flush
	// If a client can't receive data in 5 seconds, they're too slow
	FlushDeadline = 5 * time.Second

	// WriteDeadline for individual writes
	// Generous timeout for slow connections, but not infinite
	WriteDeadline = 10 * time.Second
)

// =============================================================================
// STREAM WRITER - The heart of bulletproof streaming
// =============================================================================

// StreamWriter wraps an http.ResponseWriter with bulletproof streaming behavior.
// It ensures data is delivered immediately to clients without buffering.
//
// Key features:
// - Immediate flush after every write
// - Graceful error handling
// - Metrics tracking
// - Connection health monitoring
type StreamWriter struct {
	w          http.ResponseWriter
	flusher    http.Flusher
	conn       net.Conn // Underlying TCP connection (if available)
	controller responseController
	mu         sync.Mutex // Protects writes

	// Metrics
	bytesWritten int64
	writeCount   int64
	errorCount   int64
	lastWrite    time.Time

	// State
	closed    bool
	lastError error
}

// responseController interface for Go 1.20+ ResponseController
type responseController interface {
	Flush() error
	SetWriteDeadline(deadline time.Time) error
}

// NewStreamWriter creates a bulletproof stream writer
func NewStreamWriter(w http.ResponseWriter) *StreamWriter {
	sw := &StreamWriter{
		w:         w,
		lastWrite: time.Now(),
	}

	// Get flusher - essential for streaming
	if f, ok := w.(http.Flusher); ok {
		sw.flusher = f
	}

	// Try to get the underlying connection for TCP optimizations
	// This works with hijacked connections or when ResponseWriter
	// implements the optional interfaces
	if conn := getUnderlyingConn(w); conn != nil {
		sw.conn = conn
		optimizeConnForStreaming(conn)
	}

	// Try to get ResponseController (Go 1.20+)
	sw.controller = newResponseController(w)

	return sw
}

// Write writes data to the client and immediately flushes
// This is the most critical function - it must be fast and reliable
func (sw *StreamWriter) Write(data []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return 0, io.ErrClosedPipe
	}

	if len(data) == 0 {
		return 0, nil
	}

	// Set write deadline if we have a controller
	if sw.controller != nil {
		sw.controller.SetWriteDeadline(time.Now().Add(WriteDeadline))
	}

	// Write the data
	n, err := sw.w.Write(data)

	if err != nil {
		sw.errorCount++
		sw.lastError = err
		return n, err
	}

	// Update metrics
	sw.bytesWritten += int64(n)
	sw.writeCount++
	sw.lastWrite = time.Now()

	// CRITICAL: Flush immediately after every write
	// This is what makes streaming smooth - no buffering!
	sw.flush()

	return n, nil
}

// WriteImmediate writes data with maximum priority
// Use this for critical data that must go out NOW
func (sw *StreamWriter) WriteImmediate(data []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return 0, io.ErrClosedPipe
	}

	// Set aggressive deadline
	if sw.controller != nil {
		sw.controller.SetWriteDeadline(time.Now().Add(FlushDeadline))
	}

	n, err := sw.w.Write(data)
	if err != nil {
		sw.errorCount++
		sw.lastError = err
		return n, err
	}

	sw.bytesWritten += int64(n)
	sw.writeCount++
	sw.lastWrite = time.Now()

	// Force flush
	sw.flush()

	return n, nil
}

// flush forces data to be sent to the client
func (sw *StreamWriter) flush() {
	// Try ResponseController first (most reliable)
	if sw.controller != nil {
		sw.controller.Flush()
		return
	}

	// Fall back to http.Flusher
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

// Flush explicitly flushes pending data
func (sw *StreamWriter) Flush() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.flush()
}

// Close marks the writer as closed
func (sw *StreamWriter) Close() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.closed = true
	return nil
}

// Stats returns streaming statistics
func (sw *StreamWriter) Stats() (bytesWritten, writeCount, errorCount int64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.bytesWritten, sw.writeCount, sw.errorCount
}

// LastError returns the last error encountered
func (sw *StreamWriter) LastError() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.lastError
}

// IsHealthy returns true if the writer is functioning properly
func (sw *StreamWriter) IsHealthy() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return false
	}

	// If we haven't written in a while but have errors, unhealthy
	if sw.errorCount > 0 && time.Since(sw.lastWrite) > 5*time.Second {
		return false
	}

	return true
}

// =============================================================================
// TCP CONNECTION OPTIMIZATION
// =============================================================================

// optimizeConnForStreaming applies TCP-level optimizations for audio streaming
func optimizeConnForStreaming(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		// Try to unwrap if it's a TLS connection
		if tlsConn, ok := conn.(*tls.Conn); ok {
			if nc := tlsConn.NetConn(); nc != nil {
				if tc, ok := nc.(*net.TCPConn); ok {
					tcpConn = tc
				}
			}
		}
	}

	if tcpConn == nil {
		return
	}

	// CRITICAL: Disable Nagle's algorithm
	// Nagle buffers small writes to combine them - terrible for streaming!
	// We want every write to go out immediately
	tcpConn.SetNoDelay(true)

	// Enable TCP keep-alive to detect dead connections
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(KeepAlivePeriod)

	// Set buffer sizes for smooth streaming
	// Large enough to handle bursts, small enough to avoid lag
	tcpConn.SetWriteBuffer(TCPBufferSize)
	tcpConn.SetReadBuffer(TCPBufferSize)
}

// getUnderlyingConn tries to extract the underlying net.Conn from a ResponseWriter
// NOTE: We cannot use Hijack() here as it would steal the connection from the HTTP handler
// Instead, we return nil and rely on other optimization methods
func getUnderlyingConn(w http.ResponseWriter) net.Conn {
	// We intentionally do NOT hijack the connection here
	// Hijacking steals the connection from the HTTP handler, breaking streaming
	// TCP optimizations are applied at the listener level instead
	return nil
}

// =============================================================================
// RESPONSE CONTROLLER WRAPPER
// =============================================================================

// goResponseController wraps the Go 1.20+ http.ResponseController
type goResponseController struct {
	rc *http.ResponseController
}

func newResponseController(w http.ResponseWriter) responseController {
	rc := http.NewResponseController(w)
	return &goResponseController{rc: rc}
}

func (c *goResponseController) Flush() error {
	return c.rc.Flush()
}

func (c *goResponseController) SetWriteDeadline(deadline time.Time) error {
	return c.rc.SetWriteDeadline(deadline)
}

// =============================================================================
// SERVER CONFIGURATION - Single Source of Truth
// =============================================================================

// StreamingServerConfig holds all configuration for creating optimized streaming servers
type StreamingServerConfig struct {
	Address           string
	Handler           http.Handler
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	WriteTimeout      time.Duration
	ReadTimeout       time.Duration
	TLSConfig         *tls.Config
	ConnState         func(net.Conn, http.ConnState)
	MaxHeaderBytes    int
}

// DefaultStreamingServerConfig returns optimal defaults for audio streaming
func DefaultStreamingServerConfig(cfg *config.Config) StreamingServerConfig {
	headerTimeout := cfg.Limits.HeaderTimeout
	if headerTimeout == 0 {
		headerTimeout = 5 * time.Second
	}

	idleTimeout := cfg.Limits.ClientTimeout
	if idleTimeout == 0 {
		idleTimeout = 60 * time.Second
	}

	return StreamingServerConfig{
		ReadHeaderTimeout: headerTimeout,
		IdleTimeout:       idleTimeout,
		WriteTimeout:      0,       // NO write timeout for streaming
		ReadTimeout:       0,       // NO read timeout for streaming
		MaxHeaderBytes:    1 << 20, // 1MB
	}
}

// NewStreamingServer creates an http.Server optimized for live audio streaming
func NewStreamingServer(cfg StreamingServerConfig) *http.Server {
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           cfg.Handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		ConnState:         cfg.ConnState,
	}

	// Apply TLS configuration if provided
	if cfg.TLSConfig != nil {
		server.TLSConfig = cfg.TLSConfig
	}

	return server
}

// =============================================================================
// TLS CONFIGURATION - Optimized for Streaming
// =============================================================================

// OptimizedTLSConfig returns a TLS configuration optimized for low-latency streaming
func OptimizedTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,

		// Let the client choose - modern clients make good choices
		PreferServerCipherSuites: false,

		// Fast cipher suites only
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},

		// X25519 is fastest
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},

		// Enable session resumption for faster reconnects
		SessionTicketsDisabled: false,

		// No renegotiation needed
		Renegotiation: tls.RenegotiateNever,

		// CRITICAL: HTTP/1.1 only - HTTP/2 flow control breaks audio streaming
		NextProtos: []string{"http/1.1"},
	}
}

// OptimizedTLSConfigWithCert returns an optimized TLS config with the given certificate
func OptimizedTLSConfigWithCert(cert tls.Certificate) *tls.Config {
	cfg := OptimizedTLSConfig()
	cfg.Certificates = []tls.Certificate{cert}
	return cfg
}

// OptimizedTLSConfigWithGetCert returns an optimized TLS config with a dynamic certificate getter
func OptimizedTLSConfigWithGetCert(getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *tls.Config {
	cfg := OptimizedTLSConfig()
	cfg.GetCertificate = getCert
	return cfg
}

// =============================================================================
// STREAMING SERVER FACTORIES
// =============================================================================

// StreamingHTTPServer creates an HTTP server optimized for audio streaming
func StreamingHTTPServer(addr string, handler http.Handler, cfg *config.Config, connState func(net.Conn, http.ConnState)) *http.Server {
	serverCfg := DefaultStreamingServerConfig(cfg)
	serverCfg.Address = addr
	serverCfg.Handler = handler
	serverCfg.ConnState = connState
	return NewStreamingServer(serverCfg)
}

// StreamingHTTPSServer creates an HTTPS server optimized for audio streaming
func StreamingHTTPSServer(addr string, handler http.Handler, cfg *config.Config, tlsCfg *tls.Config, connState func(net.Conn, http.ConnState)) *http.Server {
	serverCfg := DefaultStreamingServerConfig(cfg)
	serverCfg.Address = addr
	serverCfg.Handler = handler
	serverCfg.TLSConfig = tlsCfg
	serverCfg.ConnState = connState
	return NewStreamingServer(serverCfg)
}

// =============================================================================
// STREAMING LISTENER - TCP listener with optimizations
// =============================================================================

// StreamingListener wraps a net.Listener with streaming optimizations
type StreamingListener struct {
	net.Listener
}

// NewStreamingListener creates a TCP listener optimized for streaming
func NewStreamingListener(addr string) (*StreamingListener, error) {
	lc := net.ListenConfig{
		KeepAlive: KeepAlivePeriod,
	}

	ln, err := lc.Listen(nil, "tcp", addr)
	if err != nil {
		return nil, err
	}

	return &StreamingListener{Listener: ln}, nil
}

// Accept accepts a connection and optimizes it for streaming
func (sl *StreamingListener) Accept() (net.Conn, error) {
	conn, err := sl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	optimizeConnForStreaming(conn)
	return conn, nil
}

// =============================================================================
// HANDLER WRAPPERS
// =============================================================================

// WrapHandler creates a handler that adds streaming-optimized headers
func WrapHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable any proxy buffering
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		handler.ServeHTTP(w, r)
	})
}

// HSTSHandler wraps a handler to add HSTS header for HTTPS
func HSTSHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		handler.ServeHTTP(w, r)
	})
}

// =============================================================================
// BUFFERED WRITER FOR HIGH-THROUGHPUT STREAMING
// =============================================================================

// BufferedStreamWriter provides buffered writing with periodic flushing
// Use this when you want to batch small writes for efficiency
type BufferedStreamWriter struct {
	sw  *StreamWriter
	buf *bufio.Writer
	mu  sync.Mutex

	// Auto-flush settings
	maxBytes   int
	maxLatency time.Duration
	lastFlush  time.Time
	pending    int
}

// NewBufferedStreamWriter creates a buffered stream writer
// bufSize: buffer size in bytes (recommend 4-8KB for audio)
// maxLatency: maximum time before forcing a flush (recommend 20-50ms)
func NewBufferedStreamWriter(w http.ResponseWriter, bufSize int, maxLatency time.Duration) *BufferedStreamWriter {
	sw := NewStreamWriter(w)
	return &BufferedStreamWriter{
		sw:         sw,
		buf:        bufio.NewWriterSize(sw, bufSize),
		maxBytes:   bufSize / 2, // Flush at half-full
		maxLatency: maxLatency,
		lastFlush:  time.Now(),
	}
}

// Write writes data, automatically flushing when needed
func (bw *BufferedStreamWriter) Write(data []byte) (int, error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	n, err := bw.buf.Write(data)
	if err != nil {
		return n, err
	}

	bw.pending += n

	// Flush if buffer is getting full or latency exceeded
	shouldFlush := bw.pending >= bw.maxBytes ||
		time.Since(bw.lastFlush) >= bw.maxLatency

	if shouldFlush {
		if err := bw.buf.Flush(); err != nil {
			return n, err
		}
		bw.pending = 0
		bw.lastFlush = time.Now()
	}

	return n, nil
}

// Flush forces all pending data to be written
func (bw *BufferedStreamWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if err := bw.buf.Flush(); err != nil {
		return err
	}
	bw.pending = 0
	bw.lastFlush = time.Now()
	return nil
}

// Close flushes and closes the writer
func (bw *BufferedStreamWriter) Close() error {
	bw.Flush()
	return bw.sw.Close()
}
