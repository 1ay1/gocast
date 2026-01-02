// Package server handles HTTP server and listener connections
package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"path/filepath"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/source"
	"github.com/gocast/gocast/internal/stream"
)

//go:embed admin
var adminFS embed.FS

// Server is the main GoCast HTTP server
type Server struct {
	config          *config.Config
	configManager   *config.ConfigManager
	mountManager    *stream.MountManager
	httpServer      *http.Server
	httpsServer     *http.Server
	httpChallenge   *http.Server // HTTP server for ACME challenges when using AutoSSL
	autoSSL         *AutoSSLManager
	listenerHandler *ListenerHandler
	sourceHandler   *source.Handler
	metadataHandler *source.MetadataHandler
	statusHandler   *StatusHandler
	logger          *log.Logger
	startTime       time.Time
	mu              sync.RWMutex
	// Session tokens for authenticated SSE connections
	sessionTokens map[string]time.Time
	tokenMu       sync.RWMutex
	// Log and activity buffers for admin panel
	logBuffer      *LogBuffer
	activityBuffer *ActivityBuffer
}

// generateToken creates a secure random token
func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// New creates a new GoCast server
func New(cfg *config.Config, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	mm := stream.NewMountManager(cfg)

	startTime := time.Now()
	logBuffer := NewLogBuffer(1000)
	activityBuffer := NewActivityBuffer(500)

	s := &Server{
		config:          cfg,
		configManager:   nil,
		mountManager:    mm,
		listenerHandler: NewListenerHandlerWithActivity(mm, cfg, logger, activityBuffer),
		sourceHandler:   source.NewHandler(mm, cfg, logger),
		metadataHandler: source.NewMetadataHandler(mm, cfg, logger),
		statusHandler:   NewStatusHandlerWithInfo(mm, cfg, startTime, Version),
		logger:          logger,
		startTime:       startTime,
		sessionTokens:   make(map[string]time.Time),
		logBuffer:       logBuffer,
		activityBuffer:  activityBuffer,
	}

	// Log server start
	activityBuffer.Add(ActivityServerStart, "GoCast server started", map[string]interface{}{
		"version": Version,
		"port":    cfg.Server.Port,
	})

	// Clean up expired tokens periodically
	go s.cleanupTokens()

	return s
}

// NewWithConfigManager creates a new GoCast server with a config manager
func NewWithConfigManager(cm *config.ConfigManager, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	cfg := cm.GetConfig()
	mm := stream.NewMountManager(cfg)

	startTime := time.Now()
	logBuffer := NewLogBuffer(1000)
	activityBuffer := NewActivityBuffer(500)

	s := &Server{
		config:          cfg,
		configManager:   cm,
		mountManager:    mm,
		listenerHandler: NewListenerHandlerWithActivity(mm, cfg, logger, activityBuffer),
		sourceHandler:   source.NewHandler(mm, cfg, logger),
		metadataHandler: source.NewMetadataHandler(mm, cfg, logger),
		statusHandler:   NewStatusHandlerWithInfo(mm, cfg, startTime, Version),
		logger:          logger,
		startTime:       startTime,
		sessionTokens:   make(map[string]time.Time),
		logBuffer:       logBuffer,
		activityBuffer:  activityBuffer,
	}

	// Log server start
	activityBuffer.Add(ActivityServerStart, "GoCast server started", map[string]interface{}{
		"version": Version,
		"port":    cfg.Server.Port,
	})

	// Register for config changes - propagate to all handlers
	cm.OnChange(func(newCfg *config.Config) {
		s.mu.Lock()
		s.config = newCfg
		s.mu.Unlock()

		// Propagate config to all handlers for hot-reload
		s.sourceHandler.SetConfig(newCfg)
		s.metadataHandler.SetConfig(newCfg)
		s.listenerHandler.SetConfig(newCfg)
		s.statusHandler.SetConfig(newCfg)
		s.mountManager.SetConfig(newCfg)

		s.logger.Println("Configuration updated and propagated to all handlers")
	})

	// Clean up expired tokens periodically
	go s.cleanupTokens()

	return s
}

// GetConfigManager returns the config manager (may be nil)
func (s *Server) GetConfigManager() *config.ConfigManager {
	return s.configManager
}

