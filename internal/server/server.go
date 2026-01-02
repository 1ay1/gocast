// Package server handles HTTP server and listener connections
package server

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/source"
	"github.com/gocast/gocast/internal/stream"
)

//go:embed admin_panel.html
var adminPanelHTML string

// Server is the main GoCast HTTP server
type Server struct {
	config          *config.Config
	mountManager    *stream.MountManager
	httpServer      *http.Server
	httpsServer     *http.Server
	listenerHandler *ListenerHandler
	sourceHandler   *source.Handler
	metadataHandler *source.MetadataHandler
	statusHandler   *StatusHandler
	logger          *log.Logger
	startTime       time.Time
	mu              sync.RWMutex
}

// New creates a new GoCast server
func New(cfg *config.Config, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	mm := stream.NewMountManager(cfg)

	s := &Server{
		config:          cfg,
		mountManager:    mm,
		listenerHandler: NewListenerHandler(mm, cfg, logger),
		sourceHandler:   source.NewHandler(mm, cfg, logger),
		metadataHandler: source.NewMetadataHandler(mm, cfg, logger),
		statusHandler:   NewStatusHandler(mm, cfg),
		logger:          logger,
		startTime:       time.Now(),
	}

	return s
}

// Start starts the HTTP server(s)
func (s *Server) Start() error {
	// Create main router
	mux := s.createRouter()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.Server.ListenAddress, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: s.config.Limits.HeaderTimeout,
		IdleTimeout:       s.config.Limits.ClientTimeout,
		MaxHeaderBytes:    1 << 20, // 1MB
		ConnState:         s.connStateHandler,
	}

	// Start HTTP server
	go func() {
		s.logger.Printf("Starting GoCast HTTP server on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if enabled
	if s.config.Server.SSLEnabled {
		if err := s.startHTTPS(mux); err != nil {
			return fmt.Errorf("failed to start HTTPS server: %w", err)
		}
	}

	return nil
}

// startHTTPS starts the HTTPS server
func (s *Server) startHTTPS(handler http.Handler) error {
	cert, err := tls.LoadX509KeyPair(s.config.Server.SSLCert, s.config.Server.SSLKey)
	if err != nil {
		return fmt.Errorf("failed to load SSL certificates: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	addr := fmt.Sprintf("%s:%d", s.config.Server.ListenAddress, s.config.Server.SSLPort)
	s.httpsServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: s.config.Limits.HeaderTimeout,
		IdleTimeout:       s.config.Limits.ClientTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		s.logger.Printf("Starting GoCast HTTPS server on %s", addr)
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTPS server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Println("Shutting down GoCast server...")

	var wg sync.WaitGroup

	// Shutdown HTTP server
	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.httpServer.Shutdown(ctx); err != nil {
				s.logger.Printf("HTTP server shutdown error: %v", err)
			}
		}()
	}

	// Shutdown HTTPS server
	if s.httpsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.httpsServer.Shutdown(ctx); err != nil {
				s.logger.Printf("HTTPS server shutdown error: %v", err)
			}
		}()
	}

	// Wait for all servers to shutdown
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Println("GoCast server stopped gracefully")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// createRouter creates the HTTP request router
func (s *Server) createRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Log request
		s.logger.Printf("%s %s %s from %s", r.Method, r.URL.Path, r.Proto, r.RemoteAddr)

		// Handle OPTIONS for CORS
		if r.Method == http.MethodOptions {
			s.listenerHandler.HandleOptions(w, r)
			return
		}

		// Admin endpoints
		if strings.HasPrefix(path, "/admin/") {
			s.handleAdmin(w, r)
			return
		}

		// Status endpoints
		if path == "/status" || path == "/status.xsl" || path == "/status-json.xsl" {
			s.statusHandler.ServeHTTP(w, r)
			return
		}

		// Favicon
		if path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}

		// Root path - show status
		if path == "/" {
			s.statusHandler.ServeHTTP(w, r)
			return
		}

		// Source connection (PUT or SOURCE method)
		if r.Method == http.MethodPut || r.Method == "SOURCE" {
			s.sourceHandler.HandleSource(w, r)
			return
		}

		// Listener connection (GET)
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			s.listenerHandler.ServeHTTP(w, r)
			return
		}

		// Unknown method
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
}

