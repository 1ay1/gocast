// Package config handles GoCast configuration loading and management
package config

import (
	"fmt"
	"time"

	"github.com/gocast/gocast/pkg/vibe"
)

// Config represents the complete GoCast server configuration
type Config struct {
	Server    ServerConfig
	Limits    LimitsConfig
	Auth      AuthConfig
	Logging   LoggingConfig
	Mounts    map[string]*MountConfig
	Admin     AdminConfig
	Directory DirectoryConfig
}

// ServerConfig contains server-level settings
type ServerConfig struct {
	Hostname      string
	ListenAddress string
	Port          int
	SSLPort       int
	SSLEnabled    bool
	SSLCert       string
	SSLKey        string
	AdminRoot     string
	Location      string
	ServerID      string
}

// LimitsConfig contains resource limits
type LimitsConfig struct {
	MaxClients           int
	MaxSources           int
	MaxListenersPerMount int
	QueueSize            int
	ClientTimeout        time.Duration
	HeaderTimeout        time.Duration
	SourceTimeout        time.Duration
	BurstSize            int
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	SourcePassword string
	RelayPassword  string
	AdminUser      string
	AdminPassword  string
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	AccessLog string
	ErrorLog  string
	LogLevel  string
	LogSize   int
}

// MountConfig contains per-mount settings
type MountConfig struct {
	Name                string
	Password            string
	MaxListeners        int
	FallbackMount       string
	Genre               string
	Description         string
	URL                 string
	Bitrate             int
	Type                string
	Public              bool
	StreamName          string
	Hidden              bool
	BurstSize           int
	AllowedIPs          []string
	DeniedIPs           []string
	DumpFile            string
	MaxListenerDuration time.Duration
}

// AdminConfig contains admin interface settings
type AdminConfig struct {
	Enabled  bool
	User     string
	Password string
}

// DirectoryConfig contains directory/YP settings
type DirectoryConfig struct {
	Enabled  bool
	YPURLs   []string
	Interval time.Duration
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Hostname:      "localhost",
			ListenAddress: "0.0.0.0",
			Port:          8000,
			SSLPort:       8443,
			SSLEnabled:    false,
			AdminRoot:     "/admin",
			Location:      "Earth",
			ServerID:      "GoCast",
		},
		Limits: LimitsConfig{
			MaxClients:           100,
			MaxSources:           10,
			MaxListenersPerMount: 100,
			QueueSize:            262144, // 256KB (reduced for lower latency)
			ClientTimeout:        30 * time.Second,
			HeaderTimeout:        15 * time.Second,
			SourceTimeout:        10 * time.Second,
			BurstSize:            16384, // 16KB (reduced for faster start)
		},
		Auth: AuthConfig{
			SourcePassword: "hackme",
			RelayPassword:  "",
			AdminUser:      "admin",
			AdminPassword:  "hackme",
		},
		Logging: LoggingConfig{
			AccessLog: "/var/log/gocast/access.log",
			ErrorLog:  "/var/log/gocast/error.log",
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
			Enabled:  false,
			YPURLs:   []string{},
			Interval: 10 * time.Minute,
		},
	}
}