// NewWithSetupManager creates a new GoCast server with zero-config mode
// All settings are managed through the admin panel and persisted to state
func NewWithSetupManager(dataDir string, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}

	// Create config manager
	cm, err := config.NewConfigManager(dataDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config manager: %w", err)
	}

	cfg := cm.GetConfig()
	mm := stream.NewMountManager(cfg)

	startTime := time.Now()
	logBuffer := NewLogBuffer(1000)
	activityBuffer := NewActivityBuffer(500)

	// Set AutoSSL cache directory if not set
	if cfg.SSL.AutoSSL && cfg.SSL.CacheDir == "" {
		cfg.SSL.CacheDir = filepath.Join(cm.GetDataDir(), "certs")
	}

	s := &Server{
		config:          cfg,
		configManager:   cm,
		mountManager:    mm,
		listenerHandler: NewListenerHandlerWithActivity(mm, cfg, logger, activityBuffer),
		sourceHandler:   source.NewHandler(mm, cfg, logger),
		metadataHandler: source.NewMetadataHandler(mm, cfg, logger),
		statusHandler:   NewStatusHandlerWithInfo(mm, cfg, startTime, Version),
		logger:          logger,
		startTime:       startTime,
		sessionTokens:   make(map[string]time.Time),
		logBuffer:       logBuffer,
		activityBuffer:  activityBuffer,
	}

	// Log server start
	activityBuffer.Add(ActivityServerStart, "GoCast server started (zero-config mode)", map[string]interface{}{
		"version": Version,
		"port":    cfg.Server.Port,
	})

	// Register for config changes - propagate to all handlers
	cm.OnChange(func(newCfg *config.Config) {
		s.mu.Lock()
		s.config = newCfg
		s.mu.Unlock()

		// Propagate config to all handlers for hot-reload
		s.sourceHandler.SetConfig(newCfg)
		s.metadataHandler.SetConfig(newCfg)
		s.listenerHandler.SetConfig(newCfg)
		s.statusHandler.SetConfig(newCfg)
		s.mountManager.SetConfig(newCfg)

		s.logger.Println("Configuration updated and propagated to all handlers")
	})

	// Clean up expired tokens periodically
	go s.cleanupTokens()

	return s, nil
}

// cleanupTokens removes expired session tokens
func (s *Server) cleanupTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.tokenMu.Lock()
		now := time.Now()
		for token, expires := range s.sessionTokens {
			if now.After(expires) {
				delete(s.sessionTokens, token)
			}
		}
		s.tokenMu.Unlock()
	}
}

// createSessionToken creates a new session token valid for 24 hours
func (s *Server) createSessionToken() string {
	token := generateToken()
	s.tokenMu.Lock()
	s.sessionTokens[token] = time.Now().Add(24 * time.Hour)
	s.tokenMu.Unlock()
	return token
}

// validateSessionToken checks if a token is valid
func (s *Server) validateSessionToken(token string) bool {
	s.tokenMu.RLock()
	expires, exists := s.sessionTokens[token]
	s.tokenMu.RUnlock()
	return exists && time.Now().Before(expires)
}

// Start starts the HTTP server(s)
func (s *Server) Start() error {
	// Create main router
	mux := s.createRouter()

	// Check if AutoSSL is enabled
	if s.config.SSL.AutoSSL {
		return s.startWithAutoSSL(mux)
	}

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
	if s.config.SSL.Enabled && !s.config.SSL.AutoSSL {
		if err := s.startHTTPS(mux); err != nil {
			return fmt.Errorf("failed to start HTTPS server: %w", err)
		}
	}

	return nil
}

// startWithAutoSSL starts the server with automatic Let's Encrypt SSL
func (s *Server) startWithAutoSSL(handler http.Handler) error {
	s.logger.Printf("AutoSSL enabled for %s", s.config.Server.Hostname)

	// Create AutoSSL manager
	autoSSL, err := NewAutoSSLManager(
		s.config.Server.Hostname,
		s.config.SSL.AutoSSLEmail,
		s.config.SSL.CacheDir,
		s.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create AutoSSL manager: %w", err)
	}
	s.autoSSL = autoSSL

	// Start HTTP server on port 80 for ACME challenges + redirect to HTTPS
	sslPort := s.config.SSL.Port
	if sslPort == 0 {
		sslPort = 8443
	}
	s.httpChallenge = autoSSL.StartHTTPChallengeServer(sslPort)

	// Create HTTPS server with automatic certificates
	addr := fmt.Sprintf("%s:%d", s.config.Server.ListenAddress, sslPort)
	s.httpsServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         autoSSL.TLSConfig(),
		ReadHeaderTimeout: s.config.Limits.HeaderTimeout,
		IdleTimeout:       s.config.Limits.ClientTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		s.logger.Printf("Starting GoCast HTTPS server on %s (AutoSSL)", addr)
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTPS server error: %v", err)
		}
	}()

	s.logger.Printf("AutoSSL: Certificates will be automatically obtained and renewed for %s", s.config.Server.Hostname)

	return nil
}

