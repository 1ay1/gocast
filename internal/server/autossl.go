// Package server provides HTTP server functionality for GoCast
package server

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

// DNSProvider represents a DNS provider for automatic DNS-01 challenges
type DNSProvider string

const (
	DNSProviderManual     DNSProvider = "manual"
	DNSProviderCloudflare DNSProvider = "cloudflare"
)

// ChallengeStatus represents the current state of the SSL certificate process
type ChallengeStatus string

const (
	StatusNone           ChallengeStatus = "none"
	StatusReady          ChallengeStatus = "ready"           // Ready to start, no action taken yet
	StatusDNSPending     ChallengeStatus = "dns_pending"     // Waiting for user to add DNS record
	StatusDNSVerifying   ChallengeStatus = "dns_verifying"   // Checking if DNS has propagated
	StatusDNSVerified    ChallengeStatus = "dns_verified"    // DNS record confirmed, ready to get cert
	StatusObtaining      ChallengeStatus = "obtaining"       // Getting certificate from Let's Encrypt
	StatusComplete       ChallengeStatus = "complete"        // Certificate obtained successfully
	StatusHasCertificate ChallengeStatus = "has_certificate" // Already has valid certificate
	StatusError          ChallengeStatus = "error"           // Something went wrong
)

// AutoSSLConfig contains configuration for AutoSSL
type AutoSSLConfig struct {
	Hostname         string
	Email            string
	CacheDir         string
	DNSProvider      DNSProvider
	CloudflareToken  string
	CloudflareZoneID string
}

// AutoSSLStatus represents the current status for the admin panel
type AutoSSLStatus struct {
	Status          ChallengeStatus `json:"status"`
	Message         string          `json:"message"`
	FQDN            string          `json:"fqdn,omitempty"`
	TXTValue        string          `json:"txt_value,omitempty"`
	DNSVerified     bool            `json:"dns_verified"`
	CertificateInfo *CertInfo       `json:"certificate,omitempty"`
	Error           string          `json:"error,omitempty"`
	NextStep        string          `json:"next_step,omitempty"`
}

// CertInfo contains certificate information for display
type CertInfo struct {
	Domain    string `json:"domain"`
	NotBefore string `json:"not_before"`
	NotAfter  string `json:"not_after"`
	DaysLeft  int    `json:"days_left"`
}

// AutoSSLManager handles automatic SSL certificate management via Let's Encrypt DNS-01
type AutoSSLManager struct {
	config   AutoSSLConfig
	client   *acme.Client
	account  *acme.Account
	logger   *log.Logger
	cacheDir string

	// Current state
	status       ChallengeStatus
	statusMsg    string
	lastError    string
	pendingFQDN  string
	pendingValue string
	dnsVerified  bool
	statusMu     sync.RWMutex

	// Cached certificate
	cert   *tls.Certificate
	certMu sync.RWMutex

	// For tracking ongoing operations
	cancelFunc context.CancelFunc
}

// NewAutoSSLManager creates a new AutoSSL manager with DNS-01 challenge support
func NewAutoSSLManager(cfg AutoSSLConfig, logger *log.Logger) (*AutoSSLManager, error) {
	if cfg.Hostname == "" || cfg.Hostname == "localhost" {
		return nil, fmt.Errorf("AutoSSL requires a valid public hostname (got %q)", cfg.Hostname)
	}

	if net.ParseIP(cfg.Hostname) != nil {
		return nil, fmt.Errorf("AutoSSL requires a domain name, not an IP address")
	}

	if logger == nil {
		logger = log.Default()
	}

	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			cacheDir = "/var/lib/gocast/certs"
		} else {
			cacheDir = filepath.Join(homeDir, ".gocast", "certs")
		}
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create certificate cache directory: %w", err)
	}

	if cfg.DNSProvider == "" {
		cfg.DNSProvider = DNSProviderManual
	}

	if cfg.DNSProvider == DNSProviderCloudflare && cfg.CloudflareToken == "" {
		return nil, fmt.Errorf("Cloudflare DNS provider requires CloudflareToken")
	}

	mgr := &AutoSSLManager{
		config:   cfg,
		logger:   logger,
		cacheDir: cacheDir,
		status:   StatusNone,
	}

	// Clear any stale pending challenges from previous runs
	// Each server start should begin fresh (unless we have a valid cert)
	mgr.clearPendingFiles()

	// Check if we already have a valid certificate
	if mgr.HasValidCertificate() {
		if err := mgr.LoadCachedCertificate(); err == nil {
			mgr.status = StatusHasCertificate
			mgr.statusMsg = "Valid certificate loaded"
		}
	} else {
		mgr.status = StatusReady
		mgr.statusMsg = "Ready to obtain certificate"
		mgr.logger.Printf("[AutoSSL] No valid certificate found - ready to obtain one via admin panel")
	}

	return mgr, nil
}