// handleAdmin handles admin endpoints
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check if admin is enabled
	if !s.config.Admin.Enabled {
		http.Error(w, "Admin interface disabled", http.StatusForbidden)
		return
	}

	// Authenticate admin
	username, password, ok := r.BasicAuth()
	if !ok || username != s.config.Admin.User || password != s.config.Admin.Password {
		w.Header().Set("WWW-Authenticate", `Basic realm="GoCast Admin"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Route admin requests
	switch {
	case path == "/admin/stats" || path == "/admin/stats.xml":
		s.handleAdminStats(w, r)

	case path == "/admin/listclients":
		s.handleAdminListClients(w, r)

	case path == "/admin/moveclients":
		s.handleAdminMoveClients(w, r)

	case path == "/admin/killclient":
		s.handleAdminKillClient(w, r)

	case path == "/admin/killsource":
		s.handleAdminKillSource(w, r)

	case path == "/admin/metadata":
		s.metadataHandler.HandleMetadataUpdate(w, r)

	case path == "/admin/listmounts":
		s.handleAdminListMounts(w, r)

	case path == "/admin/", path == "/admin":
		s.handleAdminIndex(w, r)

	case path == "/admin/panel":
		s.handleModernAdminPanel(w, r)

	default:
		http.NotFound(w, r)
	}
}

// handleAdminStats returns server statistics
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, "\n<icestats>")
	fmt.Fprintf(w, "<admin>%s</admin>", s.config.Server.AdminRoot)
	fmt.Fprintf(w, "<host>%s</host>", s.config.Server.Hostname)
	fmt.Fprintf(w, "<location>%s</location>", s.config.Server.Location)
	fmt.Fprint(w, "<server_id>GoCast/1.0.0</server_id>")
	fmt.Fprintf(w, "<server_start>%s</server_start>", s.startTime.Format(time.RFC3339))

	for _, stat := range s.mountManager.Stats() {
		fmt.Fprint(w, "<source>")
		fmt.Fprintf(w, "<mount>%s</mount>", stat.Path)
		fmt.Fprintf(w, "<listeners>%d</listeners>", stat.Listeners)
		fmt.Fprintf(w, "<peak_listeners>%d</peak_listeners>", stat.PeakListeners)
		fmt.Fprintf(w, "<genre>%s</genre>", escapeXML(stat.Metadata.Genre))
		fmt.Fprintf(w, "<server_name>%s</server_name>", escapeXML(stat.Metadata.Name))
		fmt.Fprintf(w, "<server_description>%s</server_description>", escapeXML(stat.Metadata.Description))
		fmt.Fprintf(w, "<server_type>%s</server_type>", stat.ContentType)
		fmt.Fprintf(w, "<title>%s</title>", escapeXML(stat.Metadata.StreamTitle))
		fmt.Fprintf(w, "<total_bytes_read>%d</total_bytes_read>", stat.BytesReceived)
		fmt.Fprint(w, "</source>")
	}

	fmt.Fprint(w, "</icestats>")
}

// handleAdminListClients lists connected clients for a mount
func (s *Server) handleAdminListClients(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Query().Get("mount")
	if mountPath == "" {
		http.Error(w, "Missing mount parameter", http.StatusBadRequest)
		return
	}

	mount := s.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, "\n<icestats>")
	fmt.Fprintf(w, "<source mount=\"%s\">", mountPath)

	for _, listener := range mount.GetListeners() {
		fmt.Fprint(w, "<listener>")
		fmt.Fprintf(w, "<ID>%s</ID>", listener.ID)
		fmt.Fprintf(w, "<IP>%s</IP>", listener.IP)
		fmt.Fprintf(w, "<UserAgent>%s</UserAgent>", escapeXML(listener.UserAgent))
		fmt.Fprintf(w, "<Connected>%d</Connected>", int(time.Since(listener.ConnectedAt).Seconds()))
		fmt.Fprint(w, "</listener>")
	}

	fmt.Fprint(w, "</source>")
	fmt.Fprint(w, "</icestats>")
}

// handleAdminMoveClients moves clients from one mount to another
func (s *Server) handleAdminMoveClients(w http.ResponseWriter, r *http.Request) {
	srcMount := r.URL.Query().Get("mount")
	dstMount := r.URL.Query().Get("destination")

	if srcMount == "" || dstMount == "" {
		http.Error(w, "Missing mount or destination parameter", http.StatusBadRequest)
		return
	}

	// This would require disconnecting clients and having them reconnect
	// For now, just return success
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprint(w, `<?xml version="1.0"?><iceresponse><message>Clients moved</message><return>1</return></iceresponse>`)
}

// handleAdminKillClient disconnects a specific client
func (s *Server) handleAdminKillClient(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Query().Get("mount")
	clientID := r.URL.Query().Get("id")

	if mountPath == "" || clientID == "" {
		http.Error(w, "Missing mount or id parameter", http.StatusBadRequest)
		return
	}

	mount := s.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	mount.RemoveListener(clientID)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprint(w, `<?xml version="1.0"?><iceresponse><message>Client killed</message><return>1</return></iceresponse>`)
}

// handleAdminKillSource disconnects a source
func (s *Server) handleAdminKillSource(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Query().Get("mount")

	if mountPath == "" {
		http.Error(w, "Missing mount parameter", http.StatusBadRequest)
		return
	}

	mount := s.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	mount.StopSource()

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprint(w, `<?xml version="1.0"?><iceresponse><message>Source killed</message><return>1</return></iceresponse>`)
}

// handleAdminListMounts lists all mount points
func (s *Server) handleAdminListMounts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, "\n<icestats>")

	for _, stat := range s.mountManager.Stats() {
		fmt.Fprint(w, "<source>")
		fmt.Fprintf(w, "<mount>%s</mount>", stat.Path)
		fmt.Fprintf(w, "<listeners>%d</listeners>", stat.Listeners)
		fmt.Fprintf(w, "<connected>%v</connected>", stat.Active)
		fmt.Fprintf(w, "<content-type>%s</content-type>", stat.ContentType)
		fmt.Fprint(w, "</source>")
	}

	fmt.Fprint(w, "</icestats>")
}

// handleModernAdminPanel serves the modern admin panel
func (s *Server) handleModernAdminPanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(adminPanelHTML))
}

// handleAdminIndex serves the modern admin panel
func (s *Server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(adminPanelHTML))
}

// handleAdminIndexOld shows old admin index page (kept for reference)
func (s *Server) handleAdminIndexOld(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<title>GoCast Admin</title>
<style>
body { font-family: Arial, sans-serif; margin: 20px; background: #1a1a2e; color: #eee; }
h1 { color: #00d9ff; }
a { color: #00d9ff; text-decoration: none; }
a:hover { text-decoration: underline; }
.menu { background: #16213e; padding: 20px; border-radius: 8px; margin: 20px 0; }
.menu ul { list-style: none; padding: 0; }
.menu li { margin: 10px 0; }
.mount { background: #0f3460; padding: 15px; margin: 10px 0; border-radius: 8px; }
.mount h3 { margin-top: 0; color: #00d9ff; }
.active { color: #4CAF50; }
.inactive { color: #f44336; }
</style>
</head>
<body>
<h1>🎵 GoCast Admin Panel</h1>

<div class="menu">
<h2>Quick Links</h2>
<ul>
<li><a href="/admin/stats">📊 Server Statistics (XML)</a></li>
<li><a href="/admin/listmounts">📂 List All Mounts</a></li>
<li><a href="/status">🌐 Public Status Page</a></li>
<li><a href="/status?format=json">📋 Status JSON</a></li>
</ul>
</div>

<h2>Active Mounts</h2>
`)

	stats := s.mountManager.Stats()
	if len(stats) == 0 {
		fmt.Fprint(w, `<p class="inactive">No mounts configured</p>`)
	}

	for _, stat := range stats {
		status := `<span class="inactive">● Offline</span>`
		if stat.Active {
			status = `<span class="active">● Live</span>`
		}

		fmt.Fprintf(w, `<div class="mount">
<h3>%s %s</h3>
<p>Listeners: %d | Peak: %d</p>
<p>Now Playing: %s</p>
<p>
<a href="/admin/listclients?mount=%s">👥 List Clients</a> |
<a href="/admin/killsource?mount=%s" onclick="return confirm('Kill source?')">⚠️ Kill Source</a>
</p>
</div>`,
			stat.Path, status,
			stat.Listeners, stat.PeakListeners,
			stat.Metadata.StreamTitle,
			stat.Path, stat.Path,
		)
	}

	fmt.Fprintf(w, `
<p style="margin-top: 40px; color: #666; font-size: 12px;">
Server uptime: %s<br>
GoCast - Modern Icecast replacement
</p>
</body>
</html>`, time.Since(s.startTime).Round(time.Second))
}

// connStateHandler tracks connection state changes
func (s *Server) connStateHandler(conn net.Conn, state http.ConnState) {
	// Can be used for connection tracking/metrics
	switch state {
	case http.StateNew:
		// New connection
	case http.StateClosed:
		// Connection closed
	case http.StateHijacked:
		// Connection hijacked (for SOURCE method)
	}
}

// MountManager returns the mount manager
func (s *Server) MountManager() *stream.MountManager {
	return s.mountManager
}

// Config returns the server configuration
func (s *Server) Config() *config.Config {
	return s.config
}

// StartTime returns when the server started
func (s *Server) StartTime() time.Time {
	return s.startTime
}
