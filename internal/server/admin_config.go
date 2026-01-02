// Package server handles HTTP server and listener connections
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gocast/gocast/internal/config"
)

// ConfigAPIResponse represents a standard API response
type ConfigAPIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ServerConfigDTO represents server configuration for API
type ServerConfigDTO struct {
	Hostname      string `json:"hostname"`
	ListenAddress string `json:"listen_address,omitempty"`
	Location      string `json:"location"`
	ServerID      string `json:"server_id"`
	Port          int    `json:"port"`
	AdminRoot     string `json:"admin_root,omitempty"`
}

// SSLConfigDTO represents SSL configuration for API
type SSLConfigDTO struct {
	Enabled      bool   `json:"enabled"`
	AutoSSL      bool   `json:"auto_ssl"`
	AutoSSLEmail string `json:"auto_ssl_email,omitempty"`
	Port         int    `json:"port"`
	CertPath     string `json:"cert_path,omitempty"`
	KeyPath      string `json:"key_path,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
}

// LimitsConfigDTO represents limits configuration for API
type LimitsConfigDTO struct {
	MaxClients           int `json:"max_clients"`
	MaxSources           int `json:"max_sources"`
	MaxListenersPerMount int `json:"max_listeners_per_mount"`
	QueueSize            int `json:"queue_size"`
	BurstSize            int `json:"burst_size"`
	ClientTimeout        int `json:"client_timeout,omitempty"`
	HeaderTimeout        int `json:"header_timeout,omitempty"`
	SourceTimeout        int `json:"source_timeout,omitempty"`
}

// AuthConfigDTO represents auth configuration for API
type AuthConfigDTO struct {
	SourcePassword string `json:"source_password"`
	AdminUser      string `json:"admin_user"`
	AdminPassword  string `json:"admin_password,omitempty"`
}

// MountConfigDTO represents mount configuration for API
type MountConfigDTO struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Password     string `json:"password,omitempty"`
	MaxListeners int    `json:"max_listeners"`
	Genre        string `json:"genre"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	Bitrate      int    `json:"bitrate"`
	Type         string `json:"type"`
	Public       bool   `json:"public"`
	StreamName   string `json:"stream_name"`
	Hidden       bool   `json:"hidden"`
	BurstSize    int    `json:"burst_size"`
}

// LoggingConfigDTO represents logging configuration for API
type LoggingConfigDTO struct {
	LogLevel  string `json:"log_level"`
	AccessLog string `json:"access_log,omitempty"`
	ErrorLog  string `json:"error_log,omitempty"`
	LogSize   int    `json:"log_size,omitempty"`
}

// DirectoryConfigDTO represents directory/YP configuration for API
type DirectoryConfigDTO struct {
	Enabled  bool     `json:"enabled"`
	YPURLs   []string `json:"yp_urls,omitempty"`
	Interval int      `json:"interval,omitempty"`
}

// FullConfigDTO represents the complete configuration for API
type FullConfigDTO struct {
	Server        ServerConfigDTO           `json:"server"`
	SSL           SSLConfigDTO              `json:"ssl"`
	Limits        LimitsConfigDTO           `json:"limits"`
	Auth          AuthConfigDTO             `json:"auth"`
	Logging       LoggingConfigDTO          `json:"logging"`
	Directory     DirectoryConfigDTO        `json:"directory"`
	Mounts        map[string]MountConfigDTO `json:"mounts"`
	LastModified  string                    `json:"last_modified,omitempty"`
	SetupComplete bool                      `json:"setup_complete"`
	ConfigPath    string                    `json:"config_path,omitempty"`
}