// TLSConfig returns a TLS configuration that uses the managed certificate
func (a *AutoSSLManager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: a.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}

// GetCertificate returns the current certificate for TLS connections
func (a *AutoSSLManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	a.certMu.RLock()
	cert := a.cert
	a.certMu.RUnlock()

	if cert == nil {
		return nil, fmt.Errorf("no certificate available")
	}

	return cert, nil
}

// GetStatus returns the current status for the admin panel
func (a *AutoSSLManager) GetStatus() AutoSSLStatus {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()

	status := AutoSSLStatus{
		Status:      a.status,
		Message:     a.statusMsg,
		FQDN:        a.pendingFQDN,
		TXTValue:    a.pendingValue,
		DNSVerified: a.dnsVerified,
		Error:       a.lastError,
	}

	// Set helpful next step message
	switch a.status {
	case StatusNone, StatusReady:
		status.NextStep = "Click 'Start' to begin the certificate process"
	case StatusDNSPending:
		status.NextStep = "Add the TXT record to your DNS, then click 'Verify DNS'"
	case StatusDNSVerifying:
		status.NextStep = "Checking DNS propagation..."
	case StatusDNSVerified:
		status.NextStep = "DNS verified! Click 'Get Certificate' to complete"
	case StatusObtaining:
		status.NextStep = "Obtaining certificate from Let's Encrypt..."
	case StatusComplete, StatusHasCertificate:
		status.NextStep = "Certificate is active. Restart server if needed."
	case StatusError:
		status.NextStep = "Fix the error and try again"
	}

	// Add certificate info if we have one
	if a.status == StatusHasCertificate || a.status == StatusComplete {
		if info, err := a.getCertInfo(); err == nil {
			status.CertificateInfo = info
		}
	}

	return status
}

func (a *AutoSSLManager) getCertInfo() (*CertInfo, error) {
	a.certMu.RLock()
	cert := a.cert
	a.certMu.RUnlock()

	if cert == nil || len(cert.Certificate) == 0 {
		return nil, errors.New("no certificate")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	daysLeft := int(time.Until(x509Cert.NotAfter).Hours() / 24)

	return &CertInfo{
		Domain:    x509Cert.Subject.CommonName,
		NotBefore: x509Cert.NotBefore.Format("2006-01-02"),
		NotAfter:  x509Cert.NotAfter.Format("2006-01-02"),
		DaysLeft:  daysLeft,
	}, nil
}

func (a *AutoSSLManager) setStatus(status ChallengeStatus, msg string) {
	a.statusMu.Lock()
	a.status = status
	a.statusMsg = msg
	a.lastError = ""
	a.statusMu.Unlock()
}

func (a *AutoSSLManager) setError(err string) {
	a.statusMu.Lock()
	a.status = StatusError
	a.lastError = err
	a.statusMsg = "Error occurred"
	a.statusMu.Unlock()
}

func (a *AutoSSLManager) setPendingDNS(fqdn, value string) {
	a.statusMu.Lock()
	a.status = StatusDNSPending
	a.statusMsg = "Add the DNS TXT record below"
	a.pendingFQDN = fqdn
	a.pendingValue = value
	a.dnsVerified = false
	a.statusMu.Unlock()
}

func (a *AutoSSLManager) setDNSVerified() {
	a.statusMu.Lock()
	a.dnsVerified = true
	a.status = StatusDNSVerified
	a.statusMsg = "DNS record verified! Ready to obtain certificate."
	a.statusMu.Unlock()
}

func (a *AutoSSLManager) clearPendingChallenge() {
	a.statusMu.Lock()
	a.pendingFQDN = ""
	a.pendingValue = ""
	a.dnsVerified = false
	a.statusMu.Unlock()
}

// clearPendingFiles removes stale challenge files from disk
func (a *AutoSSLManager) clearPendingFiles() {
	os.Remove(filepath.Join(a.cacheDir, "pending_challenge"))
	os.Remove(filepath.Join(a.cacheDir, "pending_order"))
	os.Remove(filepath.Join(a.cacheDir, "pending_token"))

	// Also reset ACME client so we get fresh orders
	a.client = nil
	a.account = nil
}

// HasValidCertificate checks if we have a valid cached certificate
func (a *AutoSSLManager) HasValidCertificate() bool {
	certPath := filepath.Join(a.cacheDir, a.config.Hostname+".crt")
	keyPath := filepath.Join(a.cacheDir, a.config.Hostname+".key")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return false
	}

	if len(cert.Certificate) == 0 {
		return false
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return false
	}

	// Certificate should be valid for at least 7 more days
	return time.Now().Add(7 * 24 * time.Hour).Before(x509Cert.NotAfter)
}