// Load loads configuration from a VIBE file
func Load(filename string) (*Config, error) {
	v, err := vibe.ParseFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg := DefaultConfig()

	// Server configuration
	if server := v.GetObject("server"); server != nil {
		cfg.Server.Hostname = v.GetStringDefault("server.hostname", cfg.Server.Hostname)
		cfg.Server.ListenAddress = v.GetStringDefault("server.listen", cfg.Server.ListenAddress)
		cfg.Server.Port = int(v.GetIntDefault("server.port", int64(cfg.Server.Port)))
		cfg.Server.SSLPort = int(v.GetIntDefault("server.ssl_port", int64(cfg.Server.SSLPort)))
		cfg.Server.SSLEnabled = v.GetBoolDefault("server.ssl.enabled", cfg.Server.SSLEnabled)
		cfg.Server.SSLCert = v.GetStringDefault("server.ssl.cert", cfg.Server.SSLCert)
		cfg.Server.SSLKey = v.GetStringDefault("server.ssl.key", cfg.Server.SSLKey)
		cfg.Server.AdminRoot = v.GetStringDefault("server.admin_root", cfg.Server.AdminRoot)
		cfg.Server.Location = v.GetStringDefault("server.location", cfg.Server.Location)
		cfg.Server.ServerID = v.GetStringDefault("server.server_id", cfg.Server.ServerID)
	}

	// Limits configuration
	if limits := v.GetObject("limits"); limits != nil {
		cfg.Limits.MaxClients = int(v.GetIntDefault("limits.max_clients", int64(cfg.Limits.MaxClients)))
		cfg.Limits.MaxSources = int(v.GetIntDefault("limits.max_sources", int64(cfg.Limits.MaxSources)))
		cfg.Limits.MaxListenersPerMount = int(v.GetIntDefault("limits.max_listeners_per_mount", int64(cfg.Limits.MaxListenersPerMount)))
		cfg.Limits.QueueSize = int(v.GetIntDefault("limits.queue_size", int64(cfg.Limits.QueueSize)))
		cfg.Limits.BurstSize = int(v.GetIntDefault("limits.burst_size", int64(cfg.Limits.BurstSize)))

		if timeout := v.GetInt("limits.client_timeout"); timeout > 0 {
			cfg.Limits.ClientTimeout = time.Duration(timeout) * time.Second
		}
		if timeout := v.GetInt("limits.header_timeout"); timeout > 0 {
			cfg.Limits.HeaderTimeout = time.Duration(timeout) * time.Second
		}
		if timeout := v.GetInt("limits.source_timeout"); timeout > 0 {
			cfg.Limits.SourceTimeout = time.Duration(timeout) * time.Second
		}
	}

	// Auth configuration
	if auth := v.GetObject("auth"); auth != nil {
		cfg.Auth.SourcePassword = v.GetStringDefault("auth.source_password", cfg.Auth.SourcePassword)
		cfg.Auth.RelayPassword = v.GetStringDefault("auth.relay_password", cfg.Auth.RelayPassword)
		cfg.Auth.AdminUser = v.GetStringDefault("auth.admin_user", cfg.Auth.AdminUser)
		cfg.Auth.AdminPassword = v.GetStringDefault("auth.admin_password", cfg.Auth.AdminPassword)
	}

	// Logging configuration
	if logging := v.GetObject("logging"); logging != nil {
		cfg.Logging.AccessLog = v.GetStringDefault("logging.access_log", cfg.Logging.AccessLog)
		cfg.Logging.ErrorLog = v.GetStringDefault("logging.error_log", cfg.Logging.ErrorLog)
		cfg.Logging.LogLevel = v.GetStringDefault("logging.level", cfg.Logging.LogLevel)
		cfg.Logging.LogSize = int(v.GetIntDefault("logging.log_size", int64(cfg.Logging.LogSize)))
	}

	// Mount configurations
	if mounts := v.GetObject("mounts"); mounts != nil {
		for _, key := range mounts.Keys {
			mountPath := "mounts." + key
			mountValue := v.GetObject(mountPath)
			if mountValue == nil {
				continue
			}

			mountName := "/" + key
			if key[0] == '/' {
				mountName = key
			}

			mount := &MountConfig{
				Name:          mountName,
				Password:      v.GetStringDefault(mountPath+".password", cfg.Auth.SourcePassword),
				MaxListeners:  int(v.GetIntDefault(mountPath+".max_listeners", int64(cfg.Limits.MaxListenersPerMount))),
				FallbackMount: v.GetStringDefault(mountPath+".fallback", ""),
				Genre:         v.GetStringDefault(mountPath+".genre", ""),
				Description:   v.GetStringDefault(mountPath+".description", ""),
				URL:           v.GetStringDefault(mountPath+".url", ""),
				Bitrate:       int(v.GetIntDefault(mountPath+".bitrate", 128)),
				Type:          v.GetStringDefault(mountPath+".type", "audio/mpeg"),
				Public:        v.GetBoolDefault(mountPath+".public", true),
				StreamName:    v.GetStringDefault(mountPath+".stream_name", key),
				Hidden:        v.GetBoolDefault(mountPath+".hidden", false),
				BurstSize:     int(v.GetIntDefault(mountPath+".burst_size", int64(cfg.Limits.BurstSize))),
				AllowedIPs:    v.GetStringArray(mountPath + ".allowed_ips"),
				DeniedIPs:     v.GetStringArray(mountPath + ".denied_ips"),
				DumpFile:      v.GetStringDefault(mountPath+".dump_file", ""),
			}

			if duration := v.GetInt(mountPath + ".max_listener_duration"); duration > 0 {
				mount.MaxListenerDuration = time.Duration(duration) * time.Second
			}

			cfg.Mounts[mountName] = mount
		}
	}

	// Admin configuration
	if admin := v.GetObject("admin"); admin != nil {
		cfg.Admin.Enabled = v.GetBoolDefault("admin.enabled", cfg.Admin.Enabled)
		cfg.Admin.User = v.GetStringDefault("admin.user", cfg.Admin.User)
		cfg.Admin.Password = v.GetStringDefault("admin.password", cfg.Admin.Password)
	}

	// Directory/YP configuration
	if directory := v.GetObject("directory"); directory != nil {
		cfg.Directory.Enabled = v.GetBoolDefault("directory.enabled", cfg.Directory.Enabled)
		cfg.Directory.YPURLs = v.GetStringArray("directory.yp_urls")
		if interval := v.GetInt("directory.interval"); interval > 0 {
			cfg.Directory.Interval = time.Duration(interval) * time.Second
		}
	}

	return cfg, nil
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
		BurstSize:    c.Limits.BurstSize,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Server.SSLEnabled {
		if c.Server.SSLCert == "" {
			return fmt.Errorf("SSL enabled but no certificate path specified")
		}
		if c.Server.SSLKey == "" {
			return fmt.Errorf("SSL enabled but no key path specified")
		}
	}

	if c.Limits.MaxClients <= 0 {
		return fmt.Errorf("max_clients must be positive")
	}

	if c.Limits.MaxSources <= 0 {
		return fmt.Errorf("max_sources must be positive")
	}

	return nil
}
