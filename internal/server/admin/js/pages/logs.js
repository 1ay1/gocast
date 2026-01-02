/**
 * GoCast Admin - Logs Page
 * Real-time activity log viewer
 */

const LogsPage = {
  // Log entries
  _logs: [],

  // Max log entries to keep
  _maxLogs: 500,

  // Current filter
  _filter: "all",

  // Auto-scroll enabled
  _autoScroll: true,

  // Update interval
  _interval: null,

  /**
   * Render the logs page
   */
  render() {
    return `
            <div class="flex justify-between items-center mb-3">
                <div class="flex gap-2 items-center">
                    <label class="form-label" style="margin: 0;">Filter:</label>
                    <select id="logFilter" class="form-select" style="width: 160px;" onchange="LogsPage.setFilter(this.value)">
                        <option value="all">All Events</option>
                        <option value="connect">Connections</option>
                        <option value="disconnect">Disconnections</option>
                        <option value="source">Sources</option>
                        <option value="config">Config Changes</option>
                        <option value="error">Errors</option>
                    </select>

                    <label class="form-checkbox" style="margin-left: 16px;">
                        <input type="checkbox" id="autoScrollCheck" checked onchange="LogsPage.toggleAutoScroll(this.checked)">
                        <span>Auto-scroll</span>
                    </label>
                </div>

                <div class="flex gap-2">
                    <button class="btn btn-secondary" onclick="LogsPage.exportLogs()">
                        📥 Export
                    </button>
                    <button class="btn btn-danger" onclick="LogsPage.clearLogs()">
                        🗑️ Clear
                    </button>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">📋 Activity Log</h3>
                    <span class="badge badge-neutral" id="logCountBadge">0 entries</span>
                </div>
                <div class="card-body" id="logsContainer" style="height: 600px; overflow-y: auto; font-family: 'SF Mono', 'Monaco', 'Consolas', monospace; font-size: 0.85rem;">
                    <div class="empty-state">
                        <div class="empty-icon">📋</div>
                        <div class="empty-title">No Log Entries</div>
                        <div class="empty-text">Activity will be logged here in real-time</div>
                    </div>
                </div>
            </div>
        `;
  },

  /**
   * Initialize the page
   */
  async init() {
    this._logs = [];
    this._filter = "all";
    this._autoScroll = true;

    // Subscribe to real-time events
    API.on("listener", (data) => this.handleListenerEvent(data));
    API.on("source", (data) => this.handleSourceEvent(data));
    API.on("config", (data) => this.handleConfigEvent(data));
    API.on("message", (data) => this.handleGenericEvent(data));

    // Add initial log entry
    this.addLog("info", "Log viewer started");

    // Start polling for status updates
    this._interval = setInterval(() => this.pollStatus(), 5000);
  },

  /**
   * Clean up when leaving page
   */
  destroy() {
    if (this._interval) {
      clearInterval(this._interval);
      this._interval = null;
    }
  },

  /**
   * Poll status for changes
   */
  async pollStatus() {
    try {
      const status = await API.getStatus();
      // We could compare with previous status and log changes
      // For now, we just rely on SSE events
    } catch (err) {
      this.addLog("error", "Failed to poll status: " + err.message);
    }
  },

  /**
   * Handle listener event
   */
  handleListenerEvent(data) {
    const action = data.action || "connected";
    const mount = data.mount || "unknown";
    const ip = data.ip || "";

    const type = action === "disconnected" ? "disconnect" : "connect";
    const message = ip
      ? `Listener ${action} on ${mount} from ${ip}`
      : `Listener ${action} on ${mount}`;

    this.addLog(type, message, data);
  },

  /**
   * Handle source event
   */
  handleSourceEvent(data) {
    const action = data.action || "started";
    const mount = data.mount || "unknown";
    const message = `Source ${action} on ${mount}`;

    this.addLog("source", message, data);
  },

  /**
   * Handle config event
   */
  handleConfigEvent(data) {
    const section = data.section || "unknown";
    const message = `Configuration updated: ${section}`;

    this.addLog("config", message, data);
  },

  /**
   * Handle generic SSE event
   */
  handleGenericEvent(data) {
    if (data.type && !["listener", "source", "config", "stats"].includes(data.type)) {
      this.addLog("info", data.message || JSON.stringify(data), data);
    }
  },

  /**
   * Add a log entry
   */
  addLog(type, message, data = null) {
    const entry = {
      id: Date.now() + Math.random(),
      time: new Date(),
      type: type,
      message: message,
      data: data,
    };

    this._logs.unshift(entry);

    // Trim to max size
    if (this._logs.length > this._maxLogs) {
      this._logs.length = this._maxLogs;
    }

    // Update display
    this.renderLogs();
  },

  /**
   * Set filter
   */
  setFilter(filter) {
    this._filter = filter;
    this.renderLogs();
  },

  /**
   * Toggle auto-scroll
   */
  toggleAutoScroll(enabled) {
    this._autoScroll = enabled;
  },

  /**
   * Get filtered logs
   */
  getFilteredLogs() {
    if (this._filter === "all") {
      return this._logs;
    }

    return this._logs.filter((log) => log.type === this._filter);
  },

  /**
   * Render logs list
   */
  renderLogs() {
    const container = UI.$("logsContainer");
    if (!container) return;

    const logs = this.getFilteredLogs();

    // Update badge
    const badge = UI.$("logCountBadge");
    if (badge) {
      badge.textContent = `${logs.length} entries`;
    }

    if (logs.length === 0) {
      container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📋</div>
                    <div class="empty-title">No Log Entries</div>
                    <div class="empty-text">${this._filter === "all" ? "Activity will be logged here in real-time" : "No entries match the current filter"}</div>
                </div>
            `;
      return;
    }

    // Render log entries
    container.innerHTML = logs.map((log) => this.renderLogEntry(log)).join("");

    // Auto-scroll to top (newest entries)
    if (this._autoScroll) {
      container.scrollTop = 0;
    }
  },

  /**
   * Render a single log entry
   */
  renderLogEntry(log) {
    const typeColors = {
      connect: "var(--success)",
      disconnect: "var(--error)",
      source: "var(--info)",
      config: "var(--warning)",
      error: "var(--error)",
      info: "var(--text-muted)",
    };

    const typeIcons = {
      connect: "🟢",
      disconnect: "🔴",
      source: "📡",
      config: "⚙️",
      error: "⚠️",
      info: "ℹ️",
    };

    const time = log.time.toLocaleTimeString();
    const date = log.time.toLocaleDateString();
    const color = typeColors[log.type] || "var(--text-secondary)";
    const icon = typeIcons[log.type] || "•";

    return `
            <div class="log-entry" style="border-left: 3px solid ${color}; padding-left: 12px;">
                <span class="log-time" style="color: var(--text-muted); min-width: 140px; display: inline-block;">
                    ${date} ${time}
                </span>
                <span style="min-width: 30px; display: inline-block; text-align: center;">
                    ${icon}
                </span>
                <span class="log-type" style="color: ${color}; min-width: 80px; display: inline-block; text-transform: uppercase; font-size: 0.75rem; font-weight: 600;">
                    ${log.type}
                </span>
                <span class="log-message" style="color: var(--text-primary);">
                    ${UI.escapeHtml(log.message)}
                </span>
            </div>
        `;
  },

  /**
   * Clear all logs
   */
  async clearLogs() {
    const confirmed = await UI.confirm(
      "Are you sure you want to clear all log entries?",
      {
        title: "Clear Logs",
        confirmText: "Clear",
        danger: true,
      }
    );

    if (confirmed) {
      this._logs = [];
      this.renderLogs();
      UI.success("Logs cleared");
    }
  },

  /**
   * Export logs
   */
  exportLogs() {
    const logs = this.getFilteredLogs();

    if (logs.length === 0) {
      UI.warning("No logs to export");
      return;
    }

    // Format logs as text
    const lines = logs.map((log) => {
      const time = log.time.toISOString();
      return `[${time}] [${log.type.toUpperCase()}] ${log.message}`;
    });

    const content = lines.join("\n");
    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);

    const a = document.createElement("a");
    a.href = url;
    a.download = `gocast-logs-${new Date().toISOString().split("T")[0]}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    UI.success(`Exported ${logs.length} log entries`);
  },
};

// Export for use in app
window.LogsPage = LogsPage;