// LoadCachedCertificate loads a previously obtained certificate from disk
func (a *AutoSSLManager) LoadCachedCertificate() error {
	certPath := filepath.Join(a.cacheDir, a.config.Hostname+".crt")
	keyPath := filepath.Join(a.cacheDir, a.config.Hostname+".key")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("failed to load cached certificate: %w", err)
	}

	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %w", err)
		}

		if time.Now().After(x509Cert.NotAfter) {
			return fmt.Errorf("cached certificate has expired")
		}

		a.logger.Printf("[AutoSSL] Loaded certificate (expires %s)", x509Cert.NotAfter.Format("2006-01-02"))
	}

	a.certMu.Lock()
	a.cert = &cert
	a.certMu.Unlock()

	a.setStatus(StatusHasCertificate, "Certificate loaded")

	return nil
}

// ============================================================================
// Step 1: Generate the DNS challenge value (but don't start ACME yet)
// ============================================================================

// PrepareDNSChallenge generates the TXT record value that needs to be added
// This creates a fresh ACME order each time to ensure we get a new challenge value
func (a *AutoSSLManager) PrepareDNSChallenge() error {
	// For Cloudflare, we'll handle everything automatically
	if a.config.DNSProvider == DNSProviderCloudflare {
		a.setStatus(StatusReady, "Ready to obtain certificate automatically via Cloudflare")
		return nil
	}

	// Clear any previous pending state and reset ACME client
	// This ensures we always get a FRESH challenge value
	a.clearPendingFiles()
	a.clearPendingChallenge()
	a.client = nil
	a.account = nil

	a.setStatus(StatusDNSVerifying, "Generating new DNS challenge...")

	// Initialize ACME client fresh
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.initACMEClient(ctx); err != nil {
		a.setError("Failed to initialize ACME client: " + err.Error())
		return fmt.Errorf("failed to initialize ACME client: %w", err)
	}

	// Create a NEW order to get a fresh challenge value
	a.logger.Printf("[AutoSSL] Creating new ACME order for %s", a.config.Hostname)
	order, err := a.client.AuthorizeOrder(ctx, acme.DomainIDs(a.config.Hostname))
	if err != nil {
		a.setError("Failed to create order: " + err.Error())
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Save order URI for later
	orderPath := filepath.Join(a.cacheDir, "pending_order")
	if err := os.WriteFile(orderPath, []byte(order.URI), 0600); err != nil {
		a.setError("Failed to save order: " + err.Error())
		return fmt.Errorf("failed to save order: %w", err)
	}

	// Find the DNS-01 challenge
	for _, authzURL := range order.AuthzURLs {
		authz, err := a.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			continue
		}

		for _, challenge := range authz.Challenges {
			if challenge.Type == "dns-01" {
				txtValue, err := a.client.DNS01ChallengeRecord(challenge.Token)
				if err != nil {
					return fmt.Errorf("failed to get challenge record: %w", err)
				}

				fqdn := "_acme-challenge." + a.config.Hostname

				// Save challenge info
				challengePath := filepath.Join(a.cacheDir, "pending_challenge")
				challengeData := fmt.Sprintf("%s\n%s\n%s", challenge.URI, fqdn, txtValue)
				if err := os.WriteFile(challengePath, []byte(challengeData), 0600); err != nil {
					return fmt.Errorf("failed to save challenge: %w", err)
				}

				a.setPendingDNS(fqdn, txtValue)
				a.logger.Printf("[AutoSSL] NEW DNS challenge generated: %s TXT %s", fqdn, txtValue)
				a.logger.Printf("[AutoSSL] Add this TXT record to your DNS, then verify via admin panel")
				return nil
			}
		}
	}

	return fmt.Errorf("no DNS-01 challenge available")
}