// handleAdminConfig routes config API requests
func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check if config manager is available
	if s.configManager == nil {
		s.jsonError(w, "Configuration management not available", http.StatusServiceUnavailable)
		return
	}

	switch {
	case path == "/admin/config" && r.Method == http.MethodGet:
		s.handleGetConfig(w, r)
	case path == "/admin/config" && r.Method == http.MethodPost:
		s.handleUpdateConfig(w, r)
	case path == "/admin/config/reset" && r.Method == http.MethodPost:
		s.handleResetConfig(w, r)
	case path == "/admin/config/export" && r.Method == http.MethodGet:
		s.handleExportConfig(w, r)
	case path == "/admin/config/reload" && r.Method == http.MethodPost:
		s.handleReloadConfig(w, r)
	case path == "/admin/config/server" && r.Method == http.MethodPost:
		s.handleUpdateServerConfig(w, r)
	case path == "/admin/config/ssl" && r.Method == http.MethodGet:
		s.handleGetSSLConfig(w, r)
	case path == "/admin/config/ssl" && r.Method == http.MethodPost:
		s.handleUpdateSSLConfig(w, r)
	case path == "/admin/config/ssl/enable" && r.Method == http.MethodPost:
		s.handleEnableAutoSSL(w, r)
	case path == "/admin/config/ssl/disable" && r.Method == http.MethodPost:
		s.handleDisableSSL(w, r)
	case path == "/admin/config/limits" && r.Method == http.MethodPost:
		s.handleUpdateLimitsConfig(w, r)
	case path == "/admin/config/limits" && r.Method == http.MethodGet:
		s.handleGetLimitsConfig(w, r)
	case path == "/admin/config/auth" && r.Method == http.MethodPost:
		s.handleUpdateAuthConfig(w, r)
	case path == "/admin/config/logging" && r.Method == http.MethodPost:
		s.handleUpdateLoggingConfig(w, r)
	case path == "/admin/config/directory" && r.Method == http.MethodPost:
		s.handleUpdateDirectoryConfig(w, r)
	case strings.HasPrefix(path, "/admin/config/mounts"):
		s.handleMountsConfig(w, r)
	default:
		s.jsonError(w, "Not found", http.StatusNotFound)
	}
}

// handleGetConfig returns the current configuration
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()

	dto := FullConfigDTO{
		Server: ServerConfigDTO{
			Hostname:      cfg.Server.Hostname,
			ListenAddress: cfg.Server.ListenAddress,
			Location:      cfg.Server.Location,
			ServerID:      cfg.Server.ServerID,
			Port:          cfg.Server.Port,
			AdminRoot:     cfg.Server.AdminRoot,
		},
		SSL: SSLConfigDTO{
			Enabled:      cfg.SSL.Enabled,
			AutoSSL:      cfg.SSL.AutoSSL,
			AutoSSLEmail: cfg.SSL.AutoSSLEmail,
			Port:         cfg.SSL.Port,
			CertPath:     cfg.SSL.CertPath,
			KeyPath:      cfg.SSL.KeyPath,
			Hostname:     cfg.Server.Hostname,
		},
		Limits: LimitsConfigDTO{
			MaxClients:           cfg.Limits.MaxClients,
			MaxSources:           cfg.Limits.MaxSources,
			MaxListenersPerMount: cfg.Limits.MaxListenersPerMount,
			QueueSize:            cfg.Limits.QueueSize,
			BurstSize:            cfg.Limits.BurstSize,
			ClientTimeout:        int(cfg.Limits.ClientTimeout.Seconds()),
			HeaderTimeout:        int(cfg.Limits.HeaderTimeout.Seconds()),
			SourceTimeout:        int(cfg.Limits.SourceTimeout.Seconds()),
		},
		Auth: AuthConfigDTO{
			SourcePassword: cfg.Auth.SourcePassword,
			AdminUser:      cfg.Auth.AdminUser,
			// Don't expose admin password
		},
		Logging: LoggingConfigDTO{
			LogLevel:  cfg.Logging.LogLevel,
			AccessLog: cfg.Logging.AccessLog,
			ErrorLog:  cfg.Logging.ErrorLog,
			LogSize:   cfg.Logging.LogSize,
		},
		Directory: DirectoryConfigDTO{
			Enabled:  cfg.Directory.Enabled,
			YPURLs:   cfg.Directory.YPURLs,
			Interval: int(cfg.Directory.Interval.Seconds()),
		},
		Mounts:        make(map[string]MountConfigDTO),
		LastModified:  cfg.LastModified.Format(time.RFC3339),
		SetupComplete: s.configManager.IsSetupComplete(),
		ConfigPath:    s.configManager.GetConfigPath(),
	}

	for path, mount := range cfg.Mounts {
		dto.Mounts[path] = MountConfigDTO{
			Path:         path,
			Name:         mount.Name,
			MaxListeners: mount.MaxListeners,
			Genre:        mount.Genre,
			Description:  mount.Description,
			URL:          mount.URL,
			Bitrate:      mount.Bitrate,
			Type:         mount.Type,
			Public:       mount.Public,
			StreamName:   mount.StreamName,
			Hidden:       mount.Hidden,
			BurstSize:    mount.BurstSize,
		}
	}

	s.jsonSuccess(w, dto)
}

