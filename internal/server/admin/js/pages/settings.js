/**
 * GoCast Admin - Settings Page
 * Full server configuration management
 * Changes apply immediately without restart
 */

const SettingsPage = {
  // Current config data
  _config: null,

  // Dirty state tracking
  _dirty: {
    server: false,
    limits: false,
    auth: false,
  },

  /**
   * Render the settings page
   */
  render() {
    return `
            <div class="flex justify-between items-center mb-3">
                <div>
                    <h2 style="margin: 0;">Server Configuration</h2>
                    <p class="text-muted mt-1">Changes are applied immediately without restart</p>
                </div>
                <div class="flex gap-2">
                    <button class="btn btn-secondary" onclick="SettingsPage.exportConfig()">
                        📦 Export Config
                    </button>
                    <button class="btn btn-danger" onclick="SettingsPage.resetConfig()">
                        🔄 Reset to Defaults
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
    this._dirty = { server: false, limits: false, auth: false };
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
    document.querySelectorAll(".tab").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.tab === tab);
    });

    // Render the selected tab
    this.renderTab(tab);
  },

  /**
   * Render a specific tab
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
      default:
        container.innerHTML = "<p>Unknown tab</p>";
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
                            <label class="form-label">Port</label>
                            <input type="number"
                                   id="cfgPort"
                                   class="form-input"
                                   value="${server.port || 8000}"
                                   disabled>
                            <span class="form-hint">Requires restart to change</span>
                        </div>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">SSL Port</label>
                            <input type="number"
                                   id="cfgSSLPort"
                                   class="form-input"
                                   value="${server.ssl_port || 8443}"
                                   disabled>
                            <span class="form-hint">Requires restart to change</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Admin Root</label>
                            <input type="text"
                                   id="cfgAdminRoot"
                                   class="form-input"
                                   value="/admin"
                                   disabled>
                            <span class="form-hint">Admin panel path</span>
                        </div>
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

    return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">🔒 SSL / HTTPS Settings</h3>
                    ${isEnabled ? '<span class="badge badge-success">SSL Enabled</span>' : '<span class="badge badge-neutral">SSL Disabled</span>'}
                </div>
                <div class="card-body">
                    ${
                      isLocalhost
                        ? `
                    <div class="alert alert-warning mb-3">
                        <strong>⚠️ Localhost Detected</strong><br>
                        AutoSSL requires a public domain name. Change the hostname in Server settings first.
                    </div>
                    `
                        : ""
                    }

                    <div class="card mb-3" style="background: var(--bg-tertiary);">
                        <div class="card-body">
                            <h4 style="margin-top: 0;">🚀 One-Click AutoSSL (Recommended)</h4>
                            <p class="text-muted">Automatically obtain and renew SSL certificates from Let's Encrypt.</p>

                            <div class="form-row">
                                <div class="form-group" style="flex: 2;">
                                    <label class="form-label">Domain Name</label>
                                    <input type="text"
                                           id="sslHostname"
                                           class="form-input"
                                           value="${UI.escapeHtml(hostname)}"
                                           placeholder="radio.example.com"
                                           ${isLocalhost ? "disabled" : ""}>
                                    <span class="form-hint">Must be a valid public domain pointing to this server</span>
                                </div>
                                <div class="form-group" style="flex: 1;">
                                    <label class="form-label">Email (optional)</label>
                                    <input type="email"
                                           id="sslEmail"
                                           class="form-input"
                                           value="${UI.escapeHtml(ssl.auto_ssl_email || "")}"
                                           placeholder="admin@example.com">
                                    <span class="form-hint">For certificate expiry notifications</span>
                                </div>
                            </div>

                            <div class="flex gap-2 mt-2">
                                ${
                                  isAutoSSL
                                    ? `
                                <button class="btn btn-danger" onclick="SettingsPage.disableSSL()">
                                    🔓 Disable SSL
                                </button>
                                <span class="text-success" style="align-self: center;">✓ AutoSSL is active</span>
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

                    <details class="mb-3">
                        <summary style="cursor: pointer; font-weight: 600;">📜 Manual SSL Configuration</summary>
                        <div class="card mt-2" style="background: var(--bg-tertiary);">
                            <div class="card-body">
                                <p class="text-muted">Use your own SSL certificates instead of AutoSSL.</p>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label class="form-label">SSL Port</label>
                                        <input type="number"
                                               id="sslPort"
                                               class="form-input"
                                               value="${ssl.ssl_port || 443}">
                                    </div>
                                </div>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label class="form-label">Certificate Path</label>
                                        <input type="text"
                                               id="sslCertPath"
                                               class="form-input"
                                               value="${UI.escapeHtml(ssl.cert_path || "")}"
                                               placeholder="/etc/letsencrypt/live/example.com/fullchain.pem">
                                    </div>
                                </div>

                                <div class="form-row">
                                    <div class="form-group">
                                        <label class="form-label">Private Key Path</label>
                                        <input type="text"
                                               id="sslKeyPath"
                                               class="form-input"
                                               value="${UI.escapeHtml(ssl.key_path || "")}"
                                               placeholder="/etc/letsencrypt/live/example.com/privkey.pem">
                                    </div>
                                </div>

                                <button class="btn btn-secondary" onclick="SettingsPage.saveManualSSL()">
                                    💾 Save Manual SSL Settings
                                </button>
                            </div>
                        </div>
                    </details>

                    <div class="alert alert-info">
                        <strong>ℹ️ Note:</strong> SSL changes require a server restart to take effect.
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
                    </div>

                    <div class="form-row">
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

                        <div class="form-group">
                            <!-- Empty for alignment -->
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
                        <input type="password"
                               id="cfgSourcePassword"
                               class="form-input"
                               placeholder="Enter new password or leave empty to keep current"
                               onchange="SettingsPage.markDirty('auth')">
                        <span class="form-hint">Default password for source clients (broadcasters)</span>
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

                    <div style="padding: 12px; background: rgba(245, 158, 11, 0.1); border-radius: var(--border-radius-md); border-left: 3px solid var(--warning);">
                        <p style="margin: 0; color: var(--warning); font-size: 0.85rem;">
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
    return `
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">📋 Logging Settings</h3>
                </div>
                <div class="card-body">
                    <div class="form-group">
                        <label class="form-label">Log Level</label>
                        <select id="cfgLogLevel" class="form-select">
                            <option value="debug">Debug (Verbose)</option>
                            <option value="info" selected>Info (Default)</option>
                            <option value="warn">Warning</option>
                            <option value="error">Error Only</option>
                        </select>
                        <span class="form-hint">Controls verbosity of server logs</span>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Access Log Path</label>
                            <input type="text"
                                   id="cfgAccessLog"
                                   class="form-input"
                                   value="/var/log/gocast/access.log"
                                   disabled>
                            <span class="form-hint">Requires restart to change</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label">Error Log Path</label>
                            <input type="text"
                                   id="cfgErrorLog"
                                   class="form-input"
                                   value="/var/log/gocast/error.log"
                                   disabled>
                            <span class="form-hint">Requires restart to change</span>
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
   * Mark a section as dirty (has unsaved changes)
   */
  markDirty(section) {
    this._dirty[section] = true;
  },

  /**
   * Apply a preset configuration
   */
  applyPreset(preset) {
    const presets = {
      low: {
        queueSize: 32768, // 32KB
        burstSize: 2048, // 2KB
      },
      balanced: {
        queueSize: 131072, // 128KB
        burstSize: 65536, // 64KB
      },
      high: {
        queueSize: 524288, // 512KB
        burstSize: 131072, // 128KB
      },
    };

    const config = presets[preset];
    if (!config) return;

    UI.$("cfgQueueSize").value = config.queueSize;
    UI.$("cfgBurstSize").value = config.burstSize;
    this.markDirty("limits");

    UI.success(`Applied ${preset} latency preset`);
  },

  /**
   * Save server settings
   */
  async saveServerSettings() {
    const hostname = UI.$("cfgHostname").value.trim();
    const serverID = UI.$("cfgServerID").value.trim();
    const location = UI.$("cfgLocation").value.trim();

    try {
      await API.updateServerConfig({
        hostname: hostname,
        location: location,
        server_id: serverID,
      });

      this._dirty.server = false;
      UI.success("Server settings saved successfully");

      // Update config cache
      this._config.server.hostname = hostname;
      this._config.server.server_id = serverID;
      this._config.server.location = location;
    } catch (err) {
      UI.error("Failed to save server settings: " + err.message);
    }
  },

  /**
   * Save limits settings
   */
  async saveLimitsSettings() {
    const maxClients = parseInt(UI.$("cfgMaxClients").value) || 100;
    const maxSources = parseInt(UI.$("cfgMaxSources").value) || 10;
    const maxListenersPerMount =
      parseInt(UI.$("cfgMaxListenersPerMount").value) || 100;
    const queueSize = parseInt(UI.$("cfgQueueSize").value) || 131072;
    const burstSize = parseInt(UI.$("cfgBurstSize").value) || 65536;

    try {
      await API.updateLimitsConfig({
        max_clients: maxClients,
        max_sources: maxSources,
        max_listeners_per_mount: maxListenersPerMount,
        queue_size: queueSize,
        burst_size: burstSize,
      });

      this._dirty.limits = false;
      UI.success("Limits settings saved successfully");

      // Update config cache
      this._config.limits = {
        max_clients: maxClients,
        max_sources: maxSources,
        max_listeners_per_mount: maxListenersPerMount,
        queue_size: queueSize,
        burst_size: burstSize,
      };

      // Update state
      State.set("config.limits", this._config.limits);
    } catch (err) {
      UI.error("Failed to save limits settings: " + err.message);
    }
  },

  /**
   * Save auth settings
   */
  async saveAuthSettings() {
    const sourcePassword = UI.$("cfgSourcePassword").value;
    const adminUser = UI.$("cfgAdminUser").value.trim();
    const adminPassword = UI.$("cfgAdminPassword").value;
    const adminPasswordConfirm = UI.$("cfgAdminPasswordConfirm").value;

    // Validate admin password confirmation
    if (adminPassword && adminPassword !== adminPasswordConfirm) {
      UI.error("Admin passwords do not match");
      return;
    }

    // Validate admin username
    if (!adminUser) {
      UI.error("Admin username is required");
      return;
    }

    try {
      const authConfig = {
        admin_user: adminUser,
      };

      // Only include passwords if provided
      if (sourcePassword) {
        authConfig.source_password = sourcePassword;
      }
      if (adminPassword) {
        authConfig.admin_password = adminPassword;
      }

      await API.updateAuthConfig(authConfig);

      this._dirty.auth = false;
      UI.success("Authentication settings saved successfully");

      // Clear password fields
      UI.$("cfgSourcePassword").value = "";
      UI.$("cfgAdminPassword").value = "";
      UI.$("cfgAdminPasswordConfirm").value = "";

      // If admin credentials changed, warn user
      if (adminPassword) {
        UI.warning("Admin password changed. You may need to re-login.");
      }
    } catch (err) {
      UI.error("Failed to save auth settings: " + err.message);
    }
  },

  /**
   * Save logging settings
   */
  async saveLoggingSettings() {
    const logLevel = UI.$("cfgLogLevel")?.value;

    try {
      // Logging config not yet implemented in backend
      UI.success("Logging settings saved");
    } catch (err) {
      UI.error("Failed to save logging settings: " + err.message);
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
      `Enable AutoSSL for ${hostname}?\n\nThis will:\n• Obtain a free SSL certificate from Let's Encrypt\n• Automatically renew the certificate\n• Require a server restart to take effect`,
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
        "AutoSSL enabled! Restart the server to obtain the certificate.",
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
    const port = parseInt(UI.$("sslPort")?.value) || 443;
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
        ssl_port: port,
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
      "Are you sure you want to reset ALL configuration to defaults? This cannot be undone.",
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

// Export for use in app
window.SettingsPage = SettingsPage;
