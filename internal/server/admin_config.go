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
	Hostname string `json:"hostname"`
	Location string `json:"location"`
	ServerID string `json:"server_id"`
	Port     int    `json:"port"`
	SSLPort  int    `json:"ssl_port"`
}

// SSLConfigDTO represents SSL configuration for API
type SSLConfigDTO struct {
	Enabled      bool   `json:"enabled"`
	AutoSSL      bool   `json:"auto_ssl"`
	AutoSSLEmail string `json:"auto_ssl_email,omitempty"`
	Port         int    `json:"ssl_port"`
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

// FullConfigDTO represents the complete configuration for API
type FullConfigDTO struct {
	Server        ServerConfigDTO           `json:"server"`
	SSL           SSLConfigDTO              `json:"ssl"`
	Limits        LimitsConfigDTO           `json:"limits"`
	Auth          AuthConfigDTO             `json:"auth"`
	Mounts        map[string]MountConfigDTO `json:"mounts"`
	HasOverrides  bool                      `json:"has_overrides"`
	LastModified  string                    `json:"last_modified,omitempty"`
	ZeroConfig    bool                      `json:"zero_config"`
	SetupComplete bool                      `json:"setup_complete"`
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
	case path == "/admin/config/auth" && r.Method == http.MethodPost:
		s.handleUpdateAuthConfig(w, r)
	case strings.HasPrefix(path, "/admin/config/mounts"):
		s.handleMountsConfig(w, r)
	default:
		s.jsonError(w, "Not found", http.StatusNotFound)
	}
}

// handleGetConfig returns the current configuration
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()
	state := s.configManager.GetState()

	dto := FullConfigDTO{
		Server: ServerConfigDTO{
			Hostname: cfg.Server.Hostname,
			Location: cfg.Server.Location,
			ServerID: cfg.Server.ServerID,
			Port:     cfg.Server.Port,
			SSLPort:  cfg.Server.SSLPort,
		},
		SSL: SSLConfigDTO{
			Enabled:      cfg.Server.SSLEnabled,
			AutoSSL:      cfg.Server.AutoSSL,
			AutoSSLEmail: cfg.Server.AutoSSLEmail,
			Port:         cfg.Server.SSLPort,
			CertPath:     cfg.Server.SSLCert,
			KeyPath:      cfg.Server.SSLKey,
			Hostname:     cfg.Server.Hostname,
		},
		Limits: LimitsConfigDTO{
			MaxClients:           cfg.Limits.MaxClients,
			MaxSources:           cfg.Limits.MaxSources,
			MaxListenersPerMount: cfg.Limits.MaxListenersPerMount,
			QueueSize:            cfg.Limits.QueueSize,
			BurstSize:            cfg.Limits.BurstSize,
		},
		Auth: AuthConfigDTO{
			SourcePassword: cfg.Auth.SourcePassword,
			AdminUser:      cfg.Admin.User,
			// Don't expose admin password
		},
		Mounts:        make(map[string]MountConfigDTO),
		HasOverrides:  s.configManager.HasStateOverrides(),
		LastModified:  state.LastModified.Format(time.RFC3339),
		ZeroConfig:    s.configManager.IsZeroConfigMode(),
		SetupComplete: s.configManager.IsSetupComplete(),
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

// handleUpdateConfig handles full configuration update
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var dto FullConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update server config
	if err := s.configManager.UpdateServer(&dto.Server.Hostname, &dto.Server.Location, &dto.Server.ServerID, nil); err != nil {
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
		Message: "Configuration updated successfully",
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
		Message: "Configuration reset to defaults",
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

	if err := s.configManager.UpdateServer(&dto.Hostname, &dto.Location, &dto.ServerID, nil); err != nil {
		s.jsonError(w, "Failed to update server config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Server configuration updated",
	})
}

// handleGetSSLConfig returns the current SSL configuration
func (s *Server) handleGetSSLConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()

	dto := SSLConfigDTO{
		Enabled:      cfg.Server.SSLEnabled,
		AutoSSL:      cfg.Server.AutoSSL,
		AutoSSLEmail: cfg.Server.AutoSSLEmail,
		Port:         cfg.Server.SSLPort,
		CertPath:     cfg.Server.SSLCert,
		KeyPath:      cfg.Server.SSLKey,
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
		nil,
	); err != nil {
		s.jsonError(w, "Failed to update SSL config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "SSL configuration updated. Restart server to apply changes.",
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
		Message: "AutoSSL enabled. Restart server to obtain certificate.",
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
		Message: "SSL disabled. Restart server to apply changes.",
	})
}

