// Package config handles GoCast configuration loading and management.
//
// # Configuration Architecture
//
// GoCast uses a single JSON configuration file (config.json) that stores
// all settings. This file can be:
//   - Edited via the Admin Panel (recommended)
//   - Edited manually and hot-reloaded via admin panel or SIGHUP
//
// # Default Location
//
// The config file is stored at:
//   - ~/.gocast/config.json (default)
//   - Custom path via -data flag
//
// # Example config.json
//
//	{
//	  "version": 1,
//	  "setup_complete": true,
//	  "server": {
//	    "hostname": "radio.example.com",
//	    "listen_address": "0.0.0.0",
//	    "port": 8000
//	  },
//	  "ssl": {
//	    "enabled": true,
//	    "auto_ssl": true,
//	    "port": 8443
//	  },
//	  "limits": {
//	    "max_clients": 500,
//	    "max_sources": 10
//	  },
//	  "auth": {
//	    "source_password": "secret",
//	    "admin_user": "admin",
//	    "admin_password": "secret"
//	  },
//	  "mounts": {
//	    "/live": {
//	      "name": "/live",
//	      "max_listeners": 100,
//	      "public": true
//	    }
//	  }
//	}
//
// # Security Note
//
// The config file contains sensitive information (passwords). Ensure proper
// file permissions (chmod 600) and keep backups secure.
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

// ConfigManager handles configuration with hot reload support
type ConfigManager struct {
	// Current active configuration
	config *Config

	// Path to config file
	configPath string
	dataDir    string

	// Path to last-known-good backup
	backupPath string

	// Logger for warnings/errors
	logger *log.Logger

	// Initial admin password (shown on first run)
	initialAdminPassword string

	// Mutex for thread-safe access
	mu sync.RWMutex

	// Change callbacks for hot-reload
	changeCallbacks []func(*Config)
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(dataDir string, logger *log.Logger) (*ConfigManager, error) {
	if logger == nil {
		logger = log.Default()
	}

	// Determine data directory
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	configPath := filepath.Join(dataDir, "config.json")
	backupPath := filepath.Join(dataDir, "config.json.backup")

	cm := &ConfigManager{
		configPath:      configPath,
		backupPath:      backupPath,
		dataDir:         dataDir,
		config:          DefaultConfig(),
		changeCallbacks: make([]func(*Config), 0),
		logger:          logger,
	}

	// Try to load existing config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// First run - create initial config
		cm.config = cm.createInitialConfig()
		if err := cm.save(); err != nil {
			return nil, fmt.Errorf("failed to save initial config: %w", err)
		}
		cm.showFirstRunCredentials()
	} else {
		// Load existing config
		if err := cm.load(); err != nil {
			logger.Printf("WARNING: Failed to load config: %v", err)

			// Try to recover from backup
			if cm.tryRecoverFromBackup() {
				logger.Println("Successfully recovered config from backup")
			} else {
				logger.Println("No backup available, using defaults")
				cm.config = cm.createInitialConfig()
				// Save the default config to replace the corrupted one
				if err := cm.save(); err != nil {
					logger.Printf("WARNING: Failed to save default config: %v", err)
				}
				cm.showFirstRunCredentials()
			}
		}
	}

	return cm, nil
}

// getDefaultDataDir returns the default data directory path
func getDefaultDataDir() string {
	// Try user home directory first
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".gocast")
	}

	// Fallback to current directory
	return ".gocast"
}

// createInitialConfig creates a new config with secure generated credentials
func (cm *ConfigManager) createInitialConfig() *Config {
	cfg := DefaultConfig()
	cfg.SetupComplete = false

	// Generate secure admin password
	adminPassword := generateSecurePassword(16)
	cm.initialAdminPassword = adminPassword

	cfg.Auth.AdminUser = "admin"
	cfg.Auth.AdminPassword = adminPassword
	cfg.Admin.User = "admin"
	cfg.Admin.Password = adminPassword

	// Generate secure source password
	cfg.Auth.SourcePassword = generateSecurePassword(12)

	// Create default mount
	cfg.Mounts = map[string]*MountConfig{
		"/live": {
			Name:         "/live",
			MaxListeners: cfg.Limits.MaxListenersPerMount,
			Genre:        "Various",
			Description:  "GoCast Stream",
			Bitrate:      128,
			Type:         "audio/mpeg",
			Public:       true,
			StreamName:   "Live Stream",
			BurstSize:    cfg.Limits.BurstSize,
		},
	}

	// Set default SSL cache directory
	cfg.SSL.CacheDir = filepath.Join(cm.dataDir, "certs")

	return cfg
}