// handleReloadConfig reloads configuration from disk
func (s *Server) handleReloadConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.configManager.Reload(); err != nil {
		s.jsonError(w, "Failed to reload configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Configuration reloaded from disk. Changes applied immediately.",
	})
}

// handleUpdateConfig handles full configuration update
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var dto FullConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update server config
	var port *int
	if dto.Server.Port > 0 {
		port = &dto.Server.Port
	}
	if err := s.configManager.UpdateServer(&dto.Server.Hostname, &dto.Server.Location, &dto.Server.ServerID, nil, nil, port); err != nil {
		s.jsonError(w, "Failed to update server config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update limits config
	if err := s.configManager.UpdateLimits(
		&dto.Limits.MaxClients,
		&dto.Limits.MaxSources,
		&dto.Limits.MaxListenersPerMount,
		&dto.Limits.QueueSize,
		&dto.Limits.BurstSize,
		nil, nil, nil,
	); err != nil {
		s.jsonError(w, "Failed to update limits config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update auth config (only if password provided)
	var adminPass *string
	if dto.Auth.AdminPassword != "" {
		adminPass = &dto.Auth.AdminPassword
	}
	if err := s.configManager.UpdateAuth(&dto.Auth.SourcePassword, &dto.Auth.AdminUser, adminPass); err != nil {
		s.jsonError(w, "Failed to update auth config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Configuration updated. Changes applied immediately.",
	})
}

// handleResetConfig resets configuration to defaults
func (s *Server) handleResetConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.configManager.ResetToDefaults(); err != nil {
		s.jsonError(w, "Failed to reset configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Configuration reset to defaults. Changes applied immediately.",
	})
}

// handleExportConfig exports configuration as JSON
func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	data, err := s.configManager.ExportConfig()
	if err != nil {
		s.jsonError(w, "Failed to export configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=gocast-config.json")
	w.Write(data)
}

// handleUpdateServerConfig updates server configuration
func (s *Server) handleUpdateServerConfig(w http.ResponseWriter, r *http.Request) {
	var dto ServerConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Handle optional fields
	var listenAddr, adminRoot *string
	if dto.ListenAddress != "" {
		listenAddr = &dto.ListenAddress
	}
	if dto.AdminRoot != "" {
		adminRoot = &dto.AdminRoot
	}

	var port *int
	if dto.Port > 0 {
		port = &dto.Port
	}

	if err := s.configManager.UpdateServer(&dto.Hostname, &dto.Location, &dto.ServerID, listenAddr, adminRoot, port); err != nil {
		s.jsonError(w, "Failed to update server config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Server configuration updated. Changes applied immediately.",
	})
}

// handleGetSSLConfig returns the current SSL configuration
func (s *Server) handleGetSSLConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()

	dto := SSLConfigDTO{
		Enabled:      cfg.SSL.Enabled,
		AutoSSL:      cfg.SSL.AutoSSL,
		AutoSSLEmail: cfg.SSL.AutoSSLEmail,
		Port:         cfg.SSL.Port,
		CertPath:     cfg.SSL.CertPath,
		KeyPath:      cfg.SSL.KeyPath,
		Hostname:     cfg.Server.Hostname,
	}

	s.jsonSuccess(w, dto)
}

// handleUpdateSSLConfig updates SSL configuration
func (s *Server) handleUpdateSSLConfig(w http.ResponseWriter, r *http.Request) {
	var dto SSLConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.configManager.UpdateSSL(
		&dto.Enabled,
		&dto.AutoSSL,
		&dto.Port,
		&dto.AutoSSLEmail,
		&dto.CertPath,
		&dto.KeyPath,
	); err != nil {
		s.jsonError(w, "Failed to update SSL config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "SSL configuration updated. Changes applied immediately.",
	})
}

// handleEnableAutoSSL enables automatic SSL with Let's Encrypt
func (s *Server) handleEnableAutoSSL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hostname string `json:"hostname"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Hostname == "" || req.Hostname == "localhost" {
		s.jsonError(w, "A valid public hostname is required for AutoSSL", http.StatusBadRequest)
		return
	}

	if err := s.configManager.EnableAutoSSL(req.Hostname, req.Email); err != nil {
		s.jsonError(w, "Failed to enable AutoSSL: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "AutoSSL enabled. Changes applied immediately.",
	})
}

// handleDisableSSL disables SSL
func (s *Server) handleDisableSSL(w http.ResponseWriter, r *http.Request) {
	if err := s.configManager.DisableSSL(); err != nil {
		s.jsonError(w, "Failed to disable SSL: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "SSL disabled. Changes applied immediately.",
	})
}

// handleUpdateLimitsConfig updates limits configuration
func (s *Server) handleUpdateLimitsConfig(w http.ResponseWriter, r *http.Request) {
	var dto LimitsConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Only update fields that have valid non-zero values
	var maxClients, maxSources, maxListenersPerMount, queueSize, burstSize *int
	var clientTimeout, headerTimeout, sourceTimeout *int

	if dto.MaxClients > 0 {
		maxClients = &dto.MaxClients
	}
	if dto.MaxSources > 0 {
		maxSources = &dto.MaxSources
	}
	if dto.MaxListenersPerMount > 0 {
		maxListenersPerMount = &dto.MaxListenersPerMount
	}
	if dto.QueueSize > 0 {
		queueSize = &dto.QueueSize
	}
	if dto.BurstSize > 0 {
		burstSize = &dto.BurstSize
	}
	if dto.ClientTimeout > 0 {
		clientTimeout = &dto.ClientTimeout
	}
	if dto.HeaderTimeout > 0 {
		headerTimeout = &dto.HeaderTimeout
	}
	if dto.SourceTimeout > 0 {
		sourceTimeout = &dto.SourceTimeout
	}

	if err := s.configManager.UpdateLimits(
		maxClients,
		maxSources,
		maxListenersPerMount,
		queueSize,
		burstSize,
		clientTimeout,
		headerTimeout,
		sourceTimeout,
	); err != nil {
		s.jsonError(w, "Failed to update limits config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Limits configuration updated. Changes applied immediately.",
	})
}

// handleGetLimitsConfig returns the current limits configuration
func (s *Server) handleGetLimitsConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()

	dto := LimitsConfigDTO{
		MaxClients:           cfg.Limits.MaxClients,
		MaxSources:           cfg.Limits.MaxSources,
		MaxListenersPerMount: cfg.Limits.MaxListenersPerMount,
		QueueSize:            cfg.Limits.QueueSize,
		BurstSize:            cfg.Limits.BurstSize,
		ClientTimeout:        int(cfg.Limits.ClientTimeout.Seconds()),
		HeaderTimeout:        int(cfg.Limits.HeaderTimeout.Seconds()),
		SourceTimeout:        int(cfg.Limits.SourceTimeout.Seconds()),
	}

	s.jsonSuccess(w, dto)
}

// handleUpdateLoggingConfig updates logging configuration
func (s *Server) handleUpdateLoggingConfig(w http.ResponseWriter, r *http.Request) {
	var dto LoggingConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Handle optional fields
	var accessLog, errorLog *string
	var logSize *int
	if dto.AccessLog != "" {
		accessLog = &dto.AccessLog
	}
	if dto.ErrorLog != "" {
		errorLog = &dto.ErrorLog
	}
	if dto.LogSize > 0 {
		logSize = &dto.LogSize
	}

	if err := s.configManager.UpdateLogging(&dto.LogLevel, accessLog, errorLog, logSize); err != nil {
		s.jsonError(w, "Failed to update logging config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Logging configuration updated. Changes applied immediately.",
	})
}

// handleUpdateDirectoryConfig updates directory/YP configuration
func (s *Server) handleUpdateDirectoryConfig(w http.ResponseWriter, r *http.Request) {
	var dto DirectoryConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Handle optional interval
	var interval *int
	if dto.Interval > 0 {
		interval = &dto.Interval
	}

	if err := s.configManager.UpdateDirectory(&dto.Enabled, dto.YPURLs, interval); err != nil {
		s.jsonError(w, "Failed to update directory config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Directory configuration updated. Changes applied immediately.",
	})
}

// handleUpdateAuthConfig updates auth configuration
func (s *Server) handleUpdateAuthConfig(w http.ResponseWriter, r *http.Request) {
	var dto AuthConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Only update fields that are non-empty (leave unchanged if empty)
	var sourcePass *string
	if dto.SourcePassword != "" {
		sourcePass = &dto.SourcePassword
	}

	var adminUser *string
	if dto.AdminUser != "" {
		adminUser = &dto.AdminUser
	}

	var adminPass *string
	if dto.AdminPassword != "" {
		adminPass = &dto.AdminPassword
	}

	if err := s.configManager.UpdateAuth(sourcePass, adminUser, adminPass); err != nil {
		s.jsonError(w, "Failed to update auth config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Auth configuration updated. Changes applied immediately.",
	})
}

// handleMountsConfig handles mount CRUD operations
func (s *Server) handleMountsConfig(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// List all mounts
	if path == "/admin/config/mounts" && r.Method == http.MethodGet {
		s.handleListMountsConfig(w, r)
		return
	}

	// Create new mount
	if path == "/admin/config/mounts" && r.Method == http.MethodPost {
		s.handleCreateMountConfig(w, r)
		return
	}

	// Extract mount path from URL
	mountPath := strings.TrimPrefix(path, "/admin/config/mounts")
	if mountPath == "" {
		s.jsonError(w, "Mount path required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetMountConfig(w, r, mountPath)
	case http.MethodPut:
		s.handleUpdateMountConfig(w, r, mountPath)
	case http.MethodDelete:
		s.handleDeleteMountConfig(w, r, mountPath)
	default:
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListMountsConfig lists all mount configurations
func (s *Server) handleListMountsConfig(w http.ResponseWriter, r *http.Request) {
	mounts := s.configManager.GetAllMounts()

	result := make(map[string]MountConfigDTO)
	for path, mount := range mounts {
		result[path] = MountConfigDTO{
			Path:         path,
			Name:         mount.Name,
			MaxListeners: mount.MaxListeners,
			Genre:        mount.Genre,
			Description:  mount.Description,
			URL:          mount.URL,
			Bitrate:      mount.Bitrate,
			Type:         mount.Type,
			Public:       mount.Public,
			StreamName:   mount.StreamName,
			Hidden:       mount.Hidden,
			BurstSize:    mount.BurstSize,
		}
	}

	s.jsonSuccess(w, result)
}

// handleCreateMountConfig creates a new mount
func (s *Server) handleCreateMountConfig(w http.ResponseWriter, r *http.Request) {
	var dto MountConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if dto.Path == "" {
		s.jsonError(w, "Mount path is required", http.StatusBadRequest)
		return
	}

	// Ensure path starts with /
	if !strings.HasPrefix(dto.Path, "/") {
		dto.Path = "/" + dto.Path
	}

	cfg := s.configManager.GetConfig()

	mount := &config.MountConfig{
		Name:         dto.Path,
		Password:     dto.Password,
		MaxListeners: dto.MaxListeners,
		Genre:        dto.Genre,
		Description:  dto.Description,
		URL:          dto.URL,
		Bitrate:      dto.Bitrate,
		Type:         dto.Type,
		Public:       dto.Public,
		StreamName:   dto.StreamName,
		Hidden:       dto.Hidden,
		BurstSize:    dto.BurstSize,
	}

	// Apply defaults
	if mount.MaxListeners == 0 {
		mount.MaxListeners = cfg.Limits.MaxListenersPerMount
	}
	if mount.Type == "" {
		mount.Type = "audio/mpeg"
	}
	if mount.BurstSize == 0 {
		mount.BurstSize = cfg.Limits.BurstSize
	}
	if mount.Bitrate == 0 {
		mount.Bitrate = 128
	}

	if err := s.configManager.CreateMount(dto.Path, mount); err != nil {
		s.jsonError(w, "Failed to create mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s created. Changes applied immediately.", dto.Path),
	})
}

// handleGetMountConfig gets a specific mount configuration
func (s *Server) handleGetMountConfig(w http.ResponseWriter, r *http.Request, mountPath string) {
	mount := s.configManager.GetMount(mountPath)
	if mount == nil {
		s.jsonError(w, "Mount not found", http.StatusNotFound)
		return
	}

	dto := MountConfigDTO{
		Path:         mountPath,
		Name:         mount.Name,
		MaxListeners: mount.MaxListeners,
		Genre:        mount.Genre,
		Description:  mount.Description,
		URL:          mount.URL,
		Bitrate:      mount.Bitrate,
		Type:         mount.Type,
		Public:       mount.Public,
		StreamName:   mount.StreamName,
		Hidden:       mount.Hidden,
		BurstSize:    mount.BurstSize,
	}

	s.jsonSuccess(w, dto)
}

// handleUpdateMountConfig updates an existing mount
func (s *Server) handleUpdateMountConfig(w http.ResponseWriter, r *http.Request, mountPath string) {
	var dto MountConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get existing mount to preserve password if not provided
	existingMount := s.configManager.GetMount(mountPath)
	password := dto.Password
	if password == "" && existingMount != nil {
		password = existingMount.Password
	}

	mount := &config.MountConfig{
		Name:         mountPath,
		Password:     password,
		MaxListeners: dto.MaxListeners,
		Genre:        dto.Genre,
		Description:  dto.Description,
		URL:          dto.URL,
		Bitrate:      dto.Bitrate,
		Type:         dto.Type,
		Public:       dto.Public,
		StreamName:   dto.StreamName,
		Hidden:       dto.Hidden,
		BurstSize:    dto.BurstSize,
	}

	if err := s.configManager.UpdateMount(mountPath, mount); err != nil {
		s.jsonError(w, "Failed to update mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s updated. Changes applied immediately.", mountPath),
	})
}

// handleDeleteMountConfig deletes a mount
func (s *Server) handleDeleteMountConfig(w http.ResponseWriter, r *http.Request, mountPath string) {
	if err := s.configManager.DeleteMount(mountPath); err != nil {
		s.jsonError(w, "Failed to delete mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s deleted. Changes applied immediately.", mountPath),
	})
}

// jsonResponse writes a JSON response
func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonSuccess writes a successful JSON response with data
func (s *Server) jsonSuccess(w http.ResponseWriter, data interface{}) {
	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Data:    data,
	})
}

// jsonError writes an error JSON response
func (s *Server) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ConfigAPIResponse{
		Success: false,
		Error:   message,
	})
}

// parseIntParam parses an integer from query parameters
func parseIntParam(r *http.Request, name string, defaultValue int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return i
}

// parseBoolParam parses a boolean from query parameters
func parseBoolParam(r *http.Request, name string, defaultValue bool) bool {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}
