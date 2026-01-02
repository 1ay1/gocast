// Package config handles GoCast configuration loading and management
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RuntimeState represents the persisted runtime state from admin panel changes
type RuntimeState struct {
	// Version for state file format migrations
	Version int `json:"version"`

	// LastModified timestamp
	LastModified time.Time `json:"last_modified"`

	// Setup state for zero-config mode
	SetupComplete bool   `json:"setup_complete"`
	FirstRunToken string `json:"first_run_token,omitempty"`

	// Server overrides
	Server *ServerStateOverride `json:"server,omitempty"`

	// SSL settings
	SSL *SSLStateOverride `json:"ssl,omitempty"`

	// Limits overrides
	Limits *LimitsStateOverride `json:"limits,omitempty"`

	// Auth overrides (admin credentials)
	Auth *AuthStateOverride `json:"auth,omitempty"`

	// Mount configurations (full replacement, not merge)
	Mounts map[string]*MountConfig `json:"mounts,omitempty"`

	// Logging overrides
	Logging *LoggingStateOverride `json:"logging,omitempty"`
}

// ServerStateOverride contains server settings that can be changed at runtime
type ServerStateOverride struct {
	Hostname *string `json:"hostname,omitempty"`
	Port     *int    `json:"port,omitempty"`
	Location *string `json:"location,omitempty"`
	ServerID *string `json:"server_id,omitempty"`
}

// SSLStateOverride contains SSL settings that can be changed at runtime
type SSLStateOverride struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	Port         *int    `json:"port,omitempty"`
	AutoSSL      *bool   `json:"auto_ssl,omitempty"`
	AutoSSLEmail *string `json:"auto_ssl_email,omitempty"`
	CertPath     *string `json:"cert_path,omitempty"`
	KeyPath      *string `json:"key_path,omitempty"`
	CacheDir     *string `json:"cache_dir,omitempty"`
}

// LimitsStateOverride contains limit settings that can be changed at runtime
type LimitsStateOverride struct {
	MaxClients           *int `json:"max_clients,omitempty"`
	MaxSources           *int `json:"max_sources,omitempty"`
	MaxListenersPerMount *int `json:"max_listeners_per_mount,omitempty"`
	QueueSize            *int `json:"queue_size,omitempty"`
	BurstSize            *int `json:"burst_size,omitempty"`
}

// AuthStateOverride contains auth settings that can be changed at runtime
type AuthStateOverride struct {
	SourcePassword *string `json:"source_password,omitempty"`
	AdminUser      *string `json:"admin_user,omitempty"`
	AdminPassword  *string `json:"admin_password,omitempty"`
}

// LoggingStateOverride contains logging settings that can be changed at runtime
type LoggingStateOverride struct {
	LogLevel *string `json:"log_level,omitempty"`
}

// ConfigManager handles configuration with state persistence
type ConfigManager struct {
	// Base configuration from file
	baseConfig *Config

	// Runtime state (overrides)
	state *RuntimeState

	// Merged configuration (base + state)
	mergedConfig *Config

	// File paths
	configPath string
	statePath  string
	dataDir    string

	// Zero-config mode (no config file)
	zeroConfigMode bool

	// Logger
	logger *log.Logger

	// Initial admin password (shown once on first run)
	initialAdminPassword string

	// Mutex for thread-safe access
	mu sync.RWMutex

	// Callbacks for config change notifications
	changeCallbacks []func(*Config)
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) (*ConfigManager, error) {
	return NewConfigManagerWithLogger(configPath, nil)
}

// NewConfigManagerWithLogger creates a new configuration manager with a logger
func NewConfigManagerWithLogger(configPath string, logger *log.Logger) (*ConfigManager, error) {
	if logger == nil {
		logger = log.Default()
	}

	cm := &ConfigManager{
		configPath:      configPath,
		statePath:       getStatePath(configPath),
		state:           newEmptyState(),
		changeCallbacks: make([]func(*Config), 0),
		logger:          logger,
	}

	// Load base configuration
	if err := cm.loadBaseConfig(); err != nil {
		return nil, fmt.Errorf("failed to load base config: %w", err)
	}

	// Load state file (if exists)
	if err := cm.loadState(); err != nil {
		if !os.IsNotExist(err) {
			// State file errors are not fatal - just log and continue with empty state
			logger.Printf("Warning: could not load state file: %v\n", err)
		}
		cm.state = newEmptyState()
	}

	// Merge configs
	cm.mergeConfigs()

	return cm, nil
}