// ============================================================================
// Step 2: Verify DNS has propagated (user calls this after adding record)
// ============================================================================

// VerifyDNSRecord checks if the TXT record has propagated
// Returns nil if verified, error otherwise
func (a *AutoSSLManager) VerifyDNSRecord() error {
	a.statusMu.RLock()
	fqdn := a.pendingFQDN
	expectedValue := a.pendingValue
	a.statusMu.RUnlock()

	if fqdn == "" || expectedValue == "" {
		return errors.New("no pending DNS challenge - click 'Start' first")
	}

	a.setStatus(StatusDNSVerifying, "Checking DNS propagation...")

	// Check DNS and get what was found
	found, foundRecords := a.lookupDNSRecord(fqdn, expectedValue)

	if !found {
		a.setStatus(StatusDNSPending, "DNS record not found or incorrect value")

		// Build helpful error message
		var errMsg string
		if len(foundRecords) == 0 {
			errMsg = fmt.Sprintf("No TXT records found for %s.\n\nPlease add this TXT record to your DNS:\n\nName: %s\nValue: %s\n\nDNS propagation can take 1-5 minutes.", fqdn, fqdn, expectedValue)
		} else {
			errMsg = fmt.Sprintf("Wrong TXT record value found.\n\nFound %d record(s) for %s:\n", len(foundRecords), fqdn)
			for i, r := range foundRecords {
				errMsg += fmt.Sprintf("  %d. %s\n", i+1, r)
			}
			errMsg += fmt.Sprintf("\nExpected value:\n  %s\n\n", expectedValue)
			errMsg += "Please DELETE the old record(s) and add a new one with the correct value."
		}
		return errors.New(errMsg)
	}

	a.setDNSVerified()
	a.logger.Printf("[AutoSSL] DNS record verified for %s", fqdn)
	return nil
}

// checkDNSRecord checks if the expected TXT record exists
func (a *AutoSSLManager) checkDNSRecord(fqdn, expectedValue string) bool {
	found, _ := a.lookupDNSRecord(fqdn, expectedValue)
	return found
}

// lookupDNSRecord checks for the TXT record and returns what was found
func (a *AutoSSLManager) lookupDNSRecord(fqdn, expectedValue string) (found bool, foundRecords []string) {
	// Try multiple DNS lookups with slight delays
	for i := 0; i < 3; i++ {
		records, err := net.LookupTXT(fqdn)
		if err == nil {
			foundRecords = records
			for _, record := range records {
				if record == expectedValue {
					return true, records
				}
			}
		}
		if i < 2 {
			time.Sleep(time.Second)
		}
	}
	return false, foundRecords
}

// ============================================================================
// Step 3: Actually obtain the certificate (only after DNS is verified)
// ============================================================================

