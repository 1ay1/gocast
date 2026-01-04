/**
 * GoCast Admin - Listeners Page
 * View and manage connected listeners across all mounts
 *
 * ANTI-FLICKER DESIGN:
 * 1. Smart DOM updates - only update changed elements
 * 2. Throttled refresh to prevent rapid re-renders
 * 3. Cache comparison before DOM manipulation
 */

const ListenersPage = {
    // Current filter
    _mountFilter: null,

    // Update interval
    _interval: null,

    // Cached data
    _streams: [],
    _listeners: [],

    // Last data signature to detect changes
    _lastSignature: null,

    // Throttled refresh function
    _throttledRefresh: null,

    /**
     * Render the listeners page
     */
    render() {
        return `
            <div class="flex justify-between items-center mb-3">
                <div class="flex gap-2 items-center">
                    <label class="form-label" style="margin: 0;">Filter by Mount:</label>
                    <select id="mountFilter" class="form-select" style="width: 200px;" onchange="ListenersPage.setMountFilter(this.value)">
                        <option value="">All Mounts</option>
                    </select>
                </div>
                <div class="flex gap-2">
                    <button class="btn btn-secondary" onclick="ListenersPage.refresh()">
                        🔄 Refresh
                    </button>
                    <button class="btn btn-danger" onclick="ListenersPage.kickAll()" id="kickAllBtn" disabled>
                        ⚠️ Kick All
                    </button>
                </div>
            </div>

            <div class="grid grid-4 mb-3">
                <div class="stat-card">
                    <div class="stat-header">
                        <span class="stat-icon">👥</span>
                    </div>
                    <div class="stat-value" id="totalListenerCount">0</div>
                    <div class="stat-label">Total Listeners</div>
                </div>

                <div class="stat-card">
                    <div class="stat-header">
                        <span class="stat-icon">📡</span>
                    </div>
                    <div class="stat-value" id="activeMountCount">0</div>
                    <div class="stat-label">Active Mounts</div>
                </div>

                <div class="stat-card">
                    <div class="stat-header">
                        <span class="stat-icon">📊</span>
                    </div>
                    <div class="stat-value" id="avgListenTime">--:--</div>
                    <div class="stat-label">Avg. Listen Time</div>
                </div>

                <div class="stat-card">
                    <div class="stat-header">
                        <span class="stat-icon">🌐</span>
                    </div>
                    <div class="stat-value" id="uniqueIPs">0</div>
                    <div class="stat-label">Unique IPs</div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">👥 Connected Listeners</h3>
                    <span class="badge badge-neutral" id="listenerCountBadge">0</span>
                </div>
                <div class="card-body" id="listenersContainer">
                    <div class="loading">
                        <div class="spinner"></div>
                        <p>Loading listeners...</p>
                    </div>
                </div>
            </div>
        `;
    },

    /**
     * Initialize the page
     */
    async init() {
        // Check for mount filter from navigation params
        const params = App.getPageParams();
        if (params && params.mount) {
            this._mountFilter = params.mount;
        }

        // Create throttled refresh to prevent rapid updates
        this._throttledRefresh = UI.throttle(() => this.refresh(), 2000);

        await this.refresh();
        this._interval = setInterval(() => this._throttledRefresh(), 5000);

        // Subscribe to real-time events (throttled)
        API.on("listener", () => this._throttledRefresh());
    },

    /**
     * Clean up when leaving page
     */
    destroy() {
        if (this._interval) {
            clearInterval(this._interval);
            this._interval = null;
        }
        this._mountFilter = null;
        this._lastSignature = null;
        this._throttledRefresh = null;
    },

    /**
     * Set mount filter
     */
    setMountFilter(mount) {
        this._mountFilter = mount || null;
        this.renderListeners();
    },

    /**
     * Refresh data
     */
    async refresh() {
        try {
            // Get status for mount list
            const status = await API.getStatus();
            this._streams = status.mounts || [];

            // Update mount filter dropdown
            this.updateMountDropdown();

            // Fetch listeners for each active mount
            await this.fetchAllListeners();

            // Update stats
            this.updateStats();

            // Render listeners table
            this.renderListeners();
        } catch (err) {
            console.error("Listeners refresh error:", err);
        }
    },

    /**
     * Update mount dropdown options
     */
    updateMountDropdown() {
        const select = UI.$("mountFilter");
        if (!select) return;

        const currentValue = select.value;

        // Build options
        let options = '<option value="">All Mounts</option>';
        this._streams.forEach((stream) => {
            const selected =
                stream.path === this._mountFilter ? "selected" : "";
            const listeners = stream.listeners || 0;
            options += `<option value="${stream.path}" ${selected}>${stream.path} (${listeners})</option>`;
        });

        select.innerHTML = options;

        // Restore selection
        if (this._mountFilter) {
            select.value = this._mountFilter;
        }
    },

    /**
     * Fetch listeners from all active mounts
     */
    async fetchAllListeners() {
        this._listeners = [];

        const activeMounts = this._streams.filter((s) => s.active);

        for (const mount of activeMounts) {
            try {
                const response = await API.listClients(mount.path);
                // Parse XML response or handle JSON
                const listeners = this.parseListenersResponse(
                    response,
                    mount.path,
                );
                this._listeners.push(...listeners);
            } catch (err) {
                console.error(
                    `Error fetching listeners for ${mount.path}:`,
                    err,
                );
            }
        }
    },

    /**
     * Parse listeners response (handles both XML and JSON)
     */
    parseListenersResponse(response, mountPath) {
        // If it's already an array, use it directly
        if (Array.isArray(response)) {
            return response.map((l) => ({ ...l, mount: mountPath }));
        }

        // If it's an object with listeners property (new JSON format)
        if (
            response &&
            response.listeners &&
            Array.isArray(response.listeners)
        ) {
            return response.listeners.map((l) => ({
                id: l.id,
                ip: l.ip,
                userAgent: l.user_agent,
                connected: String(l.connected),
                connections: l.connections || 1,
                ids: l.ids || [l.id],
                mount: mountPath,
            }));
        }

        // If it's an object with data property
        if (response && response.data && Array.isArray(response.data)) {
            return response.data.map((l) => ({ ...l, mount: mountPath }));
        }

        // Try to parse XML string
        if (typeof response === "string" && response.includes("<")) {
            return this.parseXMLListeners(response, mountPath);
        }

        return [];
    },

    /**
     * Parse XML listeners response
     */
    parseXMLListeners(xml, mountPath) {
        const listeners = [];
        const parser = new DOMParser();
        const doc = parser.parseFromString(xml, "text/xml");
        const clients = doc.querySelectorAll("listener");

        clients.forEach((client) => {
            listeners.push({
                id:
                    client.querySelector("ID")?.textContent ||
                    client.querySelector("id")?.textContent ||
                    "",
                ip:
                    client.querySelector("IP")?.textContent ||
                    client.querySelector("ip")?.textContent ||
                    "",
                userAgent:
                    client.querySelector("UserAgent")?.textContent ||
                    client.querySelector("user_agent")?.textContent ||
                    "",
                connected:
                    client.querySelector("Connected")?.textContent ||
                    client.querySelector("connected")?.textContent ||
                    "0",
                mount: mountPath,
            });
        });

        return listeners;
    },

    /**
     * Update statistics (ANTI-FLICKER: uses smart text updates)
     */
    updateStats() {
        // Get filtered listeners
        const listeners = this._mountFilter
            ? this._listeners.filter((l) => l.mount === this._mountFilter)
            : this._listeners;

        // Total listeners - use smart update
        UI.updateText("totalListenerCount", String(listeners.length));

        // Active mounts
        const activeMounts = this._streams.filter((s) => s.active).length;
        UI.updateText("activeMountCount", String(activeMounts));

        // Unique IPs
        const uniqueIPs = new Set(listeners.map((l) => l.ip)).size;
        UI.updateText("uniqueIPs", String(uniqueIPs));

        // Average listen time
        if (listeners.length > 0) {
            const totalSeconds = listeners.reduce(
                (sum, l) => sum + (parseInt(l.connected) || 0),
                0,
            );
            const avgSeconds = Math.floor(totalSeconds / listeners.length);
            UI.updateText("avgListenTime", UI.formatDuration(avgSeconds));
        } else {
            UI.updateText("avgListenTime", "--:--");
        }

        // Update badge
        UI.updateText("listenerCountBadge", String(listeners.length));

        // Enable/disable kick all button
        const kickAllBtn = UI.$("kickAllBtn");
        if (kickAllBtn) kickAllBtn.disabled = listeners.length === 0;
    },

    /**
     * Render listeners table (ANTI-FLICKER: smart DOM updates)
     */
    renderListeners() {
        const container = UI.$("listenersContainer");
        if (!container) return;

        // Get filtered listeners
        const listeners = this._mountFilter
            ? this._listeners.filter((l) => l.mount === this._mountFilter)
            : this._listeners;

        // Build signature to detect changes
        const signature = listeners
            .map((l) => `${l.id}:${l.ip}:${l.connected}:${l.mount}`)
            .join("|");

        // Skip update if nothing changed
        if (this._lastSignature === signature) {
            return;
        }
        this._lastSignature = signature;

        if (listeners.length === 0) {
            const emptyHTML = `
                <div class="empty-state">
                    <div class="empty-icon">👥</div>
                    <div class="empty-title">No Listeners Connected</div>
                    <div class="empty-text">
                        ${this._mountFilter ? `No listeners on ${this._mountFilter}` : "Listeners will appear here when they connect"}
                    </div>
                </div>
            `;
            UI.updateHTML(container, emptyHTML);
            return;
        }

        // Sort by connected time (longest first)
        listeners.sort(
            (a, b) =>
                (parseInt(b.connected) || 0) - (parseInt(a.connected) || 0),
        );

        // Use requestAnimationFrame to batch DOM update
        requestAnimationFrame(() => {
            container.innerHTML = `
            <div class="table-container">
                <table class="table">
                    <thead>
                        <tr>
                            <th>IP Address</th>
                            <th>Mount</th>
                            <th>User Agent</th>
                            <th>Connected</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${listeners.map((l) => this.renderListenerRow(l)).join("")}
                    </tbody>
                </table>
            </div>
        `;
        });
    },

    /**
     * Render a single listener row
     */
    renderListenerRow(listener) {
        const connected = parseInt(listener.connected) || 0;
        const duration = UI.formatDuration(connected);

        // Truncate user agent for display
        const userAgent = listener.userAgent || "Unknown";
        const shortUA =
            userAgent.length > 50
                ? userAgent.substring(0, 50) + "..."
                : userAgent;

        // Show connection count if more than 1
        const connections = listener.connections || 1;
        const connBadge =
            connections > 1
                ? `<span class="badge badge-neutral" title="${connections} browser connections">${connections}x</span>`
                : "";

        return `
            <tr>
                <td class="mono">${UI.escapeHtml(listener.ip || "Unknown")} ${connBadge}</td>
                <td class="mono">${UI.escapeHtml(listener.mount)}</td>
                <td title="${UI.escapeHtml(userAgent)}">${UI.escapeHtml(shortUA)}</td>
                <td>${duration}</td>
                <td>
                    <button class="btn btn-sm btn-danger" onclick="ListenersPage.kickListener('${listener.mount}', '${listener.id}')" title="Kick listener${connections > 1 ? ` (all ${connections} connections)` : ""}">
                        ⏏️ Kick
                    </button>
                </td>
            </tr>
        `;
    },

    /**
     * Kick a single listener
     */
    async kickListener(mount, listenerId) {
        const confirmed = await UI.confirm(
            "Are you sure you want to disconnect this listener?",
            {
                title: "Kick Listener",
                confirmText: "Kick",
                danger: true,
            },
        );

        if (confirmed) {
            try {
                await API.killClient(mount, listenerId);
                UI.success("Listener disconnected");
                await this.refresh();
            } catch (err) {
                UI.error("Failed to kick listener: " + err.message);
            }
        }
    },

    /**
     * Kick all listeners (optionally filtered by mount)
     */
    async kickAll() {
        const mount = this._mountFilter;
        const message = mount
            ? `Are you sure you want to disconnect ALL listeners from ${mount}?`
            : "Are you sure you want to disconnect ALL listeners from ALL mounts?";

        const confirmed = await UI.confirm(message, {
            title: "Kick All Listeners",
            confirmText: "Kick All",
            danger: true,
        });

        if (confirmed) {
            try {
                const listeners = mount
                    ? this._listeners.filter((l) => l.mount === mount)
                    : this._listeners;

                let kicked = 0;
                for (const listener of listeners) {
                    try {
                        await API.killClient(listener.mount, listener.id);
                        kicked++;
                    } catch (err) {
                        console.error(`Failed to kick ${listener.id}:`, err);
                    }
                }

                UI.success(`Disconnected ${kicked} listener(s)`);
                await this.refresh();
            } catch (err) {
                UI.error("Failed to kick listeners: " + err.message);
            }
        }
    },
};

// Export for use in app
window.ListenersPage = ListenersPage;
