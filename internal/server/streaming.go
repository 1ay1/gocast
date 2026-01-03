// Package server provides HTTP/HTTPS server functionality for GoCast
// This file provides a unified streaming server builder that ensures
// consistent behavior between HTTP and HTTPS for live audio streaming.
package server

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gocast/gocast/internal/config"
)

// StreamingServerConfig holds all configuration for creating optimized streaming servers
type StreamingServerConfig struct {
	// Address to listen on (e.g., "0.0.0.0:8000")
	Address string

	// Handler to serve requests
	Handler http.Handler

	// Timeouts
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	WriteTimeout      time.Duration // 0 for streaming (no timeout)
	ReadTimeout       time.Duration // 0 for streaming (no timeout)

	// TLS configuration (nil for HTTP)
	TLSConfig *tls.Config

	// Connection state handler (optional)
	ConnState func(net.Conn, http.ConnState)

	// MaxHeaderBytes limits the size of request headers
	MaxHeaderBytes int
}

// DefaultStreamingServerConfig returns optimal defaults for audio streaming
func DefaultStreamingServerConfig(cfg *config.Config) StreamingServerConfig {
	headerTimeout := cfg.Limits.HeaderTimeout
	if headerTimeout == 0 {
		headerTimeout = 5 * time.Second
	}

	idleTimeout := cfg.Limits.ClientTimeout
	if idleTimeout == 0 {
		idleTimeout = 30 * time.Second
	}

	return StreamingServerConfig{
		ReadHeaderTimeout: headerTimeout,
		IdleTimeout:       idleTimeout,
		WriteTimeout:      0,       // No write timeout for streaming
		ReadTimeout:       0,       // No read timeout for streaming
		MaxHeaderBytes:    1 << 20, // 1MB
		ConnState:         nil,
	}
}

// NewStreamingServer creates an http.Server optimized for live audio streaming
// This is the single source of truth for server configuration - both HTTP and HTTPS
// servers should use this function to ensure consistent behavior.
func NewStreamingServer(cfg StreamingServerConfig) *http.Server {
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           cfg.Handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		ConnState:         cfg.ConnState,
	}

	// For streaming, we don't want write timeouts as streams are long-lived
	// ReadTimeout and WriteTimeout are intentionally not set (0) for streaming
	if cfg.WriteTimeout > 0 {
		server.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.ReadTimeout > 0 {
		server.ReadTimeout = cfg.ReadTimeout
	}

	// Apply TLS configuration if provided
	if cfg.TLSConfig != nil {
		server.TLSConfig = cfg.TLSConfig
	}

	return server
}

// OptimizedTLSConfig returns a TLS configuration optimized for low-latency streaming
// This configuration prioritizes:
// - Low latency over maximum security (while still being secure)
// - Session resumption to minimize handshake overhead
// - Modern cipher suites that are fast on modern hardware
// - HTTP/1.1 only (HTTP/2 flow control causes audio streaming issues)
func OptimizedTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,

		// Prefer server cipher suites for consistency
		PreferServerCipherSuites: true,

		// Optimized cipher suites for streaming - prioritize speed
		// TLS 1.3 cipher suites are automatically used when available
		// These are for TLS 1.2 fallback
		CipherSuites: []uint16{
			// TLS 1.3 suites (automatically preferred when available)
			// - TLS_AES_128_GCM_SHA256
			// - TLS_AES_256_GCM_SHA384
			// - TLS_CHACHA20_POLY1305_SHA256

			// TLS 1.2 suites - ordered by performance
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},

		// Curve preferences - X25519 is fastest
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},

		// Session tickets for faster reconnection (session resumption)
		// This significantly reduces handshake latency for returning clients
		SessionTicketsDisabled: false,

		// Renegotiation is not needed for streaming and adds complexity
		Renegotiation: tls.RenegotiateNever,

		// CRITICAL: Disable HTTP/2 for audio streaming
		// HTTP/2 has flow control mechanisms that cause buffering and skipping
		// in continuous audio streams. HTTP/1.1 provides direct, unbuffered
		// streaming which is essential for smooth audio playback.
		// This is set via NextProtos - only advertise http/1.1
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
// This is useful for AutoSSL where certificates may be renewed at runtime
func OptimizedTLSConfigWithGetCert(getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *tls.Config {
	cfg := OptimizedTLSConfig()
	cfg.GetCertificate = getCert
	return cfg
}

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

// StreamingListener wraps a net.Listener with optimizations for streaming
type StreamingListener struct {
	net.Listener
}

// NewStreamingListener creates a TCP listener with optimizations for streaming
func NewStreamingListener(addr string) (*StreamingListener, error) {
	lc := net.ListenConfig{
		// Enable TCP keep-alive for long-lived streaming connections
		KeepAlive: 30 * time.Second,
	}

	ln, err := lc.Listen(nil, "tcp", addr)
	if err != nil {
		return nil, err
	}

	return &StreamingListener{Listener: ln}, nil
}

// Accept wraps the underlying Accept and applies streaming optimizations
func (sl *StreamingListener) Accept() (net.Conn, error) {
	conn, err := sl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Apply TCP optimizations for streaming
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Disable Nagle's algorithm for low latency
		// This sends data immediately rather than buffering small packets
		tcpConn.SetNoDelay(true)

		// Enable TCP keep-alive to detect dead connections
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)

		// Set reasonable buffer sizes for streaming audio
		// 64KB is a good balance for audio streaming at up to 320kbps
		tcpConn.SetReadBuffer(65536)
		tcpConn.SetWriteBuffer(65536)
	}

	return conn, nil
}

// WrapHandler creates a handler that adds streaming-optimized headers
// This ensures consistent behavior across HTTP and HTTPS
func WrapHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable buffering for streaming responses
		w.Header().Set("X-Accel-Buffering", "no")

		// Serve the request
		handler.ServeHTTP(w, r)
	})
}

// HSTSHandler wraps a handler to add HSTS header for HTTPS connections
// Separated from the main handler for clarity and single responsibility
func HSTSHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add HSTS header (1 year, include subdomains)
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		handler.ServeHTTP(w, r)
	})
}

// StreamingResponseWriter wraps http.ResponseWriter with streaming optimizations
type StreamingResponseWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

// NewStreamingResponseWriter creates a streaming-optimized response writer
func NewStreamingResponseWriter(w http.ResponseWriter) *StreamingResponseWriter {
	flusher, _ := w.(http.Flusher)
	return &StreamingResponseWriter{
		ResponseWriter: w,
		flusher:        flusher,
	}
}

// Write writes data and immediately flushes for low latency
func (sw *StreamingResponseWriter) Write(data []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(data)
	if err == nil && sw.flusher != nil {
		sw.flusher.Flush()
	}
	return n, err
}

// Flush explicitly flushes the response
func (sw *StreamingResponseWriter) Flush() {
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

// HasFlusher returns true if the underlying writer supports flushing
func (sw *StreamingResponseWriter) HasFlusher() bool {
	return sw.flusher != nil
}

// Unwrap returns the underlying ResponseWriter (for http.Hijacker etc)
func (sw *StreamingResponseWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}