// showFirstRunCredentials displays the initial credentials to the user
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

// GetInitialAdminPassword returns the initial admin password (only available on first run)
func (cm *ConfigManager) GetInitialAdminPassword() string {
	return cm.initialAdminPassword
}

// GetDataDir returns the data directory path
func (cm *ConfigManager) GetDataDir() string {
	return cm.dataDir
}

// GetConfigPath returns the config file path
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

// IsSetupComplete returns whether initial setup has been completed
func (cm *ConfigManager) IsSetupComplete() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.SetupComplete
}

// CompleteSetup marks the initial setup as complete
func (cm *ConfigManager) CompleteSetup() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.SetupComplete = true
	return cm.saveUnlocked()
}

// load reads configuration from disk
func (cm *ConfigManager) load() error {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		// Config is corrupted - backup and report error
		cm.backupCorruptedConfig(data)
		return fmt.Errorf("config file corrupted (backup created): %w", err)
	}

	// Validate and fix any issues
	warnings := cm.validateAndFix(cfg)
	for _, w := range warnings {
		cm.logger.Printf("CONFIG WARNING: %s", w)
	}

	// Convert seconds to durations
	cfg.normalizeDurations()

	// Ensure mounts map exists
	if cfg.Mounts == nil {
		cfg.Mounts = make(map[string]*MountConfig)
	}

	cm.config = cfg
	return nil
}

// tryRecoverFromBackup attempts to load config from the backup file
func (cm *ConfigManager) tryRecoverFromBackup() bool {
	if _, err := os.Stat(cm.backupPath); os.IsNotExist(err) {
		return false
	}

	data, err := os.ReadFile(cm.backupPath)
	if err != nil {
		cm.logger.Printf("Failed to read backup file: %v", err)
		return false
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		cm.logger.Printf("Backup file is also corrupted: %v", err)
		return false
	}

	// Validate and fix
	cm.validateAndFix(cfg)
	cfg.normalizeDurations()

	if cfg.Mounts == nil {
		cfg.Mounts = make(map[string]*MountConfig)
	}

	cm.config = cfg

	// Save the recovered config as the main config
	if err := cm.saveUnlocked(); err != nil {
		cm.logger.Printf("Failed to save recovered config: %v", err)
	}

	return true
}

// saveBackup saves a copy of the current config as a backup
func (cm *ConfigManager) saveBackup() error {
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cm.backupPath, data, 0600)
}

// save persists configuration to disk (requires lock held)
func (cm *ConfigManager) save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.saveUnlocked()
}

// saveUnlocked persists configuration to disk (caller must hold lock)
func (cm *ConfigManager) saveUnlocked() error {
	cm.config.LastModified = time.Now()

	// Ensure seconds fields are updated
	cm.config.normalizeSeconds()

	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save a backup of current config before overwriting (if file exists)
	if _, err := os.Stat(cm.configPath); err == nil {
		if err := cm.saveBackup(); err != nil {
			cm.logger.Printf("WARNING: Failed to save config backup: %v", err)
		}
	}

	// Atomic write: write to temp file then rename
	tempPath := cm.configPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tempPath, cm.configPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to save config file: %w", err)
	}

	return nil
}

// backupCorruptedConfig creates a backup of a corrupted config file
func (cm *ConfigManager) backupCorruptedConfig(data []byte) {
	corruptedPath := cm.configPath + ".corrupted." + time.Now().Format("20060102-150405")
	if err := os.WriteFile(corruptedPath, data, 0600); err != nil {
		cm.logger.Printf("WARNING: Failed to backup corrupted config: %v", err)
	} else {
		cm.logger.Printf("Corrupted config backed up to: %s", corruptedPath)
	}
}

