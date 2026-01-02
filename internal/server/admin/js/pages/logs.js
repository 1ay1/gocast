/**
 * GoCast Admin - Logs Page
 * Real-time server log viewer (stdout/stderr from server)
 */

const LogsPage = {
  // Log entries (stored newest first)
  _logs: [],

  // Max log entries to keep
  _maxLogs: 1000,

  // Current filter
  _filter: "all",

  // Auto-scroll enabled
  _autoScroll: true,

  // Set of log IDs we've seen (for deduplication)
  _seenIds: new Set(),

  // Loading state
  _loading: false,

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
    this._seenIds = new Set();
    this._loading = true;

    // Subscribe to real-time log events from server
    API.on("log", (data) => this.handleLogEvent(data));
    API.on("log_history", (data) => this.handleLogHistory(data));

    // Load initial logs from server
    await this.loadServerLogs();

    this._loading = false;
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
      const response = await API.get("/logs?count=500");
      if (response.success && response.data && response.data.length > 0) {
        // Sort by ID ascending (oldest first), then add each
        const sorted = response.data.sort((a, b) => a.id - b.id);
        sorted.forEach((entry) => {
          this.addLogFromServer(entry, false); // Don't re-render each time
        });
        this.renderLogs(); // Render once at the end
      }
    } catch (err) {
      console.error("Failed to load logs:", err);
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
      // Sort by ID ascending (oldest first)
      const sorted = data.entries.sort((a, b) => a.id - b.id);
      let added = false;
      sorted.forEach((entry) => {
        if (this.addLogFromServer(entry, false)) {
          added = true;
        }
      });
      if (added) {
        this.renderLogs();
      }
    }
  },

  /**
   * Add log entry from server data
   * @param {Object} entry - The log entry from server
   * @param {boolean} render - Whether to re-render after adding
   * @returns {boolean} - Whether the entry was added (false if duplicate)
   */
  addLogFromServer(entry, render = true) {
    // Skip if we've already seen this log ID
    const id = entry.id || 0;
    if (id && this._seenIds.has(id)) {
      return false;
    }
    if (id) {
      this._seenIds.add(id);
    }

    const logEntry = {
      id: id || Date.now() + Math.random(),
      time: entry.timestamp ? new Date(entry.timestamp) : new Date(),
      type: entry.level || "info",
      source: entry.source || "server",
      message: entry.message || "",
    };

    // Insert at beginning (newest first)
    this._logs.unshift(logEntry);

    // Trim to max size and clean up seenIds for removed entries
    if (this._logs.length > this._maxLogs) {
      const removed = this._logs.splice(this._maxLogs);
      removed.forEach((r) => {
        if (r.id) this._seenIds.delete(r.id);
      });
    }

    if (render) {
      this.renderLogs();
    }

    return true;
  },

  /**
   * Add a local log entry
   */
  addLog(type, source, message) {
    const localId = Date.now() + Math.random();
    const entry = {
      id: localId,
      time: new Date(),
      type: type,
      source: source,
      message: message,
    };

    this._logs.unshift(entry);

    // Trim to max size
    if (this._logs.length > this._maxLogs) {
      this._logs.pop();
    }

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

    // Get filtered logs - already sorted newest first
    const logs = this.getFilteredLogs();

    // Update badge with total count
    const badge = UI.$("logCountBadge");
    if (badge) {
      const total = this._logs.length;
      const filtered = logs.length;
      badge.textContent =
        this._filter === "all"
          ? `${total} entries`
          : `${filtered} of ${total} entries`;
    }

    if (logs.length === 0) {
      container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📋</div>
                    <div class="empty-title">${this._loading ? "Loading logs..." : "No Log Entries"}</div>
                    <div class="empty-text">${this._filter === "all" ? "Server logs will appear here in real-time" : "No entries match the current filter"}</div>
                </div>
            `;
      return;
    }

    // Render log entries (logs are already newest first)
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
