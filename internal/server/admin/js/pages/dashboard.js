/**
 * GoCast Admin - Dashboard Page
 * Shows server stats, active streams, and real-time metrics
 */

const DashboardPage = {
    // Update interval
    _interval: null,

    // Previous stats for change detection
    _prevStats: null,

    // Bandwidth tracking - use 10-second moving average to smooth bursty data
    _bandwidthHistory: [], // [{bytes, time}, ...]
    _lastDisplayedBandwidth: 0,

    /**
     * Render the dashboard page
     */
    render() {
        return `
            <div class="grid grid-5 mb-3">
                <div class="stat-card" id="statListeners">
                    <div class="stat-header">
                        <span class="stat-icon">👥</span>
                        <span class="stat-change up" id="listenerChange"></span>
                    </div>
                    <div class="stat-value" id="totalListeners">0</div>
                    <div class="stat-label">Current Listeners</div>
                </div>

                <div class="stat-card" id="statStreams">
                    <div class="stat-header">
                        <span class="stat-icon">📡</span>
                    </div>
                    <div class="stat-value"><span id="activeStreams">0</span> / <span id="totalMounts">0</span></div>
                    <div class="stat-label">Active Streams</div>
                </div>

                <div class="stat-card" id="statBandwidth">
                    <div class="stat-header">
                        <span class="stat-icon">📶</span>
                    </div>
                    <div class="stat-value" id="totalBandwidth">0 KB/s</div>
                    <div class="stat-label">Total Bandwidth</div>
                </div>

                <div class="stat-card" id="statPeak">
                    <div class="stat-header">
                        <span class="stat-icon">📊</span>
                    </div>
                    <div class="stat-value" id="peakListeners">0</div>
                    <div class="stat-label">Peak Listeners</div>
                </div>

                <div class="stat-card" id="statUptime">
                    <div class="stat-header">
                        <span class="stat-icon">⏱️</span>
                    </div>
                    <div class="stat-value" id="serverUptime">--:--:--</div>
                    <div class="stat-label">Server Uptime</div>
                </div>
            </div>

            <div class="grid grid-2">
                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">📻 Active Streams</h3>
                        <span class="badge badge-success" id="liveCount">0 Live</span>
                    </div>
                    <div class="card-body" id="streamsList">
                        <div class="empty-state">
                            <div class="empty-icon">📡</div>
                            <div class="empty-title">No Active Streams</div>
                            <div class="empty-text">Start streaming to see activity here</div>
                        </div>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">📋 Recent Activity</h3>
                        <button class="btn btn-sm btn-secondary" onclick="DashboardPage.clearActivity()">Clear</button>
                    </div>
                    <div class="card-body" id="activityList" style="max-height: 400px; overflow-y: auto;">
                        <div class="empty-state">
                            <div class="empty-icon">📋</div>
                            <div class="empty-title">No Recent Activity</div>
                            <div class="empty-text">Events will appear here</div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card mt-3">
                <div class="card-header">
                    <h3 class="card-title">🖥️ Server Health</h3>
                </div>
                <div class="card-body">
                    <div class="grid grid-4">
                        <div class="health-item">
                            <div class="flex justify-between mb-1">
                                <span class="text-muted">Connections</span>
                                <span id="connValue">0 / 100</span>
                            </div>
                            <div id="connProgress">${UI.progressBar(0, "")}</div>
                        </div>

                        <div class="health-item">
                            <div class="flex justify-between mb-1">
                                <span class="text-muted">Sources</span>
                                <span id="srcValue">0 / 10</span>
                            </div>
                            <div id="srcProgress">${UI.progressBar(0, "")}</div>
                        </div>

                        <div class="health-item">
                            <div class="flex justify-between mb-1">
                                <span class="text-muted">Memory</span>
                                <span id="memValue">--</span>
                            </div>
                            <div id="memProgress">${UI.progressBar(0, "")}</div>
                        </div>

                        <div class="health-item">
                            <div class="flex justify-between mb-1">
                                <span class="text-muted">Buffer Health</span>
                                <span id="bufValue">100%</span>
                            </div>
                            <div id="bufProgress">${UI.progressBar(100, "success")}</div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    },

    /**
     * Initialize the dashboard
     */
    async init() {
        this._prevStats = null;

        // Start periodic updates
        this.update();
        this._interval = setInterval(() => this.update(), 2000);

        // Subscribe to real-time events
        API.on("stats", (data) => this.handleStatsUpdate(data));
        API.on("listener", (data) => this.handleListenerEvent(data));
        API.on("source", (data) => this.handleSourceEvent(data));

        // Subscribe to activity events from server
        API.on("activity", (data) => this.handleActivityEvent(data));
        API.on("activity_history", (data) => this.handleActivityHistory(data));

        // Load initial activity from server
        this.loadRecentActivity();
    },

    /**
     * Clean up when leaving page
     */
    destroy() {
        if (this._interval) {
            clearInterval(this._interval);
            this._interval = null;
        }
        this._prevStats = null;
        this._bandwidthHistory = [];
        this._lastDisplayedBandwidth = 0;
    },

    /**
     * Update dashboard data
     */
    async update() {
        try {
            const status = await API.getStatus();
            this.updateStats(status);
            this.updateStreamsList(status.mounts || []);
        } catch (err) {
            console.error("Dashboard update error:", err);
        }
    },

    /**
     * Update statistics display
     */
    updateStats(status) {
        const mounts = status.mounts || [];

        // Calculate totals
        let totalListeners = 0;
        let peakListeners = 0;
        let activeStreams = 0;

        mounts.forEach((mount) => {
            totalListeners += mount.listeners || 0;
            peakListeners = Math.max(peakListeners, mount.peak || 0);
            if (mount.active) activeStreams++;
        });

        // Update display
        const listenersEl = UI.$("totalListeners");
        if (listenersEl) listenersEl.textContent = totalListeners;

        const activeEl = UI.$("activeStreams");
        if (activeEl) activeEl.textContent = activeStreams;

        const totalMountsEl = UI.$("totalMounts");
        if (totalMountsEl) totalMountsEl.textContent = mounts.length;

        const peakEl = UI.$("peakListeners");
        if (peakEl) peakEl.textContent = peakListeners;

        // Update bandwidth display - use 10-second moving average to smooth bursty data
        const bandwidthEl = UI.$("totalBandwidth");
        if (bandwidthEl) {
            const currentBytes = status.total_bytes_sent || 0;
            const currentTime = Date.now();

            // Add current sample to history
            this._bandwidthHistory.push({
                bytes: currentBytes,
                time: currentTime,
            });

            // Keep only last 10 seconds of samples
            const cutoffTime = currentTime - 10000;
            this._bandwidthHistory = this._bandwidthHistory.filter(
                (s) => s.time > cutoffTime,
            );

            // Calculate bandwidth from oldest to newest sample in window
            let bandwidth = 0;
            if (this._bandwidthHistory.length >= 2) {
                const oldest = this._bandwidthHistory[0];
                const newest =
                    this._bandwidthHistory[this._bandwidthHistory.length - 1];
                const bytesDelta = newest.bytes - oldest.bytes;
                const timeDelta = (newest.time - oldest.time) / 1000;
                if (bytesDelta > 0 && timeDelta > 0.5) {
                    bandwidth = bytesDelta / timeDelta;
                }
            }

            // Update display - use last known value if current is 0 but we have listeners
            if (bandwidth > 0) {
                this._lastDisplayedBandwidth = bandwidth;
                bandwidthEl.textContent = this.formatBandwidth(bandwidth);
            } else if (totalListeners === 0) {
                this._lastDisplayedBandwidth = 0;
                bandwidthEl.textContent = this.formatBandwidth(0);
            } else if (this._lastDisplayedBandwidth > 0) {
                // Keep showing last value briefly while data is bursty
                bandwidthEl.textContent = this.formatBandwidth(
                    this._lastDisplayedBandwidth,
                );
            }
        }

        const liveCountEl = UI.$("liveCount");
        if (liveCountEl) {
            liveCountEl.textContent = `${activeStreams} Live`;
            liveCountEl.className = `badge ${activeStreams > 0 ? "badge-success" : "badge-neutral"}`;
        }

        // Update health indicators
        this.updateHealthIndicators(status, mounts);
    },

    /**
     * Update health progress bars
     */
    /**
     * Format bytes per second into human-readable bandwidth
     */
    formatBandwidth(bytesPerSec) {
        if (bytesPerSec < 1024) {
            return bytesPerSec + " B/s";
        } else if (bytesPerSec < 1024 * 1024) {
            return (bytesPerSec / 1024).toFixed(1) + " KB/s";
        } else {
            return (bytesPerSec / (1024 * 1024)).toFixed(2) + " MB/s";
        }
    },

    updateHealthIndicators(status, mounts) {
        const config = State.get("config.limits") || {};
        const maxClients = config.max_clients || 100;
        const maxSources = config.max_sources || 10;

        let totalListeners = 0;
        let activeSources = 0;

        mounts.forEach((mount) => {
            totalListeners += mount.listeners || 0;
            if (mount.active) activeSources++;
        });

        // Connections
        const connPercent = (totalListeners / maxClients) * 100;
        const connValue = UI.$("connValue");
        const connProgress = UI.$("connProgress");
        if (connValue)
            connValue.textContent = `${totalListeners} / ${maxClients}`;
        if (connProgress) {
            connProgress.innerHTML = UI.progressBar(
                connPercent,
                connPercent > 80 ? "warning" : "",
            );
        }

        // Sources
        const srcPercent = (activeSources / maxSources) * 100;
        const srcValue = UI.$("srcValue");
        const srcProgress = UI.$("srcProgress");
        if (srcValue) srcValue.textContent = `${activeSources} / ${maxSources}`;
        if (srcProgress) {
            srcProgress.innerHTML = UI.progressBar(
                srcPercent,
                srcPercent > 80 ? "warning" : "",
            );
        }
    },

    /**
     * Update streams list
     */
    updateStreamsList(mounts) {
        const container = UI.$("streamsList");
        if (!container) return;

        if (mounts.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📡</div>
                    <div class="empty-title">No Streams Configured</div>
                    <div class="empty-text">Add mount points in the Mounts section</div>
                </div>
            `;
            return;
        }

        const activeFirst = [...mounts].sort((a, b) => {
            // Live streams always first
            if (a.active && !b.active) return -1;
            if (!a.active && b.active) return 1;
            // Then by listener count (descending)
            const listenerDiff = (b.listeners || 0) - (a.listeners || 0);
            if (listenerDiff !== 0) return listenerDiff;
            // Stable sort by path for consistent ordering
            return (a.path || "").localeCompare(b.path || "");
        });

        container.innerHTML = activeFirst
            .map(
                (mount) => `
            <div class="stream-card ${mount.active ? "live" : ""}" style="margin-bottom: 12px;">
                <div class="stream-header">
                    <div class="stream-info">
                        <div class="stream-icon">${mount.active ? "🔴" : "⚫"}</div>
                        <div>
                            <div class="stream-name">${UI.escapeHtml(mount.name || mount.path)}</div>
                            <div class="stream-path">${UI.escapeHtml(mount.path)}</div>
                        </div>
                    </div>
                    ${UI.badge(mount.active ? "LIVE" : "OFFLINE", mount.active ? "success" : "neutral")}
                </div>
                <div class="stream-body">
                    <div class="stream-meta">
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Listeners</span>
                            <span class="stream-meta-value">${mount.listeners || 0}</span>
                        </div>
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Peak</span>
                            <span class="stream-meta-value">${mount.peak || 0}</span>
                        </div>
                        <div class="stream-meta-item">
                            <span class="stream-meta-label">Bitrate</span>
                            <span class="stream-meta-value">${mount.bitrate || 128} kbps</span>
                        </div>
                    </div>
                    ${
                        mount.active
                            ? `
                    <div class="stream-nowplaying" style="margin-top: 8px; padding: 8px; background: rgba(0,0,0,0.2); border-radius: 4px;">
                        <span style="color: #888; font-size: 11px;">🎵 Now Playing:</span>
                        <div style="color: #fff; font-size: 13px; margin-top: 2px;">${mount.metadata && mount.metadata.stream_title ? UI.escapeHtml(mount.metadata.stream_title) : '<span style="color: #666; font-style: italic;">No song info</span>'}</div>
                    </div>
                    `
                            : ""
                    }
                </div>
                ${
                    mount.active
                        ? `
                <div class="stream-footer">
                    <button class="btn btn-sm btn-secondary" onclick="DashboardPage.viewListeners('${mount.path}')">
                        👥 Listeners
                    </button>
                    <button class="btn btn-sm btn-danger" onclick="DashboardPage.killSource('${mount.path}')">
                        ⏹️ Stop
                    </button>
                </div>
                `
                        : ""
                }
            </div>
        `,
            )
            .join("");
    },

    /**
     * Handle real-time stats update
     */
    handleStatsUpdate(data) {
        this.updateStats(data);
        if (data.mounts) {
            this.updateStreamsList(data.mounts);
        }
    },

    /**
     * Load recent activity from server API
     */
    async loadRecentActivity() {
        try {
            const response = await API.get("/activity?count=20");
            if (response.success && response.data) {
                // Sort by ID ascending (oldest first), then append each
                // This puts oldest at bottom, newest at top
                const entries = response.data.sort((a, b) => a.id - b.id);
                entries.forEach((entry) => {
                    this.addActivityFromServer(entry, true);
                });
            }
        } catch (err) {
            console.error("Failed to load activity:", err);
        }
    },

    /**
     * Handle activity event from SSE
     */
    handleActivityEvent(data) {
        this.addActivityFromServer(data);
    },

    /**
     * Handle activity history from SSE on connect
     */
    handleActivityHistory(data) {
        if (data.entries && Array.isArray(data.entries)) {
            // Clear existing and add history
            const container = UI.$("activityList");
            if (container) {
                container.innerHTML = "";
            }
            // Sort by ID ascending (oldest first), prepend each so newest ends up on top
            const sorted = data.entries.sort((a, b) => a.id - b.id);
            sorted.forEach((entry) => {
                this.addActivityFromServer(entry, true);
            });
        }
    },

    /**
     * Add activity entry from server data
     * @param {boolean} prepend - if true, add to top; if false, add to bottom
     */
    addActivityFromServer(entry, prepend = true) {
        const typeMap = {
            listener_connect: "connect",
            listener_disconnect: "disconnect",
            listener_summary: "info",
            source_start: "source",
            source_stop: "source",
            config_change: "config",
            mount_create: "config",
            mount_delete: "config",
            server_start: "info",
            server_stop: "error",
            admin_action: "config",
        };

        const type = typeMap[entry.type] || "info";
        const time = entry.timestamp ? new Date(entry.timestamp) : new Date();

        this.addActivity(type, entry.message, time, prepend);
    },

    /**
     * Handle listener event
     */
    handleListenerEvent(data) {
        const action = data.action || "connected";
        const mount = data.mount || "unknown";
        const message = `Listener ${action} on ${mount}`;

        this.addActivity(
            action === "disconnected" ? "disconnect" : "connect",
            message,
        );
    },

    /**
     * Handle source event
     */
    handleSourceEvent(data) {
        const action = data.action || "started";
        const mount = data.mount || "unknown";
        const message = `Source ${action} on ${mount}`;

        this.addActivity("source", message);

        // Refresh streams
        this.update();
    },

    /**
     * Add activity entry
     * @param {boolean} prepend - if true, add to top; if false, add to bottom
     */
    addActivity(type, message, time = null, prepend = true) {
        const container = UI.$("activityList");
        if (!container) return;

        // Remove empty state if present
        const emptyState = container.querySelector(".empty-state");
        if (emptyState) {
            container.innerHTML = "";
        }

        const typeIcons = {
            connect: "🟢",
            disconnect: "🔴",
            source: "📡",
            config: "⚙️",
            error: "⚠️",
            info: "ℹ️",
        };

        const displayTime = time || new Date();

        const entry = document.createElement("div");
        entry.className = "log-entry";
        entry.innerHTML = `
            <span class="log-time">${UI.formatTime(displayTime)}</span>
            <span class="log-type">${typeIcons[type] || "ℹ️"}</span>
            <span class="log-message">${UI.escapeHtml(message)}</span>
        `;

        if (prepend) {
            container.insertBefore(entry, container.firstChild);
        } else {
            container.appendChild(entry);
        }

        // Keep only last 50 entries
        while (container.children.length > 50) {
            container.removeChild(container.lastChild);
        }
    },

    /**
     * Clear activity log
     */
    clearActivity() {
        const container = UI.$("activityList");
        if (container) {
            container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📋</div>
                    <div class="empty-title">No Recent Activity</div>
                    <div class="empty-text">Events will appear here</div>
                </div>
            `;
        }
    },

    /**
     * View listeners for a mount
     */
    async viewListeners(mountPath) {
        try {
            const response = await API.listClients(mountPath);
            // Navigate to listeners page with mount filter
            App.navigateTo("listeners", { mount: mountPath });
        } catch (err) {
            UI.error("Failed to load listeners: " + err.message);
        }
    },

    /**
     * Kill source on a mount
     */
    async killSource(mountPath) {
        const confirmed = await UI.confirm(
            `Are you sure you want to stop the source on ${mountPath}? This will disconnect the broadcaster.`,
            {
                title: "Stop Source",
                confirmText: "Stop Source",
                danger: true,
            },
        );

        if (confirmed) {
            try {
                await API.killSource(mountPath);
                UI.success(`Source stopped on ${mountPath}`);
                this.update();
            } catch (err) {
                UI.error("Failed to stop source: " + err.message);
            }
        }
    },
};

// Export for use in app
window.DashboardPage = DashboardPage;
