// Package server provides HTTP server functionality for GoCast
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// AutoSSLManager handles automatic SSL certificate management via Let's Encrypt
type AutoSSLManager struct {
	manager  *autocert.Manager
	hostname string
	email    string
	cacheDir string
	logger   *log.Logger
}

// NewAutoSSLManager creates a new AutoSSL manager
func NewAutoSSLManager(hostname, email, cacheDir string, logger *log.Logger) (*AutoSSLManager, error) {
	if hostname == "" || hostname == "localhost" {
		return nil, fmt.Errorf("AutoSSL requires a valid public hostname")
	}

	if logger == nil {
		logger = log.Default()
	}

	// Ensure cache directory exists
	if cacheDir == "" {
		cacheDir = "/var/lib/gocast/certs"
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create certificate cache directory: %w", err)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hostname),
		Cache:      autocert.DirCache(cacheDir),
		Email:      email,
	}

	return &AutoSSLManager{
		manager:  m,
		hostname: hostname,
		email:    email,
		cacheDir: cacheDir,
		logger:   logger,
	}, nil
}

// TLSConfig returns a TLS configuration that automatically handles certificates
func (a *AutoSSLManager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: a.manager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1", "acme-tls/1"},
	}
}

// HTTPHandler returns an HTTP handler for ACME HTTP-01 challenges
// This should be used on port 80 to handle Let's Encrypt verification
func (a *AutoSSLManager) HTTPHandler(fallback http.Handler) http.Handler {
	return a.manager.HTTPHandler(fallback)
}

// GetCertificate manually retrieves or renews a certificate
func (a *AutoSSLManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return a.manager.GetCertificate(hello)
}

// CacheDir returns the certificate cache directory
func (a *AutoSSLManager) CacheDir() string {
	return a.cacheDir
}

// Hostname returns the configured hostname
func (a *AutoSSLManager) Hostname() string {
	return a.hostname
}

// CertificateExists checks if a certificate already exists in the cache
func (a *AutoSSLManager) CertificateExists() bool {
	certFile := filepath.Join(a.cacheDir, a.hostname)
	_, err := os.Stat(certFile)
	return err == nil
}

// PreloadCertificate attempts to obtain a certificate before starting the server
func (a *AutoSSLManager) PreloadCertificate(ctx context.Context) error {
	a.logger.Printf("Obtaining SSL certificate for %s...", a.hostname)

	// Create a dummy ClientHelloInfo to trigger certificate fetch
	hello := &tls.ClientHelloInfo{
		ServerName: a.hostname,
	}

	cert, err := a.manager.GetCertificate(hello)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	if cert != nil {
		a.logger.Printf("SSL certificate obtained successfully for %s", a.hostname)
	}

	return nil
}

// RedirectHTTPToHTTPS creates a handler that redirects all HTTP traffic to HTTPS
// while still handling ACME challenges
func (a *AutoSSLManager) RedirectHTTPToHTTPS(httpsPort int) http.Handler {
	redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := fmt.Sprintf("https://%s", r.Host)
		if httpsPort != 443 {
			target = fmt.Sprintf("https://%s:%d", r.Host, httpsPort)
		}
		target += r.URL.RequestURI()

		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})

	// Wrap with ACME handler to handle challenges on port 80
	return a.HTTPHandler(redirect)
}

// StartHTTPChallengeServer starts an HTTP server on port 80 to handle ACME challenges
// and optionally redirect other traffic to HTTPS
func (a *AutoSSLManager) StartHTTPChallengeServer(httpsPort int) *http.Server {
	server := &http.Server{
		Addr:    ":80",
		Handler: a.RedirectHTTPToHTTPS(httpsPort),
	}

	go func() {
		a.logger.Println("Starting HTTP challenge server on :80 (redirects to HTTPS)")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Printf("HTTP challenge server error: %v", err)
		}
	}()

	return server
}
