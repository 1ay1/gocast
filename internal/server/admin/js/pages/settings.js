/**
 * GoCast Admin - Settings Page
 * Full server configuration management
 * All settings are persisted and can be changed from UI
 */

const SettingsPage = {
    // Current config data
    _config: null,

    // Dirty state tracking
    _dirty: {
        server: false,
        limits: false,
        auth: false,
        logging: false,
        directory: false,
    },

    /**
     * Render the settings page
     */
    render() {
        return `
            <div class="flex justify-between items-center mb-3">
                <div>
                    <h2 style="margin: 0;">Server Configuration</h2>
                    <p class="text-muted mt-1">All changes apply immediately - no restart required</p>
                </div>
                <div class="flex gap-2">
                    <button class="btn btn-secondary" onclick="SettingsPage.reloadConfig()">
                        🔄 Reload from Disk
                    </button>
                    <button class="btn btn-secondary" onclick="SettingsPage.exportConfig()">
                        📦 Export Config
                    </button>
                    <button class="btn btn-danger" onclick="SettingsPage.resetConfig()">
                        ⚠️ Reset to Defaults
                    </button>
                </div>
            </div>

            <div class="tabs mb-3">
                <button class="tab active" data-tab="server" onclick="SettingsPage.switchTab('server')">
                    🖥️ Server
                </button>
                <button class="tab" data-tab="ssl" onclick="SettingsPage.switchTab('ssl')">
                    🔒 SSL
                </button>
                <button class="tab" data-tab="limits" onclick="SettingsPage.switchTab('limits')">
                    🚦 Limits
                </button>
                <button class="tab" data-tab="auth" onclick="SettingsPage.switchTab('auth')">
                    🔐 Authentication
                </button>
                <button class="tab" data-tab="logging" onclick="SettingsPage.switchTab('logging')">
                    📋 Logging
                </button>
                <button class="tab" data-tab="directory" onclick="SettingsPage.switchTab('directory')">
                    📡 Directory
                </button>
            </div>

            <div id="settingsContainer">
                <div class="loading">
                    <div class="spinner"></div>
                    <p>Loading configuration...</p>
                </div>
            </div>
        `;
    },

    // Current active tab
    _activeTab: "server",

    /**
     * Initialize the page
     */
    async init() {
        this._activeTab = "server";
        await this.loadConfig();
    },

    /**
     * Clean up when leaving page
     */
    destroy() {
        this._config = null;
        this._dirty = {
            server: false,
            limits: false,
            auth: false,
            logging: false,
            directory: false,
        };
    },

    /**
     * Load configuration from server
     */
    async loadConfig() {
        try {
            this._config = await API.getConfig();
            this.renderTab(this._activeTab);
        } catch (err) {
            console.error("Settings load error:", err);
            UI.error("Failed to load configuration: " + err.message);
        }
    },

    /**
     * Switch between settings tabs
     */
    switchTab(tab) {
        this._activeTab = tab;

        // Update tab buttons
        document.querySelectorAll(".tabs .tab").forEach((btn) => {
            btn.classList.toggle("active", btn.dataset.tab === tab);
        });

        this.renderTab(tab);
    },

    /**
     * Render the current tab content
     */
    renderTab(tab) {
        const container = UI.$("settingsContainer");
        if (!container || !this._config) return;

        switch (tab) {
            case "server":
                container.innerHTML = this.renderServerTab();
                break;
            case "ssl":
                container.innerHTML = this.renderSSLTab();
                // Auto-load SSL status if AutoSSL is enabled
                if (this._config.ssl?.auto_ssl) {
                    setTimeout(() => this.refreshSSLStatus(), 100);
                }
                break;
            case "limits":
                container.innerHTML = this.renderLimitsTab();
                break;
            case "auth":
                container.innerHTML = this.renderAuthTab();
                break;
            case "logging":
                container.innerHTML = this.renderLoggingTab();
                break;
            case "directory":
                container.innerHTML = this.renderDirectoryTab();
                break;
        }
    },

    /**
     * Render server settings tab
     */
    renderServerTab() {
        const server = this._config.server || {};

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🖥️ Server Settings</h3>
                </div>
                <div class="card-body">
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Hostname</label>
                            <input type="text"
                                   id="cfgHostname"
                                   class="form-input"
                                   value="${UI.escapeHtml(server.hostname || "localhost")}"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">Public hostname for the server</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Server ID</label>
                            <input type="text"
                                   id="cfgServerID"
                                   class="form-input"
                                   value="${UI.escapeHtml(server.server_id || "GoCast")}"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">Identifier sent in server headers</span>
                        </div>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Location</label>
                            <input type="text"
                                   id="cfgLocation"
                                   class="form-input"
                                   value="${UI.escapeHtml(server.location || "Earth")}"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">Server location (metadata only)</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Listen Address</label>
                            <input type="text"
                                   id="cfgListenAddress"
                                   class="form-input"
                                   value="${UI.escapeHtml(server.listen_address || "0.0.0.0")}"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">IP address to bind to (0.0.0.0 = all interfaces)</span>
                        </div>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Port</label>
                            <input type="number"
                                   id="cfgPort"
                                   class="form-input"
                                   value="${server.port || 8000}"
                                   min="1"
                                   max="65535"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">HTTP port for the server</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Admin Root Path</label>
                            <input type="text"
                                   id="cfgAdminRoot"
                                   class="form-input"
                                   value="${UI.escapeHtml(server.admin_root || "/admin")}"
                                   onchange="SettingsPage.markDirty('server')">
                            <span class="form-hint">URL path for admin panel</span>
                        </div>
                    </div>

                    <div class="alert alert-info mt-2">
                        <strong>💡 Tip:</strong> Settings are automatically persisted to <code>~/.gocast/config.json</code>.
                        You can also edit this file directly and click "Reload from Disk".
                    </div>
                </div>
                <div class="card-footer">
                    <button class="btn btn-primary" onclick="SettingsPage.saveServerSettings()" id="saveServerBtn">
                        💾 Save Server Settings
                    </button>
                </div>
            </div>
        `;
    },

    /**
     * Render SSL settings tab
     */
    renderSSLTab() {
        const ssl = this._config.ssl || {};
        const server = this._config.server || {};
        const isAutoSSLConfigured = ssl.auto_ssl || false;
        const isEnabled = ssl.enabled || false;
        const hostname = ssl.hostname || server.hostname || "localhost";
        const isLocalhost =
            hostname === "localhost" || hostname === "127.0.0.1";
        const sslPort = ssl.port || 8443;
        const dnsProvider = ssl.dns_provider || "manual";
        const hasCloudflareToken = !!ssl.cloudflare_token;

        // Badge shows config state - actual certificate status shown in status panel
        let badgeHtml;
        if (!isAutoSSLConfigured && !isEnabled) {
            badgeHtml =
                '<span class="badge badge-neutral">Not Configured</span>';
        } else if (isAutoSSLConfigured) {
            // Will be updated by status panel to show if cert exists
            badgeHtml =
                '<span class="badge badge-warning" id="sslBadge">AutoSSL Configured</span>';
        } else {
            badgeHtml = '<span class="badge badge-success">Manual SSL</span>';
        }

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🔒 SSL / HTTPS Settings</h3>
                    ${badgeHtml}
                </div>
                <div class="card-body">
                    ${
                        isLocalhost
                            ? `
                    <div class="alert alert-warning mb-3">
                        <strong>⚠️ Localhost Detected</strong><br>
                        AutoSSL requires a public domain name. Go to <strong>Server</strong> tab and set a valid hostname first.
                    </div>
                    `
                            : ""
                    }

                    <!-- Certificate Status Panel (shown when AutoSSL is configured) -->
                    ${
                        isAutoSSLConfigured
                            ? `
                    <div id="sslStatusPanel" class="card mb-3" style="background: var(--bg-tertiary);">
                        <div class="card-body">
                            <div class="flex justify-between items-center mb-2">
                                <h4 style="margin: 0;">📜 Certificate Status</h4>
                                <button class="btn btn-sm" onclick="SettingsPage.refreshSSLStatus()">🔄 Refresh</button>
                            </div>
                            <div id="sslStatusContent">
                                <p class="text-muted">Loading status...</p>
                            </div>
                        </div>
                    </div>
                    `
                            : ""
                    }

                    <!-- AutoSSL Configuration -->
                    <div class="card mb-3" style="background: var(--bg-tertiary);">
                        <div class="card-body">
                            <h4 style="margin-top: 0;">🚀 AutoSSL Configuration</h4>
                            <p class="text-muted">Free SSL certificates from Let's Encrypt via DNS verification. Works on any port!</p>

                            <div class="form-row">
                                <div class="form-group" style="flex: 2;">
                                    <label class="form-label">Domain Name *</label>
                                    <input type="text"
                                           id="sslHostname"
                                           class="form-input"
                                           value="${UI.escapeHtml(hostname)}"
                                           placeholder="radio.example.com"
                                           ${isLocalhost || isAutoSSLConfigured ? "disabled" : ""}>
                                </div>
                                <div class="form-group" style="flex: 1;">
                                    <label class="form-label">Email</label>
                                    <input type="email"
                                           id="sslEmail"
                                           class="form-input"
                                           value="${UI.escapeHtml(ssl.auto_ssl_email || "")}"
                                           placeholder="admin@example.com"
                                           ${isAutoSSLConfigured ? "disabled" : ""}>
                                </div>
                            </div>

                            <div class="form-group">
                                <label class="form-label">DNS Provider</label>
                                <div class="flex gap-3 mt-1">
                                    <label class="flex items-center gap-2" style="cursor: pointer;">
                                        <input type="radio" name="dnsProvider" value="cloudflare"
                                               ${dnsProvider === "cloudflare" ? "checked" : ""}
                                               ${isAutoSSLConfigured ? "disabled" : ""}
                                               onchange="SettingsPage.toggleDNSProvider('cloudflare')">
                                        <span><strong>☁️ Cloudflare</strong> (Automatic)</span>
                                    </label>
                                    <label class="flex items-center gap-2" style="cursor: pointer;">
                                        <input type="radio" name="dnsProvider" value="manual"
                                               ${dnsProvider !== "cloudflare" ? "checked" : ""}
                                               ${isAutoSSLConfigured ? "disabled" : ""}
                                               onchange="SettingsPage.toggleDNSProvider('manual')">
                                        <span><strong>✋ Manual</strong></span>
                                    </label>
                                </div>
                            </div>

                            <div id="cloudflareSettings" style="display: ${dnsProvider === "cloudflare" ? "block" : "none"};">
                                <div class="form-group">
                                    <label class="form-label">Cloudflare API Token *</label>
                                    <input type="password"
                                           id="cloudflareToken"
                                           class="form-input"
                                           value="${hasCloudflareToken ? "••••••••" : ""}"
                                           placeholder="Your Cloudflare API token"
                                           ${isAutoSSLConfigured ? "disabled" : ""}>
                                    <span class="form-hint">
                                        <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank">Get token</a>
                                        with Zone:DNS:Edit permission
                                    </span>
                                </div>
                            </div>

                            <div id="manualDNSSettings" style="display: ${dnsProvider !== "cloudflare" ? "block" : "none"};">
                                <p class="text-muted" style="font-size: 0.9em;">
                                    After enabling, you'll add a DNS TXT record. The panel will guide you through it.
                                </p>
                            </div>

                            <div class="flex gap-2 mt-3 items-center">
                                ${
                                    isAutoSSLConfigured
                                        ? `
                                <button class="btn btn-danger" onclick="SettingsPage.disableSSL()">
                                    🔓 Disable AutoSSL
                                </button>
                                <span class="text-muted">AutoSSL is configured. Use the status panel above to obtain your certificate.</span>
                                `
                                        : `
                                <button class="btn btn-primary" onclick="SettingsPage.enableAutoSSL()" ${isLocalhost ? "disabled" : ""}>
                                    🔒 Enable AutoSSL
                                </button>
                                `
                                }
                            </div>
                        </div>
                    </div>

                    <!-- Manual SSL (collapsed) -->
                    <details class="mb-3">
                        <summary style="cursor: pointer; font-weight: 600;">📜 Manual SSL (Advanced)</summary>
                        <div class="card mt-2" style="background: var(--bg-tertiary);">
                            <div class="card-body">
                                <div class="form-group">
                                    <label class="form-label">SSL Port</label>
                                    <input type="number" id="sslPort" class="form-input" value="${sslPort}" min="1" max="65535">
                                </div>
                                <div class="form-group">
                                    <label class="form-label">Certificate Path</label>
                                    <input type="text" id="sslCertPath" class="form-input" value="${UI.escapeHtml(ssl.cert_path || "")}" placeholder="/path/to/cert.pem">
                                </div>
                                <div class="form-group">
                                    <label class="form-label">Private Key Path</label>
                                    <input type="text" id="sslKeyPath" class="form-input" value="${UI.escapeHtml(ssl.key_path || "")}" placeholder="/path/to/key.pem">
                                </div>
                                <button class="btn btn-secondary" onclick="SettingsPage.saveManualSSL()">💾 Save</button>
                            </div>
                        </div>
                    </details>
                </div>
            </div>
        `;
    },

    /**
     * Refresh SSL status from server
     */
    async refreshSSLStatus() {
        const panel = document.getElementById("sslStatusContent");
        if (!panel) return;

        panel.innerHTML = '<p class="text-muted">Loading...</p>';

        try {
            const status = await API.get("/config/ssl/status");
            panel.innerHTML = this.renderSSLStatusContent(status);

            // Update the badge based on actual certificate status and HTTPS state
            const badge = document.getElementById("sslBadge");
            if (badge) {
                if (
                    (status.status === "has_certificate" ||
                        status.status === "complete") &&
                    status.https_running
                ) {
                    badge.className = "badge badge-success";
                    badge.textContent = "✓ HTTPS Active";
                } else if (
                    status.status === "has_certificate" ||
                    status.status === "complete"
                ) {
                    badge.className = "badge badge-warning";
                    badge.textContent = "✓ Cert Ready (Restart Required)";
                } else if (status.status === "error") {
                    badge.className = "badge badge-danger";
                    badge.textContent = "Error";
                } else if (
                    status.status === "obtaining" ||
                    status.status === "dns_verifying"
                ) {
                    badge.className = "badge badge-warning";
                    badge.textContent = "In Progress...";
                } else if (
                    status.status === "dns_pending" ||
                    status.status === "dns_verified"
                ) {
                    badge.className = "badge badge-warning";
                    badge.textContent = "Pending DNS";
                } else {
                    badge.className = "badge badge-warning";
                    badge.textContent = "No Certificate";
                }
            }
        } catch (err) {
            panel.innerHTML = `<div class="alert alert-danger">Failed to load status: ${err.message}</div>`;
        }
    },

    /**
     * Render SSL status content based on current state
     */
    renderSSLStatusContent(status) {
        const s = status.status || "none";
        const isCloudflare = status.dns_provider === "cloudflare";
        const httpsRunning = status.https_running || false;
        const httpsPort = status.https_port || 8443;
        const hostname = this._config?.server?.hostname || "localhost";

        // Has valid certificate
        if (s === "has_certificate" || s === "complete") {
            const cert = status.certificate || {};
            const httpsUrl =
                httpsPort === 443
                    ? `https://${hostname}`
                    : `https://${hostname}:${httpsPort}`;

            return `
                <div class="alert alert-success">
                    <strong>✅ Certificate Active</strong><br>
                    Domain: <strong>${cert.domain || hostname}</strong><br>
                    Expires: ${cert.not_after || "—"} (${cert.days_left || 0} days left)
                </div>
                ${
                    httpsRunning
                        ? `
                <div class="alert alert-success" style="margin-top: 8px;">
                    <strong>🔒 HTTPS is Running</strong><br>
                    Your server is accessible at: <a href="${httpsUrl}" target="_blank">${httpsUrl}</a>
                </div>
                `
                        : `
                <div class="alert alert-warning" style="margin-top: 8px;">
                    <strong>⚠️ HTTPS Not Running</strong><br>
                    Restart the server to enable HTTPS on port ${httpsPort}.
                </div>
                `
                }
            `;
        }

        // Cloudflare - fully automatic
        if (isCloudflare && (s === "none" || s === "ready")) {
            return `
                <div class="alert alert-info">
                    <strong>☁️ Cloudflare Mode</strong><br>
                    Certificate will be obtained automatically via Cloudflare DNS.<br>
                    HTTPS will start automatically after certificate is obtained - no restart needed!
                </div>
                <button class="btn btn-primary" onclick="SettingsPage.obtainCertificate()">
                    🚀 Get Certificate Now
                </button>
            `;
        }

        // Cloudflare - obtaining
        if (isCloudflare && s === "obtaining") {
            return `
                <div class="alert alert-info">
                    <strong>🔄 ${status.message || "Obtaining certificate..."}</strong>
                </div>
                <p class="text-muted">This usually takes about 30 seconds...</p>
            `;
        }

        // Step 1: Ready to start (Manual mode)
        if (s === "none" || s === "ready") {
            return `
                <div class="alert alert-info">
                    <strong>📋 Ready to Start</strong><br>
                    Click below to generate a DNS challenge record.<br>
                    <span style="font-size: 0.9em;">HTTPS will start automatically after certificate is obtained - no restart needed!</span>
                </div>
                <div class="alert alert-warning" style="font-size: 0.9em;">
                    <strong>⚠️ Important:</strong> Each click generates a <strong>NEW</strong> value.
                    If you have old <code>_acme-challenge</code> TXT records from previous attempts,
                    <strong>delete them first</strong> before adding the new one.
                </div>
                <button class="btn btn-primary" onclick="SettingsPage.prepareDNS()">
                    ▶️ Step 1: Generate DNS Record
                </button>
            `;
        }

        // Step 2: DNS record generated, waiting for user to add it
        if (s === "dns_pending" && status.fqdn && status.txt_value) {
            return `
                <div class="alert alert-warning">
                    <strong>📝 Step 2: Add DNS Record</strong><br>
                    Add this <strong>exact</strong> TXT record to your DNS provider.
                    <strong>Delete any old _acme-challenge records first!</strong>
                </div>
                <div style="background: var(--bg-primary); padding: 16px; border-radius: 8px; margin: 12px 0; font-family: monospace; font-size: 0.9em;">
                    <div style="margin-bottom: 8px;"><strong>Name:</strong> <code>${UI.escapeHtml(status.fqdn)}</code></div>
                    <div style="margin-bottom: 8px;"><strong>Type:</strong> <code>TXT</code></div>
                    <div><strong>Value (copy exactly):</strong></div>
                    <div style="background: var(--bg-secondary); padding: 8px; border-radius: 4px; margin-top: 4px; word-break: break-all; user-select: all; cursor: pointer;" onclick="navigator.clipboard.writeText('${UI.escapeHtml(status.txt_value)}'); UI.success('Copied to clipboard!');" title="Click to copy">
                        <code>${UI.escapeHtml(status.txt_value)}</code>
                    </div>
                    <div style="margin-top: 4px; font-size: 0.85em; color: var(--text-muted);">👆 Click value to copy</div>
                </div>
                <div class="flex gap-2 flex-wrap">
                    <button class="btn btn-primary" onclick="SettingsPage.verifyDNS()">
                        ✓ Step 3: Verify DNS Record
                    </button>
                    <button class="btn btn-sm" onclick="SettingsPage.refreshSSLStatus()">🔄 Refresh</button>
                    <button class="btn btn-sm btn-danger" onclick="SettingsPage.resetSSL()">✕ Cancel</button>
                </div>
                <p class="text-muted mt-2" style="font-size: 0.85em;">
                    ⏱️ DNS propagation takes 1-5 minutes. Make sure you've <strong>deleted old records</strong> and added this exact value.
                </p>
            `;
        }

        // Step 2b: Verifying DNS
        if (s === "dns_verifying") {
            return `
                <div class="alert alert-info">
                    <strong>🔍 Checking DNS...</strong><br>
                    ${status.message || "Verifying DNS record propagation..."}
                </div>
            `;
        }

        // Step 3: DNS verified, ready to get certificate
        if (s === "dns_verified") {
            return `
                <div class="alert alert-success">
                    <strong>✅ DNS Verified!</strong><br>
                    The DNS record was found. Now you can obtain your certificate.
                </div>
                <button class="btn btn-primary" onclick="SettingsPage.obtainCertificate()">
                    🔒 Step 4: Get Certificate
                </button>
                <p class="text-muted mt-2" style="font-size: 0.85em;">
                    HTTPS will start automatically after certificate is obtained - no restart needed!
                </p>
            `;
        }

        // Obtaining certificate
        if (s === "obtaining") {
            return `
                <div class="alert alert-info">
                    <strong>🔄 ${status.message || "Obtaining certificate..."}</strong>
                </div>
                <p class="text-muted">Communicating with Let's Encrypt... This may take a minute.</p>
            `;
        }

        // Error
        if (s === "error") {
            return `
                <div class="alert alert-danger">
                    <strong>❌ Error</strong><br>
                    ${UI.escapeHtml(status.error || "Unknown error")}
                </div>
                <div class="flex gap-2 mt-2">
                    <button class="btn btn-primary" onclick="SettingsPage.prepareDNS()">🔄 Try Again</button>
                    <button class="btn btn-sm btn-danger" onclick="SettingsPage.resetSSL()">✕ Reset</button>
                </div>
            `;
        }

        // Fallback
        return `
            <div class="alert alert-info">
                <strong>Status: ${s}</strong><br>
                ${status.message || ""}
            </div>
            <button class="btn btn-primary" onclick="SettingsPage.prepareDNS()">▶️ Start</button>
        `;
    },

    /**
     * Step 1: Prepare DNS challenge (generate TXT record to add)
     */
    async prepareDNS() {
        try {
            const panel = document.getElementById("sslStatusContent");
            if (panel) {
                panel.innerHTML =
                    '<div class="alert alert-info">Generating DNS challenge...</div>';
            }

            const result = await API.post("/config/ssl/prepare", {});
            UI.success("DNS record generated. Add it to your DNS provider.");
            this.refreshSSLStatus();
        } catch (err) {
            UI.error("Failed to prepare: " + err.message);
            this.refreshSSLStatus();
        }
    },

    /**
     * Step 2: Verify DNS record has propagated
     */
    async verifyDNS() {
        try {
            const panel = document.getElementById("sslStatusContent");
            if (panel) {
                panel.innerHTML =
                    '<div class="alert alert-info">🔍 Checking DNS propagation...</div>';
            }

            await API.post("/config/ssl/verify", {});
            UI.success("✅ DNS verified! You can now get your certificate.");
            this.refreshSSLStatus();
        } catch (err) {
            UI.error(err.message);
            this.refreshSSLStatus();
        }
    },

    /**
     * Step 3: Obtain the certificate (after DNS is verified)
     */
    async obtainCertificate() {
        try {
            const panel = document.getElementById("sslStatusContent");
            if (panel) {
                panel.innerHTML =
                    '<div class="alert alert-info">🔄 Obtaining certificate from Let\'s Encrypt...</div>';
            }

            // Start polling immediately for progress updates
            this._pollSSLStatus();

            const result = await API.post("/config/ssl/obtain", {});
            UI.success("🎉 " + (result.message || "Certificate obtained!"));
            this.refreshSSLStatus();
        } catch (err) {
            UI.error("Failed: " + err.message);
            this.refreshSSLStatus();
        }
    },

    /**
     * Reset SSL state to start fresh
     */
    async resetSSL() {
        const confirmed = await UI.confirm(
            "Reset the SSL certificate process? This will cancel any pending challenges.",
            { title: "Reset SSL", confirmText: "Reset", danger: true },
        );

        if (!confirmed) return;

        try {
            await API.post("/config/ssl/reset", {});
            UI.success("SSL state reset.");
            this.refreshSSLStatus();
        } catch (err) {
            UI.error("Failed to reset: " + err.message);
        }
    },

    /**
     * Poll SSL status for progress updates
     */
    _pollSSLStatus() {
        let attempts = 0;
        const maxAttempts = 120; // 10 minutes max

        const poll = async () => {
            attempts++;
            if (attempts > maxAttempts) {
                this.refreshSSLStatus();
                return;
            }

            try {
                const status = await API.get("/config/ssl/status");
                const panel = document.getElementById("sslStatusContent");
                if (panel) {
                    panel.innerHTML = this.renderSSLStatusContent(status);
                }

                // Continue polling if still in progress
                if (
                    status.status === "obtaining" ||
                    status.status === "dns_verifying"
                ) {
                    setTimeout(poll, 2000);
                } else if (
                    status.status === "complete" ||
                    status.status === "has_certificate"
                ) {
                    if (status.https_running) {
                        UI.success(
                            "🎉 SSL Certificate obtained and HTTPS is now active!",
                        );
                    } else {
                        UI.success(
                            "🎉 SSL Certificate obtained! Restart server to enable HTTPS.",
                        );
                    }
                }
            } catch (err) {
                console.error("Poll error:", err);
                setTimeout(poll, 3000);
            }
        };

        setTimeout(poll, 1000);
    },

    /**
     * Render limits settings tab
     */
    renderLimitsTab() {
        const limits = this._config.limits || {};

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🚦 Resource Limits</h3>
                </div>
                <div class="card-body">
                    <h4 style="margin-top: 0;">Connection Limits</h4>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Max Clients</label>
                            <input type="number"
                                   id="cfgMaxClients"
                                   class="form-input"
                                   value="${limits.max_clients || 100}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Maximum total listeners allowed</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Max Sources</label>
                            <input type="number"
                                   id="cfgMaxSources"
                                   class="form-input"
                                   value="${limits.max_sources || 10}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Maximum concurrent source connections</span>
                        </div>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Max Listeners per Mount</label>
                            <input type="number"
                                   id="cfgMaxListenersPerMount"
                                   class="form-input"
                                   value="${limits.max_listeners_per_mount || 100}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Default limit per mount point</span>
                        </div>

                        <div class="form-group">
                            <!-- Empty for alignment -->
                        </div>
                    </div>

                    <hr style="border: none; border-top: 1px solid var(--border-color); margin: 24px 0;">

                    <h4>Buffer Settings</h4>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Queue Size (bytes)</label>
                            <input type="number"
                                   id="cfgQueueSize"
                                   class="form-input"
                                   value="${limits.queue_size || 131072}"
                                   min="1024"
                                   step="1024"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Buffer size per listener (131072 = 128KB)</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Burst Size (bytes)</label>
                            <input type="number"
                                   id="cfgBurstSize"
                                   class="form-input"
                                   value="${limits.burst_size || 65536}"
                                   min="0"
                                   step="1024"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Initial burst data sent to new listeners</span>
                        </div>
                    </div>

                    <div class="mt-2" style="padding: 16px; background: var(--bg-tertiary); border-radius: var(--border-radius-md);">
                        <h4 style="margin: 0 0 12px 0; font-size: 0.9rem;">Quick Presets</h4>
                        <div class="flex gap-2">
                            <button class="btn btn-sm btn-secondary" onclick="SettingsPage.applyPreset('low')">
                                📱 Low Latency
                            </button>
                            <button class="btn btn-sm btn-secondary" onclick="SettingsPage.applyPreset('balanced')">
                                ⚖️ Balanced
                            </button>
                            <button class="btn btn-sm btn-secondary" onclick="SettingsPage.applyPreset('high')">
                                📻 High Quality
                            </button>
                        </div>
                    </div>

                    <hr style="border: none; border-top: 1px solid var(--border-color); margin: 24px 0;">

                    <h4>Timeout Settings</h4>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Client Timeout (seconds)</label>
                            <input type="number"
                                   id="cfgClientTimeout"
                                   class="form-input"
                                   value="${limits.client_timeout || 30}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Timeout for inactive client connections</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Header Timeout (seconds)</label>
                            <input type="number"
                                   id="cfgHeaderTimeout"
                                   class="form-input"
                                   value="${limits.header_timeout || 5}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Time allowed to receive request headers</span>
                        </div>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Source Timeout (seconds)</label>
                            <input type="number"
                                   id="cfgSourceTimeout"
                                   class="form-input"
                                   value="${limits.source_timeout || 5}"
                                   min="1"
                                   onchange="SettingsPage.markDirty('limits')">
                            <span class="form-hint">Timeout for source client handshake</span>
                        </div>

                        <div class="form-group">
                            <!-- Empty for alignment -->
                        </div>
                    </div>
                </div>
                <div class="card-footer">
                    <button class="btn btn-primary" onclick="SettingsPage.saveLimitsSettings()" id="saveLimitsBtn">
                        💾 Save Limits Settings
                    </button>
                </div>
            </div>
        `;
    },

    /**
     * Render auth settings tab
     */
    renderAuthTab() {
        const auth = this._config.auth || {};

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🔐 Authentication Settings</h3>
                </div>
                <div class="card-body">
                    <div class="form-group">
                        <label class="form-label">Source Password</label>
                        <div class="input-group">
                            <input type="password"
                                   id="cfgSourcePassword"
                                   class="form-input"
                                   value="${UI.escapeHtml(auth.source_password || "")}"
                                   placeholder="Source password for broadcasters"
                                   onchange="SettingsPage.markDirty('auth')">
                            <button type="button" class="btn btn-secondary" onclick="UI.togglePassword('cfgSourcePassword', this)" title="Show/Hide">👁️</button>
                        </div>
                        <span class="form-hint">Default password for source clients (broadcasters). This is the password your streaming software uses.</span>
                    </div>

                    <hr style="border: none; border-top: 1px solid var(--border-color); margin: 24px 0;">

                    <h4 style="margin: 0 0 16px 0;">Admin Credentials</h4>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Admin Username</label>
                            <input type="text"
                                   id="cfgAdminUser"
                                   class="form-input"
                                   value="${UI.escapeHtml(auth.admin_user || "admin")}"
                                   onchange="SettingsPage.markDirty('auth')">
                        </div>

                        <div class="form-group">
                            <label class="form-label">New Admin Password</label>
                            <input type="password"
                                   id="cfgAdminPassword"
                                   class="form-input"
                                   placeholder="Leave empty to keep current"
                                   onchange="SettingsPage.markDirty('auth')">
                        </div>
                    </div>

                    <div class="form-group">
                        <label class="form-label">Confirm New Admin Password</label>
                        <input type="password"
                               id="cfgAdminPasswordConfirm"
                               class="form-input"
                               placeholder="Confirm new password">
                    </div>

                    <div class="alert alert-warning">
                        <p style="margin: 0;">
                            ⚠️ <strong>Warning:</strong> Changing admin credentials will require you to re-login.
                        </p>
                    </div>
                </div>
                <div class="card-footer">
                    <button class="btn btn-primary" onclick="SettingsPage.saveAuthSettings()" id="saveAuthBtn">
                        💾 Save Authentication Settings
                    </button>
                </div>
            </div>
        `;
    },

    /**
     * Render logging settings tab
     */
    renderLoggingTab() {
        const logging = this._config.logging || {};

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">📋 Logging Settings</h3>
                </div>
                <div class="card-body">
                    <div class="form-group">
                        <label class="form-label">Log Level</label>
                        <select id="cfgLogLevel" class="form-select" onchange="SettingsPage.markDirty('logging')">
                            <option value="debug" ${logging.log_level === "debug" ? "selected" : ""}>Debug (Verbose)</option>
                            <option value="info" ${logging.log_level === "info" || !logging.log_level ? "selected" : ""}>Info (Default)</option>
                            <option value="warn" ${logging.log_level === "warn" ? "selected" : ""}>Warning</option>
                            <option value="error" ${logging.log_level === "error" ? "selected" : ""}>Error Only</option>
                        </select>
                        <span class="form-hint">Controls verbosity of server logs (applied immediately)</span>
                    </div>

                    <div class="form-group">
                        <label class="form-label">Log Buffer Size</label>
                        <input type="number"
                               id="cfgLogSize"
                               class="form-input"
                               value="${logging.log_size || 10000}"
                               min="100"
                               max="100000"
                               onchange="SettingsPage.markDirty('logging')">
                        <span class="form-hint">Number of log entries to keep in memory for the admin panel</span>
                    </div>

                    <hr style="border: none; border-top: 1px solid var(--border-color); margin: 24px 0;">

                    <h4>Log File Paths</h4>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Access Log Path</label>
                            <input type="text"
                                   id="cfgAccessLog"
                                   class="form-input"
                                   value="${UI.escapeHtml(logging.access_log || "/var/log/gocast/access.log")}"
                                   onchange="SettingsPage.markDirty('logging')">
                            <span class="form-hint">Path for access log file</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Error Log Path</label>
                            <input type="text"
                                   id="cfgErrorLog"
                                   class="form-input"
                                   value="${UI.escapeHtml(logging.error_log || "/var/log/gocast/error.log")}"
                                   onchange="SettingsPage.markDirty('logging')">
                            <span class="form-hint">Path for error log file</span>
                        </div>
                    </div>
                </div>
                <div class="card-footer">
                    <button class="btn btn-primary" onclick="SettingsPage.saveLoggingSettings()">
                        💾 Save Logging Settings
                    </button>
                </div>
            </div>
        `;
    },

    /**
     * Render directory/YP settings tab
     */
    renderDirectoryTab() {
        const directory = this._config.directory || {};
        const ypUrls = (directory.yp_urls || []).join("\n");

        return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">📡 Directory / Yellow Pages Settings</h3>
                </div>
                <div class="card-body">
                    <div class="form-group">
                        <label class="form-label">
                            <input type="checkbox"
                                   id="cfgDirectoryEnabled"
                                   ${directory.enabled ? "checked" : ""}
                                   onchange="SettingsPage.markDirty('directory')">
                            Enable Directory Listing
                        </label>
                        <span class="form-hint">Publish your streams to directory servers (Yellow Pages)</span>
                    </div>

                    <div class="form-group">
                        <label class="form-label">Directory URLs</label>
                        <textarea id="cfgYPURLs"
                                  class="form-input"
                                  rows="4"
                                  placeholder="https://dir.xiph.org/cgi-bin/yp-cgi&#10;http://dir.example.com/yp"
                                  onchange="SettingsPage.markDirty('directory')">${UI.escapeHtml(ypUrls)}</textarea>
                        <span class="form-hint">One URL per line. Common: https://dir.xiph.org/cgi-bin/yp-cgi</span>
                    </div>

                    <div class="form-group">
                        <label class="form-label">Update Interval (seconds)</label>
                        <input type="number"
                               id="cfgYPInterval"
                               class="form-input"
                               value="${directory.interval || 600}"
                               min="60"
                               max="3600"
                               onchange="SettingsPage.markDirty('directory')">
                        <span class="form-hint">How often to update directory listings (min: 60s)</span>
                    </div>

                    <div class="alert alert-info">
                        <strong>ℹ️ About Directory Listings:</strong><br>
                        When enabled, GoCast will periodically announce your public streams to directory servers.
                        Listeners can then find your streams through directory search.
                        Make sure your mount points have proper metadata (name, description, genre) for best discoverability.
                    </div>
                </div>
                <div class="card-footer">
                    <button class="btn btn-primary" onclick="SettingsPage.saveDirectorySettings()">
                        💾 Save Directory Settings
                    </button>
                </div>
            </div>
        `;
    },

    /**
     * Mark a section as dirty (changed)
     */
    markDirty(section) {
        this._dirty[section] = true;
    },

    /**
     * Apply buffer preset
     */
    applyPreset(preset) {
        const presets = {
            low: {
                queueSize: 65536, // 64KB
                burstSize: 2048, // 2KB
            },
            balanced: {
                queueSize: 131072, // 128KB
                burstSize: 32768, // 32KB
            },
            high: {
                queueSize: 524288, // 512KB
                burstSize: 65536, // 64KB
            },
        };

        const config = presets[preset];
        if (!config) return;

        UI.$("cfgQueueSize").value = config.queueSize;
        UI.$("cfgBurstSize").value = config.burstSize;
        this.markDirty("limits");

        UI.success(`Applied "${preset}" preset - don't forget to save!`);
    },

    /**
     * Save server settings
     */
    async saveServerSettings() {
        const hostname = UI.$("cfgHostname")?.value?.trim();
        const serverID = UI.$("cfgServerID")?.value?.trim();
        const location = UI.$("cfgLocation")?.value?.trim();
        const listenAddress = UI.$("cfgListenAddress")?.value?.trim();
        const port = parseInt(UI.$("cfgPort")?.value) || 8000;
        const adminRoot = UI.$("cfgAdminRoot")?.value?.trim();

        try {
            await API.post("/config/server", {
                hostname,
                location,
                server_id: serverID,
                listen_address: listenAddress,
                port,
                admin_root: adminRoot,
            });
            this._dirty.server = false;
            UI.success("Server settings saved");
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to save server settings: " + err.message);
        }
    },

    /**
     * Save limits settings
     */
    async saveLimitsSettings() {
        const maxClients = parseInt(UI.$("cfgMaxClients")?.value) || 100;
        const maxSources = parseInt(UI.$("cfgMaxSources")?.value) || 10;
        const maxListenersPerMount =
            parseInt(UI.$("cfgMaxListenersPerMount")?.value) || 100;
        const queueSize = parseInt(UI.$("cfgQueueSize")?.value) || 131072;
        const burstSize = parseInt(UI.$("cfgBurstSize")?.value) || 65536;
        const clientTimeout = parseInt(UI.$("cfgClientTimeout")?.value) || 30;
        const headerTimeout = parseInt(UI.$("cfgHeaderTimeout")?.value) || 5;
        const sourceTimeout = parseInt(UI.$("cfgSourceTimeout")?.value) || 5;

        try {
            await API.post("/config/limits", {
                max_clients: maxClients,
                max_sources: maxSources,
                max_listeners_per_mount: maxListenersPerMount,
                queue_size: queueSize,
                burst_size: burstSize,
                client_timeout: clientTimeout,
                header_timeout: headerTimeout,
                source_timeout: sourceTimeout,
            });
            this._dirty.limits = false;
            this._config.limits = {
                max_clients: maxClients,
                max_sources: maxSources,
                max_listeners_per_mount: maxListenersPerMount,
                queue_size: queueSize,
                burst_size: burstSize,
                client_timeout: clientTimeout,
                header_timeout: headerTimeout,
                source_timeout: sourceTimeout,
            };
            UI.success("Limits settings saved");
        } catch (err) {
            UI.error("Failed to save limits settings: " + err.message);
        }
    },

    /**
     * Save auth settings
     */
    async saveAuthSettings() {
        const sourcePassword = UI.$("cfgSourcePassword")?.value;
        const adminUser = UI.$("cfgAdminUser")?.value?.trim();
        const adminPassword = UI.$("cfgAdminPassword")?.value;
        const adminPasswordConfirm = UI.$("cfgAdminPasswordConfirm")?.value;

        // Validate password confirmation
        if (adminPassword && adminPassword !== adminPasswordConfirm) {
            UI.error("Admin passwords do not match");
            return;
        }

        // Build config object
        const authConfig = {
            admin_user: adminUser,
        };

        if (sourcePassword) {
            authConfig.source_password = sourcePassword;
        }

        if (adminPassword) {
            authConfig.admin_password = adminPassword;
        }

        try {
            await API.post("/config/auth", authConfig);
            this._dirty.auth = false;

            if (adminPassword) {
                UI.success(
                    "Auth settings saved. You may need to re-login with the new credentials.",
                );
            } else {
                UI.success("Auth settings saved");
            }

            // Clear admin password fields (keep source password visible)
            UI.$("cfgAdminPassword").value = "";
            UI.$("cfgAdminPasswordConfirm").value = "";

            // Reload config to get fresh values
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to save auth settings: " + err.message);
        }
    },

    /**
     * Save logging settings
     */
    async saveLoggingSettings() {
        const logLevel = UI.$("cfgLogLevel")?.value || "info";
        const accessLog = UI.$("cfgAccessLog")?.value?.trim();
        const errorLog = UI.$("cfgErrorLog")?.value?.trim();
        const logSize = parseInt(UI.$("cfgLogSize")?.value) || 10000;

        try {
            await API.post("/config/logging", {
                log_level: logLevel,
                access_log: accessLog,
                error_log: errorLog,
                log_size: logSize,
            });
            this._dirty.logging = false;
            UI.success("Logging settings saved");
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to save logging settings: " + err.message);
        }
    },

    /**
     * Save directory settings
     */
    async saveDirectorySettings() {
        const enabled = UI.$("cfgDirectoryEnabled")?.checked || false;
        const ypUrlsText = UI.$("cfgYPURLs")?.value || "";
        const interval = parseInt(UI.$("cfgYPInterval")?.value) || 600;

        // Parse URLs (one per line, filter empty)
        const ypUrls = ypUrlsText
            .split("\n")
            .map((u) => u.trim())
            .filter((u) => u.length > 0);

        try {
            await API.post("/config/directory", {
                enabled,
                yp_urls: ypUrls,
                interval,
            });
            this._dirty.directory = false;
            UI.success("Directory settings saved");
        } catch (err) {
            UI.error("Failed to save directory settings: " + err.message);
        }
    },

    /**
     * Toggle DNS provider visibility
     */
    toggleDNSProvider(provider) {
        const cloudflareSettings =
            document.getElementById("cloudflareSettings");
        const manualSettings = document.getElementById("manualDNSSettings");

        if (provider === "cloudflare") {
            cloudflareSettings.style.display = "block";
            manualSettings.style.display = "none";
        } else {
            cloudflareSettings.style.display = "none";
            manualSettings.style.display = "block";
        }
    },

    /**
     * Enable AutoSSL
     */
    async enableAutoSSL() {
        const hostname = UI.$("sslHostname")?.value?.trim();
        const email = UI.$("sslEmail")?.value?.trim();
        const dnsProvider =
            document.querySelector('input[name="dnsProvider"]:checked')
                ?.value || "manual";
        const cloudflareToken = UI.$("cloudflareToken")?.value?.trim();

        if (!hostname || hostname === "localhost") {
            UI.error("Please enter a valid public domain name");
            return;
        }

        if (
            dnsProvider === "cloudflare" &&
            (!cloudflareToken || cloudflareToken.includes("•"))
        ) {
            UI.error("Please enter your Cloudflare API token");
            return;
        }

        const confirmed = await UI.confirm(
            `Enable AutoSSL for ${hostname}?\n\nAfter restart, go to Settings → SSL to obtain your certificate.`,
            { title: "Enable AutoSSL", confirmText: "Enable", danger: false },
        );

        if (!confirmed) return;

        try {
            const payload = { hostname, email, dns_provider: dnsProvider };
            if (
                dnsProvider === "cloudflare" &&
                cloudflareToken &&
                !cloudflareToken.includes("•")
            ) {
                payload.cloudflare_token = cloudflareToken;
            }

            await API.post("/config/ssl/enable", payload);
            UI.success(
                "AutoSSL enabled! Restart server, then return here to get your certificate.",
            );
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed: " + err.message);
        }
    },

    /**
     * Disable SSL
     */
    async disableSSL() {
        const confirmed = await UI.confirm(
            "Disable SSL?\n\nThe server will only be accessible via HTTP after restart.",
            {
                title: "Disable SSL",
                confirmText: "Disable",
                danger: true,
            },
        );

        if (!confirmed) return;

        try {
            await API.post("/config/ssl/disable", {});
            UI.success("SSL disabled. Restart the server to apply.");
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to disable SSL: " + err.message);
        }
    },

    /**
     * Save manual SSL settings
     */
    async saveManualSSL() {
        const port = parseInt(UI.$("sslPort")?.value) || 8443;
        const certPath = UI.$("sslCertPath")?.value?.trim();
        const keyPath = UI.$("sslKeyPath")?.value?.trim();

        if (!certPath || !keyPath) {
            UI.error("Please provide both certificate and key paths");
            return;
        }

        try {
            await API.post("/config/ssl", {
                enabled: true,
                auto_ssl: false,
                port: port,
                cert_path: certPath,
                key_path: keyPath,
            });
            UI.success("SSL settings saved. Restart the server to apply.");
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to save SSL settings: " + err.message);
        }
    },

    /**
     * Reload configuration from disk
     */
    async reloadConfig() {
        try {
            await API.post("/config/reload", {});
            UI.success("Configuration reloaded from disk");
            await this.loadConfig();
        } catch (err) {
            UI.error("Failed to reload configuration: " + err.message);
        }
    },

    /**
     * Export configuration
     */
    async exportConfig() {
        try {
            const config = await API.exportConfig();
            const blob = new Blob([JSON.stringify(config, null, 2)], {
                type: "application/json",
            });
            const url = URL.createObjectURL(blob);

            const a = document.createElement("a");
            a.href = url;
            a.download = `gocast-config-${new Date().toISOString().split("T")[0]}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            UI.success("Configuration exported successfully");
        } catch (err) {
            UI.error("Failed to export configuration: " + err.message);
        }
    },

    /**
     * Reset configuration to defaults
     */
    async resetConfig() {
        const confirmed = await UI.confirm(
            "Are you sure you want to reset ALL configuration to defaults?\n\nThis will:\n• Clear all custom settings\n• Reset passwords to defaults\n• Remove all mount configurations\n\nThis cannot be undone!",
            {
                title: "Reset Configuration",
                confirmText: "Reset All",
                danger: true,
            },
        );

        if (confirmed) {
            try {
                await API.resetConfig();
                UI.success("Configuration reset to defaults");
                await this.loadConfig();
            } catch (err) {
                UI.error("Failed to reset configuration: " + err.message);
            }
        }
    },
};
