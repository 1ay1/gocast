// Package config handles GoCast configuration loading and management.
//
// GoCast uses a single JSON configuration file that can be:
// - Edited via the Admin Panel (recommended)
// - Edited manually and hot-reloaded
//
// Default location: ~/.gocast/config.json
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config represents the complete GoCast server configuration
type Config struct {
	// Version for config file format migrations
	Version int `json:"version"`

	// LastModified timestamp
	LastModified time.Time `json:"last_modified"`

	// Setup state
	SetupComplete bool `json:"setup_complete"`

	// Server configuration
	Server ServerConfig `json:"server"`

	// Resource limits
	Limits LimitsConfig `json:"limits"`

	// Authentication settings
	Auth AuthConfig `json:"auth"`

	// Logging settings
	Logging LoggingConfig `json:"logging"`

	// Mount point configurations
	Mounts map[string]*MountConfig `json:"mounts"`

	// Admin interface settings
	Admin AdminConfig `json:"admin"`

	// Directory/YP settings
	Directory DirectoryConfig `json:"directory"`

	// SSL/TLS settings
	SSL SSLConfig `json:"ssl"`
}

// ServerConfig contains server-level settings
type ServerConfig struct {
	Hostname      string `json:"hostname"`
	ListenAddress string `json:"listen_address"`
	Port          int    `json:"port"`
	AdminRoot     string `json:"admin_root"`
	Location      string `json:"location"`
	ServerID      string `json:"server_id"`
}

// SSLConfig contains SSL/TLS settings
type SSLConfig struct {
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	AutoSSL      bool   `json:"auto_ssl"`
	AutoSSLEmail string `json:"auto_ssl_email,omitempty"`
	CertPath     string `json:"cert_path,omitempty"`
	KeyPath      string `json:"key_path,omitempty"`
	CacheDir     string `json:"cache_dir,omitempty"`
}

// LimitsConfig contains resource limits
type LimitsConfig struct {
	MaxClients           int           `json:"max_clients"`
	MaxSources           int           `json:"max_sources"`
	MaxListenersPerMount int           `json:"max_listeners_per_mount"`
	QueueSize            int           `json:"queue_size"`
	BurstSize            int           `json:"burst_size"`
	ClientTimeout        time.Duration `json:"-"`
	ClientTimeoutSeconds int           `json:"client_timeout"`
	HeaderTimeout        time.Duration `json:"-"`
	HeaderTimeoutSeconds int           `json:"header_timeout"`
	SourceTimeout        time.Duration `json:"-"`
	SourceTimeoutSeconds int           `json:"source_timeout"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	SourcePassword string `json:"source_password"`
	RelayPassword  string `json:"relay_password,omitempty"`
	AdminUser      string `json:"admin_user"`
	AdminPassword  string `json:"admin_password"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	AccessLog string `json:"access_log"`
	ErrorLog  string `json:"error_log"`
	LogLevel  string `json:"log_level"`
	LogSize   int    `json:"log_size"`
}

// MountConfig contains per-mount settings
type MountConfig struct {
	Name                string        `json:"name"`
	Password            string        `json:"password,omitempty"`
	MaxListeners        int           `json:"max_listeners"`
	FallbackMount       string        `json:"fallback_mount,omitempty"`
	Genre               string        `json:"genre,omitempty"`
	Description         string        `json:"description,omitempty"`
	URL                 string        `json:"url,omitempty"`
	Bitrate             int           `json:"bitrate"`
	Type                string        `json:"type"`
	Public              bool          `json:"public"`
	StreamName          string        `json:"stream_name,omitempty"`
	Hidden              bool          `json:"hidden,omitempty"`
	BurstSize           int           `json:"burst_size,omitempty"`
	AllowedIPs          []string      `json:"allowed_ips,omitempty"`
	DeniedIPs           []string      `json:"denied_ips,omitempty"`
	DumpFile            string        `json:"dump_file,omitempty"`
	MaxListenerDuration time.Duration `json:"-"`
	MaxListenerSeconds  int           `json:"max_listener_duration,omitempty"`
}

// AdminConfig contains admin interface settings
type AdminConfig struct {
	Enabled  bool   `json:"enabled"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// DirectoryConfig contains directory/YP settings
type DirectoryConfig struct {
	Enabled         bool          `json:"enabled"`
	YPURLs          []string      `json:"yp_urls,omitempty"`
	Interval        time.Duration `json:"-"`
	IntervalSeconds int           `json:"interval"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:       1,
		LastModified:  time.Now(),
		SetupComplete: false,
		Server: ServerConfig{
			Hostname:      "localhost",
			ListenAddress: "0.0.0.0",
			Port:          8000,
			AdminRoot:     "/admin",
			Location:      "Earth",
			ServerID:      "GoCast",
		},
		SSL: SSLConfig{
			Enabled:      false,
			Port:         8443,
			AutoSSL:      false,
			AutoSSLEmail: "",
			CacheDir:     "",
		},
		Limits: LimitsConfig{
			MaxClients:           100,
			MaxSources:           10,
			MaxListenersPerMount: 100,
			QueueSize:            131072, // 128KB
			BurstSize:            2048,   // 2KB
			ClientTimeout:        30 * time.Second,
			ClientTimeoutSeconds: 30,
			HeaderTimeout:        5 * time.Second,
			HeaderTimeoutSeconds: 5,
			SourceTimeout:        5 * time.Second,
			SourceTimeoutSeconds: 5,
		},
		Auth: AuthConfig{
			SourcePassword: "hackme",
			RelayPassword:  "",
			AdminUser:      "admin",
			AdminPassword:  "hackme",
		},
		Logging: LoggingConfig{
			AccessLog: "",
			ErrorLog:  "",
			LogLevel:  "info",
			LogSize:   10000,
		},
		Mounts: make(map[string]*MountConfig),
		Admin: AdminConfig{
			Enabled:  true,
			User:     "admin",
			Password: "hackme",
		},
		Directory: DirectoryConfig{
			Enabled:         false,
			YPURLs:          []string{},
			Interval:        10 * time.Minute,
			IntervalSeconds: 600,
		},
	}
}