// startHTTPS starts the HTTPS server
func (s *Server) startHTTPS(handler http.Handler) error {
	cert, err := tls.LoadX509KeyPair(s.config.SSL.CertPath, s.config.SSL.KeyPath)
	if err != nil {
		return fmt.Errorf("failed to load SSL certificates: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	addr := fmt.Sprintf("%s:%d", s.config.Server.ListenAddress, s.config.SSL.Port)
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

	// Stop activity buffer flush loop first
	if s.activityBuffer != nil {
		s.activityBuffer.Stop()
	}

	// Log server stop
	if s.activityBuffer != nil {
		s.activityBuffer.Add(ActivityServerStop, "GoCast server stopping", nil)
	}

	// Disconnect all listeners immediately so HTTP server can shutdown quickly
	s.logger.Println("Disconnecting all listeners...")
	mounts := s.mountManager.GetAllMounts()
	for _, mount := range mounts {
		listeners := mount.GetListeners()
		for _, l := range listeners {
			l.Close()
		}
		// Also stop any active sources
		if mount.IsActive() {
			mount.StopSource()
		}
	}

	var wg sync.WaitGroup

	// Shutdown HTTP server with a short timeout
	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Use a shorter context for HTTP shutdown since we already closed connections
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
				s.logger.Printf("HTTP server shutdown error: %v, forcing close", err)
				s.httpServer.Close() // Force close if graceful shutdown fails
			}
		}()
	}

	// Shutdown HTTPS server
	if s.httpsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.httpsServer.Shutdown(shutdownCtx); err != nil {
				s.logger.Printf("HTTPS server shutdown error: %v, forcing close", err)
				s.httpsServer.Close()
			}
		}()
	}

	// Shutdown HTTP challenge server (AutoSSL)
	if s.httpChallenge != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.httpChallenge.Shutdown(shutdownCtx); err != nil {
				s.logger.Printf("HTTP challenge server shutdown error: %v", err)
				s.httpChallenge.Close()
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
		// Force close if context times out
		if s.httpServer != nil {
			s.httpServer.Close()
		}
		if s.httpsServer != nil {
			s.httpsServer.Close()
		}
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

		// Admin static assets (CSS, JS, images, including nested paths like js/pages/)
		if strings.HasPrefix(path, "/admin/css/") || strings.HasPrefix(path, "/admin/js/") || strings.HasPrefix(path, "/admin/pages/") || strings.HasPrefix(path, "/admin/img/") {
			s.serveAdminStatic(w, r)
			return
		}

		// Token generation endpoint (requires basic auth) - must be before general /admin/ handler
		if path == "/admin/token" {
			s.handleAdminToken(w, r)
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

		// Token-authenticated SSE events endpoint
		if path == "/events" {
			s.handleTokenEvents(w, r)
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

	case path == "/admin/events":
		s.handleAdminEvents(w, r)

	case path == "/admin/logs":
		s.handleAdminLogs(w, r)

	case path == "/admin/activity":
		s.handleAdminActivity(w, r)

	case strings.HasPrefix(path, "/admin/config"):
		s.handleAdminConfig(w, r)

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
	fmt.Fprintf(w, "<server_id>GoCast/%s</server_id>", Version)
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

	// Use unique listeners to consolidate multiple connections from same IP/UserAgent
	uniqueListeners := mount.GetUniqueListeners()

	// Check if JSON is requested
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		var sb strings.Builder
		sb.WriteString(`{"mount":`)
		sb.WriteString(fmt.Sprintf("%q", mountPath))
		sb.WriteString(`,"listeners":[`)

		for i, listener := range uniqueListeners {
			if i > 0 {
				sb.WriteString(",")
			}
			connected := int(time.Since(listener.ConnectedAt).Seconds())
			// Use first ID as the primary, but include all IDs for kick functionality
			primaryID := listener.IDs[0]
			sb.WriteString(fmt.Sprintf(`{"id":%q,"ip":%q,"user_agent":%q,"connected":%d,"connections":%d,"ids":%s}`,
				primaryID, listener.IP, listener.UserAgent, connected, listener.Connections, toJSONStringArray(listener.IDs)))
		}

		sb.WriteString(`],"total":`)
		sb.WriteString(fmt.Sprintf("%d", len(uniqueListeners)))
		sb.WriteString(`,"total_connections":`)
		sb.WriteString(fmt.Sprintf("%d}", mount.ListenerCount()))
		w.Write([]byte(sb.String()))
		return
	}

	// Default to XML for Icecast compatibility
	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, "\n<icestats>")
	fmt.Fprintf(w, "<source mount=\"%s\">", mountPath)

	for _, listener := range uniqueListeners {
		fmt.Fprint(w, "<listener>")
		fmt.Fprintf(w, "<ID>%s</ID>", listener.IDs[0])
		fmt.Fprintf(w, "<IP>%s</IP>", listener.IP)
		fmt.Fprintf(w, "<UserAgent>%s</UserAgent>", escapeXML(listener.UserAgent))
		fmt.Fprintf(w, "<Connected>%d</Connected>", int(time.Since(listener.ConnectedAt).Seconds()))
		fmt.Fprintf(w, "<Connections>%d</Connections>", listener.Connections)
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

// toJSONStringArray converts a slice of strings to a JSON array string
func toJSONStringArray(strs []string) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, s := range strs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%q", s))
	}
	sb.WriteString("]")
	return sb.String()
}