// NewZeroConfigManager creates a configuration manager without a config file
// All settings are persisted to state.json in the data directory
func NewZeroConfigManager(dataDir string, logger *log.Logger) (*ConfigManager, error) {
	if logger == nil {
		logger = log.Default()
	}

	// Determine data directory
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	statePath := filepath.Join(dataDir, "config.json")

	cm := &ConfigManager{
		configPath:      "",
		statePath:       statePath,
		dataDir:         dataDir,
		zeroConfigMode:  true,
		state:           newEmptyState(),
		baseConfig:      DefaultConfig(),
		changeCallbacks: make([]func(*Config), 0),
		logger:          logger,
	}

	// Load state file (if exists)
	err := cm.loadState()
	if err != nil {
		if os.IsNotExist(err) {
			// First run - create initial secure config
			cm.state = cm.createInitialState()
			if saveErr := cm.saveState(); saveErr != nil {
				return nil, fmt.Errorf("failed to save initial state: %w", saveErr)
			}

			// Show first-run credentials
			cm.showFirstRunCredentials()
		} else {
			// Actual error loading state
			logger.Printf("Warning: could not load state file: %v", err)
			cm.state = newEmptyState()
		}
	}

	// Merge configs
	cm.mergeConfigs()

	return cm, nil
}

// getDefaultDataDir returns the default data directory based on OS
func getDefaultDataDir() string {
	// Try common locations in order
	locations := []string{
		"/var/lib/gocast",
		filepath.Join(os.Getenv("HOME"), ".gocast"),
		"./data",
	}

	for _, loc := range locations {
		if err := os.MkdirAll(loc, 0700); err == nil {
			return loc
		}
	}

	// Fallback to current directory
	return "./data"
}

// createInitialState creates secure initial state for first run
func (cm *ConfigManager) createInitialState() *RuntimeState {
	state := newEmptyState()
	state.SetupComplete = false
	state.FirstRunToken = generateSecureToken(32)

	// Generate secure admin password
	adminPassword := generateSecurePassword(16)
	cm.initialAdminPassword = adminPassword

	state.Auth = &AuthStateOverride{
		AdminUser:      strPtr("admin"),
		AdminPassword:  strPtr(adminPassword),
		SourcePassword: strPtr(generateSecurePassword(12)),
	}

	// Create default mount
	state.Mounts = map[string]*MountConfig{
		"/live": {
			Name:         "/live",
			MaxListeners: 100,
			Type:         "audio/mpeg",
			Public:       true,
			StreamName:   "Live Stream",
		},
	}

	return state
}

// showFirstRunCredentials displays the initial credentials
func (cm *ConfigManager) showFirstRunCredentials() {
	cm.logger.Println("╔════════════════════════════════════════════════════════════╗")
	cm.logger.Println("║              GOCAST FIRST-RUN SETUP                        ║")
	cm.logger.Println("╠════════════════════════════════════════════════════════════╣")
	cm.logger.Printf("║  Admin Username: %-41s ║\n", "admin")
	cm.logger.Printf("║  Admin Password: %-41s ║\n", cm.initialAdminPassword)
	cm.logger.Println("║                                                            ║")
	cm.logger.Println("║  ⚠️  SAVE THIS PASSWORD - IT WON'T BE SHOWN AGAIN!         ║")
	cm.logger.Println("║                                                            ║")
	cm.logger.Println("║  Open admin panel to complete setup and configure SSL      ║")
	cm.logger.Println("╚════════════════════════════════════════════════════════════╝")
}

// GetInitialAdminPassword returns the initial admin password (only on first run)
func (cm *ConfigManager) GetInitialAdminPassword() string {
	return cm.initialAdminPassword
}

// IsZeroConfigMode returns true if running without a config file
func (cm *ConfigManager) IsZeroConfigMode() bool {
	return cm.zeroConfigMode
}

// IsSetupComplete returns true if initial setup is complete
func (cm *ConfigManager) IsSetupComplete() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.state.SetupComplete
}

// CompleteSetup marks initial setup as complete
func (cm *ConfigManager) CompleteSetup() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.state.SetupComplete = true
	cm.state.FirstRunToken = "" // Clear the token

	return cm.saveState()
}

// GetDataDir returns the data directory path
func (cm *ConfigManager) GetDataDir() string {
	if cm.dataDir != "" {
		return cm.dataDir
	}
	return filepath.Dir(cm.statePath)
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}
	return string(bytes)
}

// generateSecureToken generates a secure random hex token
func generateSecureToken(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}

// getStatePath derives the state file path from config path
func getStatePath(configPath string) string {
	dir := filepath.Dir(configPath)
	return filepath.Join(dir, "state.json")
}