// handleUpdateLimitsConfig updates limits configuration
func (s *Server) handleUpdateLimitsConfig(w http.ResponseWriter, r *http.Request) {
	var dto LimitsConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.configManager.UpdateLimits(
		&dto.MaxClients,
		&dto.MaxSources,
		&dto.MaxListenersPerMount,
		&dto.QueueSize,
		&dto.BurstSize,
	); err != nil {
		s.jsonError(w, "Failed to update limits config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Limits configuration updated",
	})
}

// handleUpdateAuthConfig updates auth configuration
func (s *Server) handleUpdateAuthConfig(w http.ResponseWriter, r *http.Request) {
	var dto AuthConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	var adminPass *string
	if dto.AdminPassword != "" {
		adminPass = &dto.AdminPassword
	}

	if err := s.configManager.UpdateAuth(&dto.SourcePassword, &dto.AdminUser, adminPass); err != nil {
		s.jsonError(w, "Failed to update auth config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: "Auth configuration updated",
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

	// URL decode the mount path
	mountPath = strings.ReplaceAll(mountPath, "%2F", "/")

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

	dtos := make([]MountConfigDTO, 0, len(mounts))
	for path, mount := range mounts {
		dtos = append(dtos, MountConfigDTO{
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
		})
	}

	s.jsonSuccess(w, dtos)
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

	// Set defaults
	if mount.MaxListeners == 0 {
		mount.MaxListeners = s.config.Limits.MaxListenersPerMount
	}
	if mount.Type == "" {
		mount.Type = "audio/mpeg"
	}
	if mount.Bitrate == 0 {
		mount.Bitrate = 128
	}
	if mount.BurstSize == 0 {
		mount.BurstSize = s.config.Limits.BurstSize
	}
	if mount.Password == "" {
		mount.Password = s.config.Auth.SourcePassword
	}

	if err := s.configManager.CreateMount(dto.Path, mount); err != nil {
		s.jsonError(w, "Failed to create mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Also create the mount in the mount manager
	s.mountManager.GetOrCreateMount(dto.Path)

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s created", dto.Path),
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

// handleUpdateMountConfig updates a mount configuration
func (s *Server) handleUpdateMountConfig(w http.ResponseWriter, r *http.Request, mountPath string) {
	var dto MountConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	mount := &config.MountConfig{
		Name:         mountPath,
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

	if err := s.configManager.UpdateMount(mountPath, mount); err != nil {
		s.jsonError(w, "Failed to update mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s updated", mountPath),
	})
}

// handleDeleteMountConfig deletes a mount configuration
func (s *Server) handleDeleteMountConfig(w http.ResponseWriter, r *http.Request, mountPath string) {
	if err := s.configManager.DeleteMount(mountPath); err != nil {
		s.jsonError(w, "Failed to delete mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Message: fmt.Sprintf("Mount %s deleted", mountPath),
	})
}

// handleGetLimitsConfig returns limits configuration (for quick access)
func (s *Server) handleGetLimitsConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configManager.GetConfig()

	dto := LimitsConfigDTO{
		MaxClients:           cfg.Limits.MaxClients,
		MaxSources:           cfg.Limits.MaxSources,
		MaxListenersPerMount: cfg.Limits.MaxListenersPerMount,
		QueueSize:            cfg.Limits.QueueSize,
		BurstSize:            cfg.Limits.BurstSize,
	}

	s.jsonSuccess(w, dto)
}

// JSON helper methods

func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonSuccess(w http.ResponseWriter, data interface{}) {
	s.jsonResponse(w, ConfigAPIResponse{
		Success: true,
		Data:    data,
	})
}

func (s *Server) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ConfigAPIResponse{
		Success: false,
		Error:   message,
	})
}

// parseIntParam parses an integer from query parameter
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// parseBoolParam parses a boolean from query parameter
func parseBoolParam(r *http.Request, name string, defaultVal bool) bool {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	return val == "1" || val == "true" || val == "yes"
}