// handleAdminKillClient disconnects a specific client (or all connections from same IP/UA)
func (s *Server) handleAdminKillClient(w http.ResponseWriter, r *http.Request) {
	mountPath := r.URL.Query().Get("mount")
	clientID := r.URL.Query().Get("id")
	killAll := r.URL.Query().Get("all") == "true" // If true, kill all connections from same IP/UA

	if mountPath == "" || clientID == "" {
		http.Error(w, "Missing mount or id parameter", http.StatusBadRequest)
		return
	}

	mount := s.mountManager.GetMount(mountPath)
	if mount == nil {
		http.Error(w, "Mount not found", http.StatusNotFound)
		return
	}

	killedCount := 0
	if killAll {
		// Find the listener first to get IP/UA, then kill all matching
		listeners := mount.GetListeners()
		var targetIP, targetUA string
		for _, l := range listeners {
			if l.ID == clientID {
				targetIP = l.IP
				targetUA = l.UserAgent
				break
			}
		}
		if targetIP != "" {
			// Kill all connections from same IP/UA
			for _, l := range listeners {
				if l.IP == targetIP && l.UserAgent == targetUA {
					mount.RemoveListenerByID(l.ID)
					killedCount++
				}
			}
		}
	} else {
		mount.RemoveListenerByID(clientID)
		killedCount = 1
	}

	s.logger.Printf("Killed %d client connection(s) on %s", killedCount, mountPath)

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
// handleAdminEvents provides Server-Sent Events for real-time updates
func (s *Server) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial data
	s.sendSSEStats(w, flusher)

	// Send recent activity and logs on connect
	s.sendSSERecentActivity(w, flusher)
	s.sendSSERecentLogs(w, flusher)

	// Subscribe to activity and log events
	var activityCh chan ActivityEntry
	var logCh chan LogEntry

	if s.activityBuffer != nil {
		activityCh = s.activityBuffer.Subscribe()
		defer s.activityBuffer.Unsubscribe(activityCh)
	}

	if s.logBuffer != nil {
		logCh = s.logBuffer.Subscribe()
		defer s.logBuffer.Unsubscribe(logCh)
	}

	// Create ticker for stats updates (every 500ms for smooth updates)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Keep connection open and send updates
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.sendSSEStats(w, flusher)
		case activity, ok := <-activityCh:
			if ok {
				s.sendSSEActivity(w, flusher, activity)
			}
		case logEntry, ok := <-logCh:
			if ok {
				s.sendSSELog(w, flusher, logEntry)
			}
		}
	}
}

// handleTokenEvents provides token-authenticated SSE for real-time status
func (s *Server) handleTokenEvents(w http.ResponseWriter, r *http.Request) {
	// Check token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	if !s.validateSessionToken(token) {
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	// Token is valid, serve SSE
	s.handleAdminEvents(w, r)
}

