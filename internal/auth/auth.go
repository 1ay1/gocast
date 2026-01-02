// Package auth provides authentication utilities for GoCast
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gocast/gocast/internal/config"
)

// Authenticator handles authentication for source and admin connections
type Authenticator struct {
	config       *config.Config
	failedLogins map[string]*loginAttempt
	mu           sync.RWMutex
	maxAttempts  int
	lockoutTime  time.Duration
}

// loginAttempt tracks failed login attempts
type loginAttempt struct {
	Count     int
	LastTry   time.Time
	LockedOut bool
}

// CredentialType represents the type of credentials
type CredentialType int

const (
	CredentialSource CredentialType = iota
	CredentialRelay
	CredentialAdmin
)

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.Config) *Authenticator {
	return &Authenticator{
		config:       cfg,
		failedLogins: make(map[string]*loginAttempt),
		maxAttempts:  5,
		lockoutTime:  5 * time.Minute,
	}
}

// SetConfig updates the authenticator's configuration (for hot-reload support)
func (a *Authenticator) SetConfig(cfg *config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = cfg
}

// getConfig returns the current config with proper locking
func (a *Authenticator) getConfig() *config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// Authenticate checks credentials from an HTTP request
func (a *Authenticator) Authenticate(r *http.Request, credType CredentialType) bool {
	// Check for lockout
	clientIP := getClientIP(r)
	if a.isLockedOut(clientIP) {
		return false
	}

	// Try Basic auth first
	username, password, ok := r.BasicAuth()
	if !ok {
		// Try ICY-style headers
		username = r.Header.Get("ice-username")
		password = r.Header.Get("ice-password")
		if password == "" {
			// Try Authorization header manually
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Basic ") {
				decoded, err := base64.StdEncoding.DecodeString(auth[6:])
				if err == nil {
					parts := strings.SplitN(string(decoded), ":", 2)
					if len(parts) == 2 {
						username = parts[0]
						password = parts[1]
					}
				}
			}
		}
	}

	// Validate credentials
	if a.validateCredentials(username, password, credType, r.URL.Path) {
		a.clearFailedAttempts(clientIP)
		return true
	}

	a.recordFailedAttempt(clientIP)
	return false
}

// validateCredentials checks username and password against configuration
func (a *Authenticator) validateCredentials(username, password string, credType CredentialType, mountPath string) bool {
	switch credType {
	case CredentialSource:
		return a.validateSourceCredentials(username, password, mountPath)
	case CredentialRelay:
		return a.validateRelayCredentials(username, password)
	case CredentialAdmin:
		return a.validateAdminCredentials(username, password)
	default:
		return false
	}
}

// validateSourceCredentials validates source client credentials
func (a *Authenticator) validateSourceCredentials(username, password, mountPath string) bool {
	cfg := a.getConfig()

	// Check mount-specific password first
	if mount, exists := cfg.Mounts[mountPath]; exists {
		if mount.Password != "" {
			if secureCompare(password, mount.Password) {
				return true
			}
		}
	}

	// Username can be empty or "source" for Icecast compatibility
	if username != "" && username != "source" {
		// Check if admin credentials are being used for source
		if secureCompare(username, cfg.Auth.AdminUser) &&
			secureCompare(password, cfg.Auth.AdminPassword) {
			return true
		}
		return false
	}

	// Check global source password
	return secureCompare(password, cfg.Auth.SourcePassword)
}

// validateRelayCredentials validates relay connection credentials
func (a *Authenticator) validateRelayCredentials(username, password string) bool {
	cfg := a.getConfig()
	if cfg.Auth.RelayPassword == "" {
		return false
	}
	return secureCompare(password, cfg.Auth.RelayPassword)
}

// validateAdminCredentials validates admin interface credentials
func (a *Authenticator) validateAdminCredentials(username, password string) bool {
	cfg := a.getConfig()
	return secureCompare(username, cfg.Auth.AdminUser) &&
		secureCompare(password, cfg.Auth.AdminPassword)
}

// isLockedOut checks if an IP is locked out due to failed attempts
func (a *Authenticator) isLockedOut(ip string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	attempt, exists := a.failedLogins[ip]
	if !exists {
		return false
	}

	if attempt.LockedOut {
		// Check if lockout has expired
		if time.Since(attempt.LastTry) > a.lockoutTime {
			return false
		}
		return true
	}

	return false
}

// recordFailedAttempt records a failed login attempt
func (a *Authenticator) recordFailedAttempt(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	attempt, exists := a.failedLogins[ip]
	if !exists {
		attempt = &loginAttempt{}
		a.failedLogins[ip] = attempt
	}

	// Reset count if last attempt was long ago
	if time.Since(attempt.LastTry) > a.lockoutTime {
		attempt.Count = 0
		attempt.LockedOut = false
	}

	attempt.Count++
	attempt.LastTry = time.Now()

	if attempt.Count >= a.maxAttempts {
		attempt.LockedOut = true
	}
}

// clearFailedAttempts clears failed login attempts for an IP
func (a *Authenticator) clearFailedAttempts(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.failedLogins, ip)
}

// CleanupExpired removes expired lockout entries
func (a *Authenticator) CleanupExpired() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for ip, attempt := range a.failedLogins {
		if time.Since(attempt.LastTry) > a.lockoutTime*2 {
			delete(a.failedLogins, ip)
		}
	}
}

// StartCleanup starts a goroutine to periodically clean up expired entries
func (a *Authenticator) StartCleanup(done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(a.lockoutTime)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.CleanupExpired()
			}
		}
	}()
}

// RequireAuth is an HTTP middleware that requires authentication
func (a *Authenticator) RequireAuth(credType CredentialType, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Authenticate(r, credType) {
			realm := "GoCast"
			switch credType {
			case CredentialSource:
				realm = "GoCast Source"
			case CredentialAdmin:
				realm = "GoCast Admin"
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuthFunc is a function wrapper for RequireAuth
func (a *Authenticator) RequireAuthFunc(credType CredentialType, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.Authenticate(r, credType) {
			realm := "GoCast"
			switch credType {
			case CredentialSource:
				realm = "GoCast Source"
			case CredentialAdmin:
				realm = "GoCast Admin"
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

// secureCompare performs a constant-time string comparison
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// HashPassword generates a simple hash for a password (for display masking)
func HashPassword(password string) string {
	if len(password) <= 2 {
		return "***"
	}
	return password[:1] + strings.Repeat("*", len(password)-2) + password[len(password)-1:]
}

// ValidatePasswordStrength checks if a password meets minimum requirements
func ValidatePasswordStrength(password string) bool {
	// Minimum 6 characters
	if len(password) < 6 {
		return false
	}

	// Check for variety
	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}

	// Require at least 2 of 3 character types
	count := 0
	if hasUpper {
		count++
	}
	if hasLower {
		count++
	}
	if hasDigit {
		count++
	}

	return count >= 2
}