// ObtainCertificate completes the ACME flow and gets the certificate
// This should only be called after VerifyDNSRecord succeeds
func (a *AutoSSLManager) ObtainCertificate(ctx context.Context) error {
	a.statusMu.RLock()
	dnsVerified := a.dnsVerified
	fqdn := a.pendingFQDN
	value := a.pendingValue
	a.statusMu.RUnlock()

	// For Cloudflare, we do everything automatically
	if a.config.DNSProvider == DNSProviderCloudflare {
		return a.obtainWithCloudflare(ctx)
	}

	// For manual mode, DNS must be verified first
	if !dnsVerified {
		return errors.New("DNS record not verified. Please verify DNS first before obtaining certificate.")
	}

	if fqdn == "" || value == "" {
		return errors.New("no pending challenge. Please start the process first.")
	}

	a.setStatus(StatusObtaining, "Obtaining certificate from Let's Encrypt...")
	a.logger.Printf("[AutoSSL] Starting certificate obtainment for %s", a.config.Hostname)

	// Load saved challenge info
	challengePath := filepath.Join(a.cacheDir, "pending_challenge")
	challengeData, err := os.ReadFile(challengePath)
	if err != nil {
		return fmt.Errorf("failed to read pending challenge: %w", err)
	}

	parts := strings.SplitN(string(challengeData), "\n", 3)
	if len(parts) != 3 {
		return errors.New("invalid pending challenge data")
	}
	challengeURI := parts[0]

	// Load saved order
	orderPath := filepath.Join(a.cacheDir, "pending_order")
	orderData, err := os.ReadFile(orderPath)
	if err != nil {
		return fmt.Errorf("failed to read pending order: %w", err)
	}
	orderURI := string(orderData)

	// Initialize ACME client
	if err := a.initACMEClient(ctx); err != nil {
		a.setError("Failed to initialize ACME client: " + err.Error())
		return err
	}

	// Accept the challenge
	a.setStatus(StatusObtaining, "Submitting challenge to Let's Encrypt...")
	challenge, err := a.client.Accept(ctx, &acme.Challenge{URI: challengeURI})
	if err != nil {
		a.setError("Failed to accept challenge: " + err.Error())
		return fmt.Errorf("failed to accept challenge: %w", err)
	}

	// Wait for authorization
	a.setStatus(StatusObtaining, "Waiting for Let's Encrypt verification...")
	_, err = a.client.WaitAuthorization(ctx, challenge.URI)
	if err != nil {
		a.setError("Challenge verification failed: " + err.Error())
		return fmt.Errorf("challenge verification failed: %w", err)
	}

	// Wait for order to be ready
	a.setStatus(StatusObtaining, "Finalizing order...")
	order, err := a.client.WaitOrder(ctx, orderURI)
	if err != nil {
		a.setError("Order failed: " + err.Error())
		return fmt.Errorf("order failed: %w", err)
	}

	// Generate key and CSR
	a.setStatus(StatusObtaining, "Generating certificate...")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.setError("Failed to generate key: " + err.Error())
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		DNSNames: []string{a.config.Hostname},
	}, privateKey)
	if err != nil {
		a.setError("Failed to create CSR: " + err.Error())
		return fmt.Errorf("failed to create CSR: %w", err)
	}

	// Finalize order
	derChain, _, err := a.client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		a.setError("Failed to finalize certificate: " + err.Error())
		return fmt.Errorf("failed to finalize order: %w", err)
	}

	// Save certificate
	if err := a.saveCertificate(privateKey, derChain); err != nil {
		a.setError("Failed to save certificate: " + err.Error())
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Load the certificate
	if err := a.LoadCachedCertificate(); err != nil {
		a.setError("Failed to load certificate: " + err.Error())
		return fmt.Errorf("failed to load new certificate: %w", err)
	}

	// Cleanup pending files
	os.Remove(filepath.Join(a.cacheDir, "pending_challenge"))
	os.Remove(filepath.Join(a.cacheDir, "pending_order"))
	os.Remove(filepath.Join(a.cacheDir, "pending_token"))

	a.clearPendingChallenge()
	a.setStatus(StatusComplete, "Certificate obtained successfully! Restart server to enable HTTPS.")
	a.logger.Printf("[AutoSSL] Certificate obtained successfully for %s", a.config.Hostname)

	return nil
}