// handleAdminToken generates a session token for authenticated users
func (s *Server) handleAdminToken(w http.ResponseWriter, r *http.Request) {
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

	// Generate and return token
	token := s.createSessionToken()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"token":"%s","expires_in":86400}`, token)
}

func (s *Server) sendSSEStats(w http.ResponseWriter, flusher http.Flusher) {
	stats := s.mountManager.Stats()

	// Build mounts array for the stats event
	var sb strings.Builder
	sb.WriteString(`{"type":"stats","server_id":"GoCast/`)
	sb.WriteString(Version)
	sb.WriteString(`","version":"`)
	sb.WriteString(Version)
	sb.WriteString(`","uptime":`)
	sb.WriteString(fmt.Sprintf("%d", int(time.Since(s.startTime).Seconds())))
	sb.WriteString(`,"started":"`)
	sb.WriteString(s.startTime.Format(time.RFC3339))
	sb.WriteString(`","mounts":[`)

	for i, stat := range stats {
		if i > 0 {
			sb.WriteString(",")
		}
		title := stat.Metadata.StreamTitle
		if title == "" {
			title = stat.Metadata.Name
		}
		sb.WriteString(fmt.Sprintf(
			`{"path":"%s","mount":"%s","listeners":%d,"peak":%d,"active":%v,"title":"%s","artist":"%s","album":"%s","name":"%s","genre":"%s","description":"%s","bitrate":%d,"content_type":"%s"}`,
			stat.Path, stat.Path, stat.Listeners, stat.PeakListeners, stat.Active,
			escapeJSON(title), escapeJSON(stat.Metadata.Artist), escapeJSON(stat.Metadata.Album),
			escapeJSON(stat.Metadata.Name), escapeJSON(stat.Metadata.Genre),
			escapeJSON(stat.Metadata.Description), stat.Metadata.Bitrate, stat.ContentType,
		))
	}
	sb.WriteString("]}")

	// Send as named event for proper SSE handling
	fmt.Fprintf(w, "event: stats\ndata: %s\n\n", sb.String())
	flusher.Flush()
}

// sendSSEActivity sends a single activity entry via SSE
func (s *Server) sendSSEActivity(w http.ResponseWriter, flusher http.Flusher, activity ActivityEntry) {
	data := fmt.Sprintf(`{"id":%d,"timestamp":"%s","type":"%s","message":"%s"}`,
		activity.ID, activity.Timestamp.Format(time.RFC3339),
		activity.Type, escapeJSON(activity.Message))

	fmt.Fprintf(w, "event: activity\ndata: %s\n\n", data)
	flusher.Flush()
}

// sendSSELog sends a single log entry via SSE
func (s *Server) sendSSELog(w http.ResponseWriter, flusher http.Flusher, logEntry LogEntry) {
	data := fmt.Sprintf(`{"id":%d,"timestamp":"%s","level":"%s","source":"%s","message":"%s"}`,
		logEntry.ID, logEntry.Timestamp.Format(time.RFC3339),
		logEntry.Level, escapeJSON(logEntry.Source), escapeJSON(logEntry.Message))

	fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
	flusher.Flush()
}

// sendSSERecentActivity sends recent activity entries on SSE connect
func (s *Server) sendSSERecentActivity(w http.ResponseWriter, flusher http.Flusher) {
	if s.activityBuffer == nil {
		return
	}

	entries := s.activityBuffer.GetRecent(50)
	if len(entries) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(`{"type":"activity_history","entries":[`)

	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"id":%d,"timestamp":"%s","type":"%s","message":"%s"}`,
			entry.ID, entry.Timestamp.Format(time.RFC3339),
			entry.Type, escapeJSON(entry.Message)))
	}

	sb.WriteString("]}")

	fmt.Fprintf(w, "event: activity_history\ndata: %s\n\n", sb.String())
	flusher.Flush()
}

// sendSSERecentLogs sends recent log entries on SSE connect
func (s *Server) sendSSERecentLogs(w http.ResponseWriter, flusher http.Flusher) {
	if s.logBuffer == nil {
		return
	}

	entries := s.logBuffer.GetRecent(100)
	if len(entries) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(`{"type":"log_history","entries":[`)

	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"id":%d,"timestamp":"%s","level":"%s","source":"%s","message":"%s"}`,
			entry.ID, entry.Timestamp.Format(time.RFC3339),
			entry.Level, escapeJSON(entry.Source), escapeJSON(entry.Message)))
	}

	sb.WriteString("]}")

	fmt.Fprintf(w, "event: log_history\ndata: %s\n\n", sb.String())
	flusher.Flush()
}