// validateAndFix validates configuration and fixes any issues
// Returns a list of warnings for issues that were auto-fixed
func (cm *ConfigManager) validateAndFix(cfg *Config) []string {
	var warnings []string

	// Fix invalid limits
	if cfg.Limits.MaxClients <= 0 {
		warnings = append(warnings, "Invalid max_clients, setting to 100")
		cfg.Limits.MaxClients = 100
	}
	if cfg.Limits.MaxClients > 100000 {
		warnings = append(warnings, "max_clients too high, capping at 100000")
		cfg.Limits.MaxClients = 100000
	}
	if cfg.Limits.MaxSources <= 0 {
		warnings = append(warnings, "Invalid max_sources, setting to 10")
		cfg.Limits.MaxSources = 10
	}
	if cfg.Limits.MaxSources > 1000 {
		warnings = append(warnings, "max_sources too high, capping at 1000")
		cfg.Limits.MaxSources = 1000
	}
	if cfg.Limits.MaxListenersPerMount <= 0 {
		warnings = append(warnings, "Invalid max_listeners_per_mount, setting to 100")
		cfg.Limits.MaxListenersPerMount = 100
	}
	if cfg.Limits.QueueSize < 1024 {
		warnings = append(warnings, "queue_size too small, setting to 1024")
		cfg.Limits.QueueSize = 1024
	}
	if cfg.Limits.QueueSize > 10*1024*1024 {
		warnings = append(warnings, "queue_size too large (>10MB), capping at 10MB")
		cfg.Limits.QueueSize = 10 * 1024 * 1024
	}
	if cfg.Limits.BurstSize < 0 {
		warnings = append(warnings, "Invalid burst_size, setting to 2048")
		cfg.Limits.BurstSize = 2048
	}

	// Fix invalid timeouts
	if cfg.Limits.ClientTimeoutSeconds <= 0 {
		cfg.Limits.ClientTimeoutSeconds = 30
	}
	if cfg.Limits.ClientTimeoutSeconds > 3600 {
		warnings = append(warnings, "client_timeout too high, capping at 3600s")
		cfg.Limits.ClientTimeoutSeconds = 3600
	}
	if cfg.Limits.HeaderTimeoutSeconds <= 0 {
		cfg.Limits.HeaderTimeoutSeconds = 5
	}
	if cfg.Limits.SourceTimeoutSeconds <= 0 {
		cfg.Limits.SourceTimeoutSeconds = 5
	}

	// Fix invalid ports
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		warnings = append(warnings, "Invalid server port, setting to 8000")
		cfg.Server.Port = 8000
	}
	if cfg.SSL.Port <= 0 || cfg.SSL.Port > 65535 {
		warnings = append(warnings, "Invalid SSL port, setting to 8443")
		cfg.SSL.Port = 8443
	}

	// Fix missing server settings
	if cfg.Server.ListenAddress == "" {
		cfg.Server.ListenAddress = "0.0.0.0"
	}
	if cfg.Server.Hostname == "" {
		cfg.Server.Hostname = "localhost"
	}
	if cfg.Server.AdminRoot == "" {
		cfg.Server.AdminRoot = "/admin"
	}

	// Fix missing auth
	if cfg.Auth.AdminUser == "" {
		warnings = append(warnings, "No admin username, setting to 'admin'")
		cfg.Auth.AdminUser = "admin"
	}
	if cfg.Auth.AdminPassword == "" {
		newPass := generateSecurePassword(16)
		warnings = append(warnings, fmt.Sprintf("No admin password set, generated: %s", newPass))
		cfg.Auth.AdminPassword = newPass
	}
	if cfg.Auth.SourcePassword == "" {
		newPass := generateSecurePassword(12)
		warnings = append(warnings, fmt.Sprintf("No source password set, generated: %s", newPass))
		cfg.Auth.SourcePassword = newPass
	}

	// Sync admin config
	cfg.Admin.User = cfg.Auth.AdminUser
	cfg.Admin.Password = cfg.Auth.AdminPassword

	// Validate and fix mount configurations
	for path, mount := range cfg.Mounts {
		mountWarnings := cm.validateMount(path, mount)
		warnings = append(warnings, mountWarnings...)
	}

	// Ensure version is set
	if cfg.Version == 0 {
		cfg.Version = 1
	}

	// Validate logging
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[cfg.Logging.LogLevel] {
		warnings = append(warnings, fmt.Sprintf("Invalid log_level '%s', setting to 'info'", cfg.Logging.LogLevel))
		cfg.Logging.LogLevel = "info"
	}
	if cfg.Logging.LogSize <= 0 {
		cfg.Logging.LogSize = 10000
	}

	// Validate directory settings
	if cfg.Directory.IntervalSeconds < 60 && cfg.Directory.Enabled {
		warnings = append(warnings, "Directory interval too short, setting to 60s minimum")
		cfg.Directory.IntervalSeconds = 60
	}

	return warnings
}