// newEmptyState creates a new empty runtime state
func newEmptyState() *RuntimeState {
	return &RuntimeState{
		Version:      1,
		LastModified: time.Now(),
		Mounts:       make(map[string]*MountConfig),
	}
}

// loadBaseConfig loads the base configuration from file
func (cm *ConfigManager) loadBaseConfig() error {
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		cm.baseConfig = DefaultConfig()
		return nil
	}

	cfg, err := Load(cm.configPath)
	if err != nil {
		return err
	}

	cm.baseConfig = cfg
	return nil
}

// loadState loads the runtime state from file
func (cm *ConfigManager) loadState() error {
	if _, err := os.Stat(cm.statePath); os.IsNotExist(err) {
		return os.ErrNotExist // Return error to signal first run
	}

	data, err := os.ReadFile(cm.statePath)
	if err != nil {
		return err
	}

	state := &RuntimeState{}
	if err := json.Unmarshal(data, state); err != nil {
		return err
	}

	cm.state = state
	return nil
}

// saveState persists the runtime state to file
func (cm *ConfigManager) saveState() error {
	cm.state.LastModified = time.Now()

	data, err := json.MarshalIndent(cm.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cm.statePath, data, 0644)
}

// mergeConfigs merges base config with state overrides
func (cm *ConfigManager) mergeConfigs() {
	// Start with a copy of base config
	merged := cm.copyConfig(cm.baseConfig)

	// Apply server overrides
	if cm.state.Server != nil {
		if cm.state.Server.Hostname != nil {
			merged.Server.Hostname = *cm.state.Server.Hostname
		}
		if cm.state.Server.Port != nil {
			merged.Server.Port = *cm.state.Server.Port
		}
		if cm.state.Server.Location != nil {
			merged.Server.Location = *cm.state.Server.Location
		}
		if cm.state.Server.ServerID != nil {
			merged.Server.ServerID = *cm.state.Server.ServerID
		}
	}

	// Apply SSL overrides
	if cm.state.SSL != nil {
		if cm.state.SSL.Enabled != nil {
			merged.Server.SSLEnabled = *cm.state.SSL.Enabled
		}
		if cm.state.SSL.Port != nil {
			merged.Server.SSLPort = *cm.state.SSL.Port
		}
		if cm.state.SSL.AutoSSL != nil {
			merged.Server.AutoSSL = *cm.state.SSL.AutoSSL
		}
		if cm.state.SSL.AutoSSLEmail != nil {
			merged.Server.AutoSSLEmail = *cm.state.SSL.AutoSSLEmail
		}
		if cm.state.SSL.CertPath != nil {
			merged.Server.SSLCert = *cm.state.SSL.CertPath
		}
		if cm.state.SSL.KeyPath != nil {
			merged.Server.SSLKey = *cm.state.SSL.KeyPath
		}
		if cm.state.SSL.CacheDir != nil {
			merged.Server.AutoSSLCache = *cm.state.SSL.CacheDir
		}
	}

	// Apply limits overrides
	if cm.state.Limits != nil {
		if cm.state.Limits.MaxClients != nil {
			merged.Limits.MaxClients = *cm.state.Limits.MaxClients
		}
		if cm.state.Limits.MaxSources != nil {
			merged.Limits.MaxSources = *cm.state.Limits.MaxSources
		}
		if cm.state.Limits.MaxListenersPerMount != nil {
			merged.Limits.MaxListenersPerMount = *cm.state.Limits.MaxListenersPerMount
		}
		if cm.state.Limits.QueueSize != nil {
			merged.Limits.QueueSize = *cm.state.Limits.QueueSize
		}
		if cm.state.Limits.BurstSize != nil {
			merged.Limits.BurstSize = *cm.state.Limits.BurstSize
		}
	}

	// Apply auth overrides
	if cm.state.Auth != nil {
		if cm.state.Auth.SourcePassword != nil {
			merged.Auth.SourcePassword = *cm.state.Auth.SourcePassword
		}
		if cm.state.Auth.AdminUser != nil {
			merged.Admin.User = *cm.state.Auth.AdminUser
			merged.Auth.AdminUser = *cm.state.Auth.AdminUser
		}
		if cm.state.Auth.AdminPassword != nil {
			merged.Admin.Password = *cm.state.Auth.AdminPassword
			merged.Auth.AdminPassword = *cm.state.Auth.AdminPassword
		}
	}

	// Apply logging overrides
	if cm.state.Logging != nil {
		if cm.state.Logging.LogLevel != nil {
			merged.Logging.LogLevel = *cm.state.Logging.LogLevel
		}
	}

	// Apply mount overrides - mounts from state completely replace base mounts
	if len(cm.state.Mounts) > 0 {
		merged.Mounts = make(map[string]*MountConfig)
		for path, mount := range cm.state.Mounts {
			merged.Mounts[path] = cm.copyMountConfig(mount)
		}
	}

	cm.mergedConfig = merged
}