// Load loads configuration from a JSON file
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Convert seconds to durations
	cfg.normalizeDurations()

	return cfg, nil
}

// Save saves the configuration to a JSON file
func (c *Config) Save(filename string) error {
	c.LastModified = time.Now()

	// Convert durations to seconds for JSON
	c.normalizeSeconds()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Atomic write: temp file then rename
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := os.Rename(tempFile, filename); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to save config file: %w", err)
	}

	return nil
}

// normalizeDurations converts seconds fields to time.Duration
func (c *Config) normalizeDurations() {
	if c.Limits.ClientTimeoutSeconds > 0 {
		c.Limits.ClientTimeout = time.Duration(c.Limits.ClientTimeoutSeconds) * time.Second
	}
	if c.Limits.HeaderTimeoutSeconds > 0 {
		c.Limits.HeaderTimeout = time.Duration(c.Limits.HeaderTimeoutSeconds) * time.Second
	}
	if c.Limits.SourceTimeoutSeconds > 0 {
		c.Limits.SourceTimeout = time.Duration(c.Limits.SourceTimeoutSeconds) * time.Second
	}
	if c.Directory.IntervalSeconds > 0 {
		c.Directory.Interval = time.Duration(c.Directory.IntervalSeconds) * time.Second
	}

	// Normalize mount durations
	for _, m := range c.Mounts {
		if m.MaxListenerSeconds > 0 {
			m.MaxListenerDuration = time.Duration(m.MaxListenerSeconds) * time.Second
		}
	}
}

// normalizeSeconds converts time.Duration to seconds for JSON storage
func (c *Config) normalizeSeconds() {
	c.Limits.ClientTimeoutSeconds = int(c.Limits.ClientTimeout.Seconds())
	c.Limits.HeaderTimeoutSeconds = int(c.Limits.HeaderTimeout.Seconds())
	c.Limits.SourceTimeoutSeconds = int(c.Limits.SourceTimeout.Seconds())
	c.Directory.IntervalSeconds = int(c.Directory.Interval.Seconds())

	for _, m := range c.Mounts {
		if m.MaxListenerDuration > 0 {
			m.MaxListenerSeconds = int(m.MaxListenerDuration.Seconds())
		}
	}
}

// GetMountConfig returns the configuration for a specific mount
// If no specific configuration exists, returns a default configuration
func (c *Config) GetMountConfig(mountPath string) *MountConfig {
	if mount, exists := c.Mounts[mountPath]; exists {
		return mount
	}

	// Return a default mount config
	return &MountConfig{
		Name:         mountPath,
		Password:     c.Auth.SourcePassword,
		MaxListeners: c.Limits.MaxListenersPerMount,
		Type:         "audio/mpeg",
		Public:       true,
		Bitrate:      128,
		BurstSize:    c.Limits.BurstSize,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.SSL.Enabled && !c.SSL.AutoSSL {
		if c.SSL.CertPath == "" {
			return fmt.Errorf("SSL enabled but no certificate path specified (use auto_ssl for automatic certificates)")
		}
		if c.SSL.KeyPath == "" {
			return fmt.Errorf("SSL enabled but no key path specified (use auto_ssl for automatic certificates)")
		}
	}

	if c.SSL.AutoSSL {
		if c.Server.Hostname == "" || c.Server.Hostname == "localhost" {
			return fmt.Errorf("auto_ssl requires a valid public hostname (not localhost)")
		}
	}

	if c.SSL.Port <= 0 || c.SSL.Port > 65535 {
		return fmt.Errorf("invalid SSL port: %d", c.SSL.Port)
	}

	if c.Limits.MaxClients <= 0 {
		return fmt.Errorf("max_clients must be positive")
	}

	if c.Limits.MaxSources <= 0 {
		return fmt.Errorf("max_sources must be positive")
	}

	return nil
}

// Clone creates a deep copy of the config
func (c *Config) Clone() *Config {
	// Marshal and unmarshal for deep copy
	data, _ := json.Marshal(c)
	clone := &Config{}
	json.Unmarshal(data, clone)
	clone.normalizeDurations()
	return clone
}