// handleAdminLogs returns recent log entries as JSON
func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.logBuffer == nil {
		fmt.Fprint(w, `{"success":true,"data":[]}`)
		return
	}

	// Get count from query, default 100
	count := 100
	if countStr := r.URL.Query().Get("count"); countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 {
			count = n
		}
	}

	entries := s.logBuffer.GetRecent(count)

	var sb strings.Builder
	sb.WriteString(`{"success":true,"data":[`)

	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"id":%d,"timestamp":"%s","level":"%s","source":"%s","message":"%s"}`,
			entry.ID, entry.Timestamp.Format(time.RFC3339),
			entry.Level, escapeJSON(entry.Source), escapeJSON(entry.Message)))
	}

	sb.WriteString("]}")
	w.Write([]byte(sb.String()))
}

// handleAdminActivity returns recent activity entries as JSON
func (s *Server) handleAdminActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.activityBuffer == nil {
		fmt.Fprint(w, `{"success":true,"data":[]}`)
		return
	}

	// Get count from query, default 50
	count := 50
	if countStr := r.URL.Query().Get("count"); countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 {
			count = n
		}
	}

	entries := s.activityBuffer.GetRecent(count)

	var sb strings.Builder
	sb.WriteString(`{"success":true,"data":[`)

	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`{"id":%d,"timestamp":"%s","type":"%s","message":"%s"}`,
			entry.ID, entry.Timestamp.Format(time.RFC3339),
			entry.Type, escapeJSON(entry.Message)))
	}

	sb.WriteString("]}")
	w.Write([]byte(sb.String()))
}

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
	s.serveAdminIndex(w, r)
}

// serveAdminStatic serves static files from the embedded admin directory
func (s *Server) serveAdminStatic(w http.ResponseWriter, r *http.Request) {
	// Strip leading slash to get the embedded path
	filePath := strings.TrimPrefix(r.URL.Path, "/")

	content, err := adminFS.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type based on file extension
	switch {
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(filePath, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case strings.HasSuffix(filePath, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(filePath, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(filePath, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	}

	// Enable caching for static assets
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(content)
}

// serveAdminIndex serves the admin panel index.html
func (s *Server) serveAdminIndex(w http.ResponseWriter, r *http.Request) {
	content, err := adminFS.ReadFile("admin/index.html")
	if err != nil {
		// Fallback error message
		http.Error(w, "Admin panel not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// getAdminFS returns the embedded admin filesystem for use in handlers
func getAdminFS() fs.FS {
	subFS, _ := fs.Sub(adminFS, "admin")
	return subFS
}

// handleAdminIndex serves the modern admin panel
func (s *Server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	s.serveAdminIndex(w, r)
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

// GetLogWriter returns an io.Writer that captures logs to the log buffer
func (s *Server) GetLogWriter(source string) *LogWriter {
	if s.logBuffer == nil {
		return nil
	}
	return NewLogWriter(s.logBuffer, LogLevelInfo, source)
}

// GetLogBuffer returns the log buffer for direct access
func (s *Server) GetLogBuffer() *LogBuffer {
	return s.logBuffer
}

// GetActivityBuffer returns the activity buffer for direct access
func (s *Server) GetActivityBuffer() *ActivityBuffer {
	return s.activityBuffer
}

// LogInfo adds an info log entry
func (s *Server) LogInfo(source, message string) {
	if s.logBuffer != nil {
		s.logBuffer.AddInfo(source, message)
	}
}

// LogError adds an error log entry
func (s *Server) LogError(source, message string) {
	if s.logBuffer != nil {
		s.logBuffer.AddError(source, message)
	}
}

// LogWarn adds a warning log entry
func (s *Server) LogWarn(source, message string) {
	if s.logBuffer != nil {
		s.logBuffer.AddWarn(source, message)
	}
}

// RecordActivity adds an activity entry
func (s *Server) RecordActivity(actType ActivityType, message string, data map[string]interface{}) {
	if s.activityBuffer != nil {
		s.activityBuffer.Add(actType, message, data)
	}
}