// copyConfig creates a deep copy of a Config
func (cm *ConfigManager) copyConfig(src *Config) *Config {
	dst := &Config{
		Server:    src.Server,
		Limits:    src.Limits,
		Auth:      src.Auth,
		Logging:   src.Logging,
		Admin:     src.Admin,
		Directory: src.Directory,
		Mounts:    make(map[string]*MountConfig),
	}

	for path, mount := range src.Mounts {
		dst.Mounts[path] = cm.copyMountConfig(mount)
	}

	return dst
}

// copyMountConfig creates a deep copy of a MountConfig
func (cm *ConfigManager) copyMountConfig(src *MountConfig) *MountConfig {
	dst := *src
	if src.AllowedIPs != nil {
		dst.AllowedIPs = make([]string, len(src.AllowedIPs))
		copy(dst.AllowedIPs, src.AllowedIPs)
	}
	if src.DeniedIPs != nil {
		dst.DeniedIPs = make([]string, len(src.DeniedIPs))
		copy(dst.DeniedIPs, src.DeniedIPs)
	}
	return &dst
}

// GetConfig returns the current merged configuration
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.mergedConfig
}

// GetBaseConfig returns the base configuration (from file)
func (cm *ConfigManager) GetBaseConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.baseConfig
}

// GetState returns the current runtime state
func (cm *ConfigManager) GetState() *RuntimeState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.state
}

// OnChange registers a callback for configuration changes
func (cm *ConfigManager) OnChange(callback func(*Config)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.changeCallbacks = append(cm.changeCallbacks, callback)
}

// notifyChange notifies all registered callbacks of a config change
func (cm *ConfigManager) notifyChange() {
	for _, cb := range cm.changeCallbacks {
		go cb(cm.mergedConfig)
	}
}