// validateMount validates a single mount configuration and fixes issues
func (cm *ConfigManager) validateMount(path string, mount *MountConfig) []string {
	var warnings []string

	if mount == nil {
		return warnings
	}

	// Fix mount name
	if mount.Name == "" {
		mount.Name = path
	}

	// Fix invalid max_listeners
	if mount.MaxListeners <= 0 {
		warnings = append(warnings, fmt.Sprintf("Mount %s: invalid max_listeners, setting to 100", path))
		mount.MaxListeners = 100
	}
	if mount.MaxListeners > 100000 {
		warnings = append(warnings, fmt.Sprintf("Mount %s: max_listeners too high, capping at 100000", path))
		mount.MaxListeners = 100000
	}

	// Fix invalid bitrate
	if mount.Bitrate < 0 {
		mount.Bitrate = 128
	}
	if mount.Bitrate > 10000 {
		warnings = append(warnings, fmt.Sprintf("Mount %s: bitrate suspiciously high (%d), capping at 10000", path, mount.Bitrate))
		mount.Bitrate = 10000
	}

	// Fix content type
	if mount.Type == "" {
		mount.Type = "audio/mpeg"
	}

	// Fix burst size
	if mount.BurstSize < 0 {
		mount.BurstSize = 2048
	}
	if mount.BurstSize > 1024*1024 {
		warnings = append(warnings, fmt.Sprintf("Mount %s: burst_size too large (>1MB), capping at 1MB", path))
		mount.BurstSize = 1024 * 1024
	}

	return warnings
}

// Reload reloads configuration from disk (hot reload)
// This is a safe reload - if the new config is invalid, the old config is kept
func (cm *ConfigManager) Reload() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Save current config as backup before attempting reload
	oldConfig := cm.config.Clone()

	if err := cm.load(); err != nil {
		// Restore old config on failure
		cm.config = oldConfig
		return fmt.Errorf("reload failed (keeping previous config): %w", err)
	}

	cm.notifyChange()
	return nil
}

// GetConfig returns the current configuration (read-only copy)
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Clone()
}

// GetConfigDirect returns a direct pointer to the config (use carefully)
func (cm *ConfigManager) GetConfigDirect() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
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
		go cb(cm.config.Clone())
	}
}

// ----- Update Methods -----