// obtainWithCloudflare handles the full automatic flow for Cloudflare
func (a *AutoSSLManager) obtainWithCloudflare(ctx context.Context) error {
	a.setStatus(StatusObtaining, "Starting automatic certificate flow via Cloudflare...")
	a.logger.Printf("[AutoSSL] Starting Cloudflare automatic flow for %s", a.config.Hostname)

	// Initialize ACME client
	if err := a.initACMEClient(ctx); err != nil {
		a.setError("Failed to initialize ACME client: " + err.Error())
		return err
	}

	// Create order
	a.setStatus(StatusObtaining, "Creating certificate order...")
	order, err := a.client.AuthorizeOrder(ctx, acme.DomainIDs(a.config.Hostname))
	if err != nil {
		a.setError("Failed to create order: " + err.Error())
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Process authorizations
	for _, authzURL := range order.AuthzURLs {
		authz, err := a.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			a.setError("Failed to get authorization: " + err.Error())
			return fmt.Errorf("failed to get authorization: %w", err)
		}

		if authz.Status == acme.StatusValid {
			continue
		}

		// Find DNS-01 challenge
		var dnsChallenge *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "dns-01" {
				dnsChallenge = c
				break
			}
		}

		if dnsChallenge == nil {
			a.setError("No DNS-01 challenge available")
			return errors.New("no DNS-01 challenge available")
		}

		// Get TXT value
		txtValue, err := a.client.DNS01ChallengeRecord(dnsChallenge.Token)
		if err != nil {
			a.setError("Failed to get challenge record: " + err.Error())
			return fmt.Errorf("failed to get challenge record: %w", err)
		}

		fqdn := "_acme-challenge." + a.config.Hostname

		// Create DNS record via Cloudflare
		a.setStatus(StatusObtaining, "Creating DNS record via Cloudflare...")
		if err := a.createCloudflareTXTRecord(ctx, fqdn, txtValue); err != nil {
			a.setError("Failed to create Cloudflare DNS record: " + err.Error())
			return err
		}

		// Wait for DNS propagation
		a.setStatus(StatusObtaining, "Waiting for DNS propagation...")
		if err := a.waitForDNS(ctx, fqdn, txtValue, 2*time.Minute); err != nil {
			a.deleteCloudflareTXTRecord(ctx, fqdn) // Cleanup
			a.setError("DNS propagation timeout: " + err.Error())
			return err
		}

		// Accept challenge
		a.setStatus(StatusObtaining, "Submitting challenge to Let's Encrypt...")
		if _, err := a.client.Accept(ctx, dnsChallenge); err != nil {
			a.deleteCloudflareTXTRecord(ctx, fqdn) // Cleanup
			a.setError("Failed to accept challenge: " + err.Error())
			return fmt.Errorf("failed to accept challenge: %w", err)
		}

		// Wait for verification
		a.setStatus(StatusObtaining, "Waiting for Let's Encrypt verification...")
		if _, err := a.client.WaitAuthorization(ctx, dnsChallenge.URI); err != nil {
			a.deleteCloudflareTXTRecord(ctx, fqdn) // Cleanup
			a.setError("Challenge verification failed: " + err.Error())
			return fmt.Errorf("challenge verification failed: %w", err)
		}

		// Cleanup DNS record
		a.deleteCloudflareTXTRecord(ctx, fqdn)
	}

	// Wait for order to be ready
	a.setStatus(StatusObtaining, "Finalizing order...")
	order, err = a.client.WaitOrder(ctx, order.URI)
	if err != nil {
		a.setError("Order failed: " + err.Error())
		return fmt.Errorf("order failed: %w", err)
	}

	// Generate key and CSR
	a.setStatus(StatusObtaining, "Generating certificate...")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.setError("Failed to generate key: " + err.Error())
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		DNSNames: []string{a.config.Hostname},
	}, privateKey)
	if err != nil {
		a.setError("Failed to create CSR: " + err.Error())
		return fmt.Errorf("failed to create CSR: %w", err)
	}

	// Finalize order
	derChain, _, err := a.client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		a.setError("Failed to finalize certificate: " + err.Error())
		return fmt.Errorf("failed to finalize order: %w", err)
	}

	// Save certificate
	if err := a.saveCertificate(privateKey, derChain); err != nil {
		a.setError("Failed to save certificate: " + err.Error())
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Load the certificate
	if err := a.LoadCachedCertificate(); err != nil {
		a.setError("Failed to load certificate: " + err.Error())
		return fmt.Errorf("failed to load new certificate: %w", err)
	}

	a.setStatus(StatusComplete, "Certificate obtained successfully! Restart server to enable HTTPS.")
	a.logger.Printf("[AutoSSL] Certificate obtained successfully for %s via Cloudflare", a.config.Hostname)

	return nil
}