// UpdateServer updates server configuration
func (cm *ConfigManager) UpdateServer(hostname, location, serverID *string, port *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state.Server == nil {
		cm.state.Server = &ServerStateOverride{}
	}

	if hostname != nil {
		cm.state.Server.Hostname = hostname
	}
	if port != nil {
		cm.state.Server.Port = port
	}
	if location != nil {
		cm.state.Server.Location = location
	}
	if serverID != nil {
		cm.state.Server.ServerID = serverID
	}

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateSSL updates SSL configuration
func (cm *ConfigManager) UpdateSSL(enabled, autoSSL *bool, port *int, email, certPath, keyPath, cacheDir *string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state.SSL == nil {
		cm.state.SSL = &SSLStateOverride{}
	}

	if enabled != nil {
		cm.state.SSL.Enabled = enabled
	}
	if autoSSL != nil {
		cm.state.SSL.AutoSSL = autoSSL
	}
	if port != nil {
		cm.state.SSL.Port = port
	}
	if email != nil {
		cm.state.SSL.AutoSSLEmail = email
	}
	if certPath != nil {
		cm.state.SSL.CertPath = certPath
	}
	if keyPath != nil {
		cm.state.SSL.KeyPath = keyPath
	}
	if cacheDir != nil {
		cm.state.SSL.CacheDir = cacheDir
	}

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// EnableAutoSSL enables automatic SSL with Let's Encrypt
func (cm *ConfigManager) EnableAutoSSL(hostname, email string) error {
	// Update hostname first
	if err := cm.UpdateServer(&hostname, nil, nil, nil); err != nil {
		return err
	}

	// Set default cache directory
	cacheDir := filepath.Join(cm.GetDataDir(), "certs")

	// Enable AutoSSL
	enabled := true
	autoSSL := true
	port := 443

	return cm.UpdateSSL(&enabled, &autoSSL, &port, &email, nil, nil, &cacheDir)
}

// DisableSSL disables SSL
func (cm *ConfigManager) DisableSSL() error {
	enabled := false
	autoSSL := false
	return cm.UpdateSSL(&enabled, &autoSSL, nil, nil, nil, nil, nil)
}

// UpdateLimits updates limits configuration
func (cm *ConfigManager) UpdateLimits(maxClients, maxSources, maxListenersPerMount, queueSize, burstSize *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state.Limits == nil {
		cm.state.Limits = &LimitsStateOverride{}
	}

	if maxClients != nil {
		cm.state.Limits.MaxClients = maxClients
	}
	if maxSources != nil {
		cm.state.Limits.MaxSources = maxSources
	}
	if maxListenersPerMount != nil {
		cm.state.Limits.MaxListenersPerMount = maxListenersPerMount
	}
	if queueSize != nil {
		cm.state.Limits.QueueSize = queueSize
	}
	if burstSize != nil {
		cm.state.Limits.BurstSize = burstSize
	}

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateAuth updates authentication configuration
func (cm *ConfigManager) UpdateAuth(sourcePassword, adminUser, adminPassword *string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state.Auth == nil {
		cm.state.Auth = &AuthStateOverride{}
	}

	if sourcePassword != nil {
		cm.state.Auth.SourcePassword = sourcePassword
	}
	if adminUser != nil {
		cm.state.Auth.AdminUser = adminUser
	}
	if adminPassword != nil {
		cm.state.Auth.AdminPassword = adminPassword
	}

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateLogging updates logging configuration
func (cm *ConfigManager) UpdateLogging(logLevel *string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.state.Logging == nil {
		cm.state.Logging = &LoggingStateOverride{}
	}

	if logLevel != nil {
		cm.state.Logging.LogLevel = logLevel
	}

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// CreateMount creates a new mount configuration
func (cm *ConfigManager) CreateMount(path string, mount *MountConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Ensure path starts with /
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}

	// Check if mount already exists
	if _, exists := cm.state.Mounts[path]; exists {
		return fmt.Errorf("mount %s already exists", path)
	}

	// If this is the first mount in state, copy all base mounts first
	if len(cm.state.Mounts) == 0 && len(cm.baseConfig.Mounts) > 0 {
		for p, m := range cm.baseConfig.Mounts {
			cm.state.Mounts[p] = cm.copyMountConfig(m)
		}
	}

	mount.Name = path
	cm.state.Mounts[path] = mount

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateMount updates an existing mount configuration
func (cm *ConfigManager) UpdateMount(path string, mount *MountConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Ensure path starts with /
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}

	// If this is the first update in state, copy all base mounts first
	if len(cm.state.Mounts) == 0 && len(cm.baseConfig.Mounts) > 0 {
		for p, m := range cm.baseConfig.Mounts {
			cm.state.Mounts[p] = cm.copyMountConfig(m)
		}
	}

	mount.Name = path
	cm.state.Mounts[path] = mount

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// DeleteMount removes a mount configuration
func (cm *ConfigManager) DeleteMount(path string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Ensure path starts with /
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}

	// If this is the first delete in state, copy all base mounts first
	if len(cm.state.Mounts) == 0 && len(cm.baseConfig.Mounts) > 0 {
		for p, m := range cm.baseConfig.Mounts {
			cm.state.Mounts[p] = cm.copyMountConfig(m)
		}
	}

	delete(cm.state.Mounts, path)

	cm.mergeConfigs()
	if err := cm.saveState(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// GetMount returns a specific mount configuration
func (cm *ConfigManager) GetMount(path string) *MountConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if mount, exists := cm.mergedConfig.Mounts[path]; exists {
		return mount
	}
	return nil
}

// GetAllMounts returns all mount configurations
func (cm *ConfigManager) GetAllMounts() map[string]*MountConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]*MountConfig)
	for path, mount := range cm.mergedConfig.Mounts {
		result[path] = cm.copyMountConfig(mount)
	}
	return result
}

// ResetToDefaults resets all runtime state to base config defaults
func (cm *ConfigManager) ResetToDefaults() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.state = newEmptyState()
	cm.mergeConfigs()

	// Remove state file
	if err := os.Remove(cm.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	cm.notifyChange()
	return nil
}

// ReloadBaseConfig reloads the base configuration from file
func (cm *ConfigManager) ReloadBaseConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := cm.loadBaseConfig(); err != nil {
		return err
	}

	cm.mergeConfigs()
	cm.notifyChange()
	return nil
}

// ExportConfig exports the current merged config as JSON
func (cm *ConfigManager) ExportConfig() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return json.MarshalIndent(cm.mergedConfig, "", "  ")
}

// HasStateOverrides returns true if there are any runtime overrides
func (cm *ConfigManager) HasStateOverrides() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.state.Server != nil ||
		cm.state.Limits != nil ||
		cm.state.Auth != nil ||
		cm.state.Logging != nil ||
		len(cm.state.Mounts) > 0
}