// UpdateServer updates server configuration (changes apply immediately)
func (cm *ConfigManager) UpdateServer(hostname, location, serverID, listenAddress, adminRoot *string, port *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if hostname != nil {
		cm.config.Server.Hostname = *hostname
	}
	if location != nil {
		cm.config.Server.Location = *location
	}
	if serverID != nil {
		cm.config.Server.ServerID = *serverID
	}
	if listenAddress != nil {
		cm.config.Server.ListenAddress = *listenAddress
	}
	if adminRoot != nil {
		cm.config.Server.AdminRoot = *adminRoot
	}
	if port != nil {
		cm.config.Server.Port = *port
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateSSL updates SSL configuration (changes apply immediately)
func (cm *ConfigManager) UpdateSSL(enabled, autoSSL *bool, port *int, email, certPath, keyPath *string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if enabled != nil {
		cm.config.SSL.Enabled = *enabled
	}
	if autoSSL != nil {
		cm.config.SSL.AutoSSL = *autoSSL
	}
	if port != nil {
		cm.config.SSL.Port = *port
	}
	if email != nil {
		cm.config.SSL.AutoSSLEmail = *email
	}
	if certPath != nil {
		cm.config.SSL.CertPath = *certPath
	}
	if keyPath != nil {
		cm.config.SSL.KeyPath = *keyPath
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// EnableAutoSSL enables automatic SSL with Let's Encrypt (applies immediately)
func (cm *ConfigManager) EnableAutoSSL(hostname, email string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.Server.Hostname = hostname
	cm.config.SSL.Enabled = true
	cm.config.SSL.AutoSSL = true
	cm.config.SSL.Port = 8443
	cm.config.SSL.AutoSSLEmail = email
	cm.config.SSL.CacheDir = filepath.Join(cm.dataDir, "certs")

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// DisableSSL disables SSL (applies immediately)
func (cm *ConfigManager) DisableSSL() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config.SSL.Enabled = false
	cm.config.SSL.AutoSSL = false

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateLimits updates limits configuration
func (cm *ConfigManager) UpdateLimits(maxClients, maxSources, maxListenersPerMount, queueSize, burstSize, clientTimeout, headerTimeout, sourceTimeout *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if maxClients != nil {
		cm.config.Limits.MaxClients = *maxClients
	}
	if maxSources != nil {
		cm.config.Limits.MaxSources = *maxSources
	}
	if maxListenersPerMount != nil {
		cm.config.Limits.MaxListenersPerMount = *maxListenersPerMount
	}
	if queueSize != nil {
		cm.config.Limits.QueueSize = *queueSize
	}
	if burstSize != nil {
		cm.config.Limits.BurstSize = *burstSize
	}
	if clientTimeout != nil {
		cm.config.Limits.ClientTimeoutSeconds = *clientTimeout
		cm.config.Limits.ClientTimeout = time.Duration(*clientTimeout) * time.Second
	}
	if headerTimeout != nil {
		cm.config.Limits.HeaderTimeoutSeconds = *headerTimeout
		cm.config.Limits.HeaderTimeout = time.Duration(*headerTimeout) * time.Second
	}
	if sourceTimeout != nil {
		cm.config.Limits.SourceTimeoutSeconds = *sourceTimeout
		cm.config.Limits.SourceTimeout = time.Duration(*sourceTimeout) * time.Second
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateAuth updates authentication configuration
func (cm *ConfigManager) UpdateAuth(sourcePassword, adminUser, adminPassword *string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if sourcePassword != nil {
		cm.config.Auth.SourcePassword = *sourcePassword
	}
	if adminUser != nil {
		cm.config.Auth.AdminUser = *adminUser
		cm.config.Admin.User = *adminUser
	}
	if adminPassword != nil {
		cm.config.Auth.AdminPassword = *adminPassword
		cm.config.Admin.Password = *adminPassword
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateLogging updates logging configuration (applies immediately)
func (cm *ConfigManager) UpdateLogging(logLevel, accessLog, errorLog *string, logSize *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if logLevel != nil {
		cm.config.Logging.LogLevel = *logLevel
	}
	if accessLog != nil {
		cm.config.Logging.AccessLog = *accessLog
	}
	if errorLog != nil {
		cm.config.Logging.ErrorLog = *errorLog
	}
	if logSize != nil {
		cm.config.Logging.LogSize = *logSize
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// UpdateDirectory updates directory/YP configuration
func (cm *ConfigManager) UpdateDirectory(enabled *bool, ypURLs []string, interval *int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if enabled != nil {
		cm.config.Directory.Enabled = *enabled
	}
	if ypURLs != nil {
		cm.config.Directory.YPURLs = ypURLs
	}
	if interval != nil {
		cm.config.Directory.IntervalSeconds = *interval
		cm.config.Directory.Interval = time.Duration(*interval) * time.Second
	}

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// ----- Mount Management -----

// CreateMount creates a new mount configuration
func (cm *ConfigManager) CreateMount(path string, mount *MountConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Ensure path starts with /
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}

	// Check if mount already exists
	if _, exists := cm.config.Mounts[path]; exists {
		return fmt.Errorf("mount %s already exists", path)
	}

	mount.Name = path
	cm.config.Mounts[path] = mount

	if err := cm.saveUnlocked(); err != nil {
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

	mount.Name = path
	cm.config.Mounts[path] = mount

	if err := cm.saveUnlocked(); err != nil {
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

	delete(cm.config.Mounts, path)

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// GetMount returns a specific mount configuration
func (cm *ConfigManager) GetMount(path string) *MountConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if mount, exists := cm.config.Mounts[path]; exists {
		return mount
	}
	return nil
}

// GetAllMounts returns all mount configurations
func (cm *ConfigManager) GetAllMounts() map[string]*MountConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]*MountConfig)
	for path, mount := range cm.config.Mounts {
		// Create a copy
		mountCopy := *mount
		result[path] = &mountCopy
	}
	return result
}

// ----- Other Methods -----

// ResetToDefaults resets configuration to defaults (preserving credentials)
func (cm *ConfigManager) ResetToDefaults() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Save current auth
	oldAuth := cm.config.Auth
	oldAdmin := cm.config.Admin

	// Reset to defaults
	cm.config = DefaultConfig()

	// Restore auth
	cm.config.Auth = oldAuth
	cm.config.Admin = oldAdmin
	cm.config.SetupComplete = true

	if err := cm.saveUnlocked(); err != nil {
		return err
	}

	cm.notifyChange()
	return nil
}

// ExportConfig exports the current config as JSON
func (cm *ConfigManager) ExportConfig() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return json.MarshalIndent(cm.config, "", "  ")
}

// HasStateOverrides returns true (for compatibility)
func (cm *ConfigManager) HasStateOverrides() bool {
	return true // Always using JSON config
}

// ----- Helper Functions -----

// generateSecurePassword generates a secure random password
func generateSecurePassword(length int) string {
	const charset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	randomBytes := make([]byte, length)
	rand.Read(randomBytes)
	for i := 0; i < length; i++ {
		result[i] = charset[int(randomBytes[i])%len(charset)]
	}
	return string(result)
}

// generateSecureToken generates a secure random hex token
func generateSecureToken(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
