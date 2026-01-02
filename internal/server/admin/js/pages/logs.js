/**
 * GoCast Admin - Logs Page
 * Real-time server log viewer (stdout/stderr from server)
 */

const LogsPage = {
  // Log entries
  _logs: [],

  // Max log entries to keep
  _maxLogs: 1000,

  // Current filter
  _filter: "all",

  // Auto-scroll enabled
  _autoScroll: true,

  // Last log ID received
  _lastLogId: 0,

  /**
   * Render the logs page
   */
  render() {
    return `
            <div class="flex justify-between items-center mb-3">
                <div class="flex gap-2 items-center">
                    <label class="form-label" style="margin: 0;">Filter:</label>
                    <select id="logFilter" class="form-select" style="width: 160px;" onchange="LogsPage.setFilter(this.value)">
                        <option value="all">All Logs</option>
                        <option value="info">Info</option>
                        <option value="warn">Warnings</option>
                        <option value="error">Errors</option>
                        <option value="debug">Debug</option>
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
    this._lastLogId = 0;

    // Subscribe to real-time log events from server
    API.on("log", (data) => this.handleLogEvent(data));
    API.on("log_history", (data) => this.handleLogHistory(data));

    // Load initial logs from server
    await this.loadServerLogs();

    // Add initial entry
    this.addLog("info", "server", "Log viewer connected");
  },

  /**
   * Clean up when leaving page
   */
  destroy() {
    // Nothing to clean up
  },

  /**
   * Load logs from server API
   */
  async loadServerLogs() {
    try {
      const response = await API.get("/logs?count=200");
      if (response.success && response.data) {
        // Add entries (they come in chronological order, oldest first after GetRecent)
        response.data.forEach((entry) => {
          this.addLogFromServer(entry);
        });
      }
    } catch (err) {
      console.error("Failed to load logs:", err);
      this.addLog(
        "error",
        "logs",
        "Failed to load server logs: " + err.message,
      );
    }
  },

  /**
   * Handle log event from SSE
   */
  handleLogEvent(data) {
    this.addLogFromServer(data);
  },

  /**
   * Handle log history from SSE on connect
   */
  handleLogHistory(data) {
    if (data.entries && Array.isArray(data.entries)) {
      data.entries.forEach((entry) => {
        this.addLogFromServer(entry);
      });
    }
  },

  /**
   * Add log entry from server data
   */
  addLogFromServer(entry) {
    // Skip if we've already seen this log
    if (entry.id && entry.id <= this._lastLogId) {
      return;
    }
    if (entry.id) {
      this._lastLogId = entry.id;
    }

    const logEntry = {
      id: entry.id || Date.now() + Math.random(),
      time: entry.timestamp ? new Date(entry.timestamp) : new Date(),
      type: entry.level || "info",
      source: entry.source || "server",
      message: entry.message || "",
    };

    this._logs.unshift(logEntry);

    // Trim to max size
    if (this._logs.length > this._maxLogs) {
      this._logs.length = this._maxLogs;
    }

    this.renderLogs();
  },

  /**
   * Add a log entry (for local logs like "Log viewer connected")
   */
  addLog(type, source, message) {
    const entry = {
      id: Date.now() + Math.random(),
      time: new Date(),
      type: type,
      source: source,
      message: message,
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
      debug: "var(--text-muted)",
      info: "var(--info)",
      warn: "var(--warning)",
      error: "var(--error)",
    };

    const typeIcons = {
      debug: "🔍",
      info: "ℹ️",
      warn: "⚠️",
      error: "❌",
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
                <span class="log-type" style="color: ${color}; min-width: 60px; display: inline-block; text-transform: uppercase; font-size: 0.75rem; font-weight: 600;">
                    ${log.type}
                </span>
                <span class="log-source" style="color: var(--accent-primary); min-width: 80px; display: inline-block; font-size: 0.75rem;">
                    [${UI.escapeHtml(log.source)}]
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
      },
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
