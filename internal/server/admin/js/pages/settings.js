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
    const isAutoSSL = ssl.auto_ssl || false;
    const isEnabled = ssl.enabled || false;
    const hostname = ssl.hostname || server.hostname || "localhost";
    const isLocalhost = hostname === "localhost" || hostname === "127.0.0.1";
    const sslPort = ssl.port || 8443;

    return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🔒 SSL / HTTPS Settings</h3>
                    ${
                      isEnabled
                        ? isAutoSSL
                          ? '<span class="badge badge-success">AutoSSL Active</span>'
                          : '<span class="badge badge-success">SSL Enabled</span>'
                        : '<span class="badge badge-neutral">SSL Disabled</span>'
                    }
                </div>
                <div class="card-body">
                    ${
                      isLocalhost
                        ? `
                    <div class="alert alert-warning mb-3">
                        <strong>⚠️ Localhost Detected</strong><br>
                        AutoSSL requires a public domain name. Go to <strong>Server</strong> tab and set a valid hostname first (e.g., radio.example.com).
                    </div>
                    `
                        : ""
                    }

                    ${
                      isEnabled && isAutoSSL
                        ? `
                    <div class="alert alert-success mb-3">
                        <strong>✅ AutoSSL is Active</strong><br>
                        Your server is secured with a Let's Encrypt certificate for <strong>${UI.escapeHtml(hostname)}</strong>.<br>
                        HTTPS URL: <a href="https://${UI.escapeHtml(hostname)}:${sslPort}" target="_blank">https://${UI.escapeHtml(hostname)}:${sslPort}</a>
                    </div>
                    `
                        : ""
                    }

                    <div class="card mb-3" style="background: var(--bg-tertiary);">
                        <div class="card-body">
                            <h4 style="margin-top: 0;">🚀 One-Click AutoSSL (Recommended)</h4>
                            <p class="text-muted">Automatically obtain and renew free SSL certificates from Let's Encrypt. No manual certificate management needed!</p>

                            <div class="form-row">
                                <div class="form-group" style="flex: 2;">
                                    <label class="form-label">Domain Name *</label>
                                    <input type="text"
                                           id="sslHostname"
                                           class="form-input"
                                           value="${UI.escapeHtml(hostname)}"
                                           placeholder="radio.example.com"
                                           ${isLocalhost ? "disabled" : ""}>
                                    <span class="form-hint">Your public domain pointing to this server (DNS must be configured)</span>
                                </div>
                                <div class="form-group" style="flex: 1;">
                                    <label class="form-label">Email (recommended)</label>
                                    <input type="email"
                                           id="sslEmail"
                                           class="form-input"
                                           value="${UI.escapeHtml(ssl.auto_ssl_email || "")}"
                                           placeholder="admin@example.com">
                                    <span class="form-hint">Get notified before certificate expires</span>
                                </div>
                            </div>

                            <div class="flex gap-2 mt-2 items-center">
                                ${
                                  isAutoSSL
                                    ? `
                                <button class="btn btn-danger" onclick="SettingsPage.disableSSL()">
                                    🔓 Disable SSL
                                </button>
                                <span class="text-success">✓ AutoSSL is active on port ${sslPort}</span>
                                `
                                    : `
                                <button class="btn btn-primary" onclick="SettingsPage.enableAutoSSL()" ${isLocalhost ? "disabled" : ""}>
                                    🔒 Enable AutoSSL
                                </button>
                                ${isLocalhost ? '<span class="text-muted">Set a public hostname first</span>' : '<span class="text-muted">Will listen on port 8443</span>'}
                                `
                                }
                            </div>

                            ${
                              !isAutoSSL && !isLocalhost
                                ? `
                            <div class="alert alert-info mt-3" style="margin-bottom: 0;">
                                <strong>📋 Before enabling AutoSSL:</strong>
                                <ol style="margin: 8px 0 0 20px; padding: 0;">
                                    <li>Point your domain's DNS to this server's IP address</li>
                                    <li>Ensure port 80 is accessible (for Let's Encrypt verification)</li>
                                    <li>Ensure port 8443 is accessible (for HTTPS traffic)</li>
                                </ol>
                            </div>
                            `
                                : ""
                            }
                        </div>
                    </div>

                    <details class="mb-3">
                        <summary style="cursor: pointer; font-weight: 600;">📜 Manual SSL Configuration (Advanced)</summary>
                        <div class="card mt-2" style="background: var(--bg-tertiary);">
                            <div class="card-body">
                                <p class="text-muted">Use your own SSL certificates (e.g., from a commercial CA or self-signed).</p>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label class="form-label">SSL Port</label>
                                        <input type="number"
                                               id="sslPort"
                                               class="form-input"
                                               value="${sslPort}"
                                               min="1"
                                               max="65535">
                                        <span class="form-hint">Default: 8443 (use 443 if you have root access)</span>
                                    </div>
                                </div>

                                <div class="form-group">
                                    <label class="form-label">Certificate Path (fullchain.pem)</label>
                                    <input type="text"
                                           id="sslCertPath"
                                           class="form-input"
                                           value="${UI.escapeHtml(ssl.cert_path || "")}"
                                           placeholder="/etc/ssl/certs/server.crt">
                                    <span class="form-hint">Full path to your SSL certificate file</span>
                                </div>

                                <div class="form-group">
                                    <label class="form-label">Private Key Path (privkey.pem)</label>
                                    <input type="text"
                                           id="sslKeyPath"
                                           class="form-input"
                                           value="${UI.escapeHtml(ssl.key_path || "")}"
                                           placeholder="/etc/ssl/private/server.key">
                                    <span class="form-hint">Full path to your SSL private key file</span>
                                </div>

                                <button class="btn btn-secondary" onclick="SettingsPage.saveManualSSL()">
                                    💾 Save Manual SSL Settings
                                </button>
                            </div>
                        </div>
                    </details>

                    <div class="alert alert-info">
                        <strong>ℹ️ Note:</strong> After enabling SSL, restart GoCast for changes to take effect.
                        Your streams will be available at both HTTP and HTTPS URLs.
                    </div>
                </div>
            </div>
        `;
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
   * Enable AutoSSL
   */
  async enableAutoSSL() {
    const hostname = UI.$("sslHostname")?.value?.trim();
    const email = UI.$("sslEmail")?.value?.trim();

    if (!hostname || hostname === "localhost") {
      UI.error("Please enter a valid public domain name");
      return;
    }

    const confirmed = await UI.confirm(
      `Enable AutoSSL for ${hostname}?\n\nThis will:\n• Obtain a free SSL certificate from Let's Encrypt\n• Automatically renew the certificate\n• Listen on HTTPS port 8443\n• Require a server restart to take effect\n\nMake sure port 80 and 8443 are accessible from the internet.`,
      {
        title: "Enable AutoSSL",
        confirmText: "Enable SSL",
        danger: false,
      },
    );

    if (!confirmed) return;

    try {
      await API.post("/config/ssl/enable", { hostname, email });
      UI.success(
        "AutoSSL enabled! Restart the server to start HTTPS on port 8443.",
      );
      await this.loadConfig();
    } catch (err) {
      UI.error("Failed to enable AutoSSL: " + err.message);
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
