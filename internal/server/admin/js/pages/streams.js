/**
 * GoCast Admin - Streams Page
 * Live stream monitoring and management
 */

const StreamsPage = {
  // Update interval
  _interval: null,

  /**
   * Render the streams page
   */
  render() {
    return `
            <div class="flex justify-between items-center mb-3">
                <div class="flex gap-2">
                    <button class="btn btn-secondary ${this._filter === "all" ? "active" : ""}" onclick="StreamsPage.setFilter('all')">
                        All Streams
                    </button>
                    <button class="btn btn-secondary ${this._filter === "live" ? "active" : ""}" onclick="StreamsPage.setFilter('live')">
                        🔴 Live Only
                    </button>
                </div>
                <button class="btn btn-icon" onclick="StreamsPage.refresh()" title="Refresh">
                    <span>🔄</span>
                </button>
            </div>

            <div id="streamsContainer">
                <div class="loading">
                    <div class="spinner"></div>
                    <p>Loading streams...</p>
                </div>
            </div>
        `;
  },

  // Current filter
  _filter: "all",

  /**
   * Initialize the page
   */
  async init() {
    this._filter = "all";
    await this.refresh();
    this._interval = setInterval(() => this.refresh(), 3000);

    // Subscribe to real-time events
    API.on("source", () => this.refresh());
    API.on("stats", (data) => this.handleStatsUpdate(data));
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
   * Set filter mode
   */
  setFilter(filter) {
    this._filter = filter;
    this.refresh();
  },

  /**
   * Refresh streams data
   */
  async refresh() {
    try {
      const status = await API.getStatus();
      this.renderStreams(status.mounts || []);
    } catch (err) {
      console.error("Streams refresh error:", err);
      UI.error("Failed to load streams");
    }
  },

  /**
   * Handle real-time stats update
   */
  handleStatsUpdate(data) {
    if (data.mounts) {
      this.renderStreams(data.mounts);
    }
  },

  /**
   * Render streams list
   */
  renderStreams(mounts) {
    const container = UI.$("streamsContainer");
    if (!container) return;

    // Apply filter
    let filtered = mounts;
    if (this._filter === "live") {
      filtered = mounts.filter((m) => m.active);
    }

    // Sort: active first, then by listeners, then by path (for stable ordering)
    filtered.sort((a, b) => {
      if (a.active && !b.active) return -1;
      if (!a.active && b.active) return 1;
      const listenerDiff = (b.listeners || 0) - (a.listeners || 0);
      if (listenerDiff !== 0) return listenerDiff;
      // Stable sort by path when everything else is equal
      return (a.path || "").localeCompare(b.path || "");
    });

    if (filtered.length === 0) {
      container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">${this._filter === "live" ? "🔴" : "📡"}</div>
                    <div class="empty-title">${this._filter === "live" ? "No Live Streams" : "No Streams Found"}</div>
                    <div class="empty-text">
                        ${this._filter === "live" ? "No streams are currently broadcasting" : "Configure mount points to enable streaming"}
                    </div>
                </div>
            `;
      return;
    }

    container.innerHTML = `
            <div class="grid grid-2">
                ${filtered.map((mount) => this.renderStreamCard(mount)).join("")}
            </div>
        `;
  },

  /**
   * Render a single stream card
   */
  renderStreamCard(mount) {
    const isLive = mount.active;
    const listeners = mount.listeners || 0;
    const peak = mount.peak || 0;
    const bitrate = mount.bitrate || 128;
    const genre = mount.genre || "Unknown";
    const name = mount.name || mount.path;

    return `
            <div class="stream-card ${isLive ? "live" : ""}">
                <div class="stream-header">
                    <div class="stream-info">
                        <div class="stream-icon">${isLive ? "🔴" : "⚫"}</div>
                        <div>
                            <div class="stream-name">${UI.escapeHtml(name)}</div>
                            <div class="stream-path">${UI.escapeHtml(mount.path)}</div>
                        </div>
                    </div>
                    ${UI.badge(isLive ? "LIVE" : "OFFLINE", isLive ? "success" : "neutral")}
                </div>

                <div class="stream-body">
                    <div class="stream-meta">
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Listeners</span>
                            <span class="stream-meta-value">${listeners}</span>
                        </div>
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Peak</span>
                            <span class="stream-meta-value">${peak}</span>
                        </div>
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Bitrate</span>
                            <span class="stream-meta-value">${bitrate} kbps</span>
                        </div>
                    </div>

                    ${
                      isLive
                        ? `
                    <div class="mt-2">
                        <div class="flex justify-between mb-1">
                            <span class="text-muted">Genre</span>
                            <span>${UI.escapeHtml(genre)}</span>
                        </div>
                    </div>
                    `
                        : ""
                    }
                </div>

                <div class="stream-footer">
                    <button class="btn btn-sm btn-secondary" onclick="StreamsPage.copyStreamUrl('${mount.path}')">
                        📋 Copy URL
                    </button>
                    ${
                      isLive
                        ? `
                        <button class="btn btn-sm btn-secondary" onclick="StreamsPage.viewListeners('${mount.path}')">
                            👥 ${listeners} Listeners
                        </button>
                        <button class="btn btn-sm btn-secondary" onclick="StreamsPage.listenToStream('${mount.path}')">
                            🎧 Listen
                        </button>
                        <button class="btn btn-sm btn-danger" onclick="StreamsPage.killSource('${mount.path}')">
                            ⏹️ Stop
                        </button>
                    `
                        : `
                        <button class="btn btn-sm btn-secondary" onclick="StreamsPage.editMount('${mount.path}')" disabled>
                            ⚙️ Configure
                        </button>
                    `
                    }
                </div>
            </div>
        `;
  },

  /**
   * Copy stream URL to clipboard
   */
  copyStreamUrl(path) {
    const url = `${window.location.origin}${path}`;
    navigator.clipboard
      .writeText(url)
      .then(() => {
        UI.success("Stream URL copied to clipboard");
      })
      .catch(() => {
        UI.error("Failed to copy URL");
      });
  },

  /**
   * Open stream in new tab for listening
   */
  listenToStream(path) {
    window.open(path, "_blank");
  },

  /**
   * View listeners for a stream
   */
  viewListeners(path) {
    App.navigateTo("listeners", { mount: path });
  },

  /**
   * Navigate to mount config
   */
  editMount(path) {
    App.navigateTo("mounts", { edit: path });
  },

  /**
   * Stop a source
   */
  async killSource(path) {
    const confirmed = await UI.confirm(
      `Are you sure you want to stop the source on ${path}? This will disconnect the broadcaster and all listeners.`,
      {
        title: "Stop Source",
        confirmText: "Stop Source",
        danger: true,
      },
    );

    if (confirmed) {
      try {
        await API.killSource(path);
        UI.success(`Source stopped on ${path}`);
        await this.refresh();
      } catch (err) {
        UI.error("Failed to stop source: " + err.message);
      }
    }
  },
};

// Export for use in app
window.StreamsPage = StreamsPage;