// ============================================================================
// ACME Client Management
// ============================================================================

func (a *AutoSSLManager) initACMEClient(ctx context.Context) error {
	if a.client != nil {
		return nil
	}

	accountKeyPath := filepath.Join(a.cacheDir, "account.key")
	accountKey, err := a.loadOrCreateKey(accountKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load/create account key: %w", err)
	}

	a.client = &acme.Client{
		Key:          accountKey,
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}

	a.account, err = a.client.Register(ctx, &acme.Account{
		Contact: []string{"mailto:" + a.config.Email},
	}, acme.AcceptTOS)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			a.account, err = a.client.GetReg(ctx, "")
			if err != nil {
				return fmt.Errorf("failed to register/get account: %w", err)
			}
		}
	}

	return nil
}

func (a *AutoSSLManager) loadOrCreateKey(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block != nil && block.Type == "EC PRIVATE KEY" {
			return x509.ParseECPrivateKey(block.Bytes)
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}

	return key, nil
}

func (a *AutoSSLManager) waitForDNS(ctx context.Context, fqdn, expectedValue string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for DNS record")
			}
			if a.checkDNSRecord(fqdn, expectedValue) {
				return nil
			}
		}
	}
}

func (a *AutoSSLManager) saveCertificate(key crypto.Signer, derChain [][]byte) error {
	certPath := filepath.Join(a.cacheDir, a.config.Hostname+".crt")
	keyPath := filepath.Join(a.cacheDir, a.config.Hostname+".key")

	var certPEM []byte
	for _, der := range derChain {
		block := &pem.Block{Type: "CERTIFICATE", Bytes: der}
		certPEM = append(certPEM, pem.EncodeToMemory(block)...)
	}
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key.(*ecdsa.PrivateKey))
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return err
	}

	return nil
}

// ============================================================================
// Cloudflare DNS Management
// ============================================================================

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareZonesResponse struct {
	Success bool             `json:"success"`
	Result  []cloudflareZone `json:"result"`
}

type cloudflareDNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

type cloudflareCreateResponse struct {
	Success bool                `json:"success"`
	Result  cloudflareDNSRecord `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type cloudflareListResponse struct {
	Success bool                  `json:"success"`
	Result  []cloudflareDNSRecord `json:"result"`
}

func (a *AutoSSLManager) createCloudflareTXTRecord(ctx context.Context, fqdn, value string) error {
	zoneID := a.config.CloudflareZoneID
	if zoneID == "" {
		var err error
		zoneID, err = a.getCloudflareZoneID(ctx)
		if err != nil {
			return fmt.Errorf("failed to get zone ID: %w", err)
		}
	}

	record := cloudflareDNSRecord{
		Type:    "TXT",
		Name:    fqdn,
		Content: value,
		TTL:     60,
	}

	body, err := json.Marshal(record)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+a.config.CloudflareToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result cloudflareCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		errMsg := "unknown error"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Message
		}
		return fmt.Errorf("Cloudflare API error: %s", errMsg)
	}

	return nil
}

func (a *AutoSSLManager) getCloudflareZoneID(ctx context.Context) (string, error) {
	parts := strings.Split(a.config.Hostname, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid hostname")
	}
	rootDomain := strings.Join(parts[len(parts)-2:], ".")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", rootDomain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+a.config.CloudflareToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result cloudflareZonesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Success || len(result.Result) == 0 {
		return "", fmt.Errorf("zone not found for %s", rootDomain)
	}

	return result.Result[0].ID, nil
}

func (a *AutoSSLManager) deleteCloudflareTXTRecord(ctx context.Context, fqdn string) {
	zoneID := a.config.CloudflareZoneID
	if zoneID == "" {
		var err error
		zoneID, err = a.getCloudflareZoneID(ctx)
		if err != nil {
			return
		}
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=TXT&name=%s", zoneID, fqdn)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.config.CloudflareToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result cloudflareListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	for _, record := range result.Result {
		delURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, record.ID)
		delReq, err := http.NewRequestWithContext(ctx, "DELETE", delURL, nil)
		if err != nil {
			continue
		}
		delReq.Header.Set("Authorization", "Bearer "+a.config.CloudflareToken)
		delResp, err := http.DefaultClient.Do(delReq)
		if err != nil {
			continue
		}
		delResp.Body.Close()
	}
}

// ============================================================================
// Certificate Renewal
// ============================================================================

// StartRenewalLoop starts a background loop that renews the certificate before expiry
func (a *AutoSSLManager) StartRenewalLoop(ctx context.Context) {
	a.logger.Printf("[AutoSSL] Starting certificate renewal loop (checks every 12 hours)")
	go func() {
		// Check more frequently - every 12 hours
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				a.logger.Printf("[AutoSSL] Renewal loop stopped")
				return
			case <-ticker.C:
				a.checkAndRenewCertificate()
			}
		}
	}()
}

// checkAndRenewCertificate checks if certificate needs renewal (30 days before expiry)
func (a *AutoSSLManager) checkAndRenewCertificate() {
	certInfo, err := a.getCertInfo()
	if err != nil || certInfo == nil {
		a.logger.Printf("[AutoSSL] No certificate found during renewal check: %v", err)
		return
	}

	daysLeft := certInfo.DaysLeft
	a.logger.Printf("[AutoSSL] Certificate check: %d days until expiry", daysLeft)

	// Renew if less than 30 days remaining
	if daysLeft > 30 {
		return
	}

	a.logger.Printf("[AutoSSL] Certificate expires in %d days - renewal needed", daysLeft)

	// Only auto-renew if using Cloudflare (fully automatic)
	if a.config.DNSProvider == DNSProviderCloudflare {
		a.logger.Printf("[AutoSSL] Starting automatic renewal via Cloudflare...")
		renewCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := a.obtainWithCloudflare(renewCtx); err != nil {
			a.logger.Printf("[AutoSSL] Automatic renewal failed: %v", err)
			a.setError("Automatic renewal failed: " + err.Error())
		} else {
			a.logger.Printf("[AutoSSL] Certificate renewed successfully!")
			a.setStatus(StatusHasCertificate, "Certificate renewed successfully")
		}
	} else {
		// Manual mode - notify user via status
		a.logger.Printf("[AutoSSL] Manual renewal required - certificate expires in %d days", daysLeft)
		a.setStatus(StatusDNSPending, fmt.Sprintf("Certificate expires in %d days - renew via admin panel", daysLeft))
	}
}

// ============================================================================
// Reset / Cleanup
// ============================================================================

// Reset clears any pending state and allows starting fresh
func (a *AutoSSLManager) Reset() {
	a.statusMu.Lock()
	a.status = StatusReady
	a.statusMsg = "Ready to obtain certificate"
	a.lastError = ""
	a.pendingFQDN = ""
	a.pendingValue = ""
	a.dnsVerified = false
	a.statusMu.Unlock()

	// Clean up pending files
	os.Remove(filepath.Join(a.cacheDir, "pending_challenge"))
	os.Remove(filepath.Join(a.cacheDir, "pending_order"))
	os.Remove(filepath.Join(a.cacheDir, "pending_token"))

	// Reset ACME client so we get fresh orders
	a.client = nil
	a.account = nil
}

// ============================================================================
// Simple Factory
// ============================================================================

// NewAutoSSLManagerSimple creates an AutoSSL manager with minimal configuration
func NewAutoSSLManagerSimple(hostname, email, cacheDir string, logger *log.Logger) (*AutoSSLManager, error) {
	return NewAutoSSLManager(AutoSSLConfig{
		Hostname:    hostname,
		Email:       email,
		CacheDir:    cacheDir,
		DNSProvider: DNSProviderManual,
	}, logger)
}

// NewAutoSSLManagerWithCloudflare creates an AutoSSL manager with Cloudflare DNS
func NewAutoSSLManagerWithCloudflare(hostname, email, cacheDir, cfToken string, logger *log.Logger) (*AutoSSLManager, error) {
	return NewAutoSSLManager(AutoSSLConfig{
		Hostname:        hostname,
		Email:           email,
		CacheDir:        cacheDir,
		DNSProvider:     DNSProviderCloudflare,
		CloudflareToken: cfToken,
	}, logger)
}
