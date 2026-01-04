/**
 * GoCast Admin State Management
 * Centralized state store with reactive updates
 *
 * ANTI-FLICKER DESIGN:
 * 1. Deep comparison before updates - only notify if data actually changed
 * 2. Debounced updates - prevent rapid successive notifications
 * 3. Cache last known good data - don't clear on temporary errors
 * 4. Batch updates - group multiple changes into single notification
 */

const State = {
    // Application state
    data: {
        // Server info
        server: {
            connected: false,
            hostname: "",
            version: "",
            startTime: null,
            uptime: 0,
        },

        // Statistics
        stats: {
            totalListeners: 0,
            peakListeners: 0,
            activeMounts: 0,
            totalMounts: 0,
            bandwidth: 0,
        },

        // Configuration
        config: {
            server: {},
            limits: {},
            auth: {},
            mounts: {},
        },

        // Live streams
        streams: [],

        // Mount configurations
        mounts: [],

        // Listeners
        listeners: [],

        // Activity log
        logs: [],

        // Current page
        currentPage: "dashboard",

        // Loading states
        loading: {
            global: false,
            streams: false,
            mounts: false,
            config: false,
        },

        // Last successful update timestamps (for cache management)
        _lastUpdate: {
            status: 0,
            config: 0,
            streams: 0,
        },
    },

    // Subscribers for state changes
    subscribers: new Map(),

    // Debounce timers for paths
    _debounceTimers: new Map(),

    // Batch update queue
    _batchQueue: new Map(),
    _batchTimer: null,

    // Last notified values (for diffing)
    _lastNotified: new Map(),

    /**
     * Deep equality check for objects/arrays
     */
    deepEqual(a, b) {
        if (a === b) return true;
        if (a === null || b === null) return a === b;
        if (typeof a !== typeof b) return false;

        if (typeof a !== "object") return a === b;

        if (Array.isArray(a) !== Array.isArray(b)) return false;

        if (Array.isArray(a)) {
            if (a.length !== b.length) return false;
            for (let i = 0; i < a.length; i++) {
                if (!this.deepEqual(a[i], b[i])) return false;
            }
            return true;
        }

        const keysA = Object.keys(a);
        const keysB = Object.keys(b);

        if (keysA.length !== keysB.length) return false;

        for (const key of keysA) {
            if (!keysB.includes(key)) return false;
            if (!this.deepEqual(a[key], b[key])) return false;
        }

        return true;
    },

    /**
     * Get a value from state using dot notation
     */
    get(path) {
        return path.split(".").reduce((obj, key) => {
            return obj && obj[key] !== undefined ? obj[key] : undefined;
        }, this.data);
    },

    /**
     * Set a value in state using dot notation
     * Only notifies if value actually changed
     */
    set(path, value, options = {}) {
        const keys = path.split(".");
        const lastKey = keys.pop();
        const target = keys.reduce((obj, key) => {
            if (obj[key] === undefined) {
                obj[key] = {};
            }
            return obj[key];
        }, this.data);

        const oldValue = target[lastKey];

        // Skip if value hasn't changed (deep comparison)
        if (!options.force && this.deepEqual(oldValue, value)) {
            return false;
        }

        target[lastKey] = value;

        // Use batched notification by default
        if (options.immediate) {
            this.notify(path, value, oldValue);
        } else {
            this.queueNotification(path, value, oldValue);
        }

        return true;
    },

    /**
     * Queue a notification for batched delivery
     */
    queueNotification(path, newValue, oldValue) {
        this._batchQueue.set(path, { newValue, oldValue });

        // Clear existing timer and set new one
        if (this._batchTimer) {
            clearTimeout(this._batchTimer);
        }

        // Flush batch after short delay (16ms = 1 frame)
        this._batchTimer = setTimeout(() => {
            this.flushBatch();
        }, 16);
    },

    /**
     * Flush all queued notifications
     */
    flushBatch() {
        const queue = new Map(this._batchQueue);
        this._batchQueue.clear();
        this._batchTimer = null;

        for (const [path, { newValue, oldValue }] of queue) {
            this.notify(path, newValue, oldValue);
        }
    },

    /**
     * Update multiple values at once (batched)
     */
    update(updates, options = {}) {
        let anyChanged = false;

        Object.entries(updates).forEach(([path, value]) => {
            if (this.set(path, value, { ...options, immediate: false })) {
                anyChanged = true;
            }
        });

        // Flush immediately if requested
        if (options.immediate && anyChanged) {
            this.flushBatch();
        }

        return anyChanged;
    },

    /**
     * Subscribe to state changes
     */
    subscribe(path, callback) {
        if (!this.subscribers.has(path)) {
            this.subscribers.set(path, new Set());
        }
        this.subscribers.get(path).add(callback);

        // Return unsubscribe function
        return () => {
            this.subscribers.get(path).delete(callback);
        };
    },

    /**
     * Notify subscribers of state changes (with deduplication)
     */
    notify(path, newValue, oldValue) {
        // Check if we've already notified with this exact value recently
        const lastNotified = this._lastNotified.get(path);
        if (lastNotified && this.deepEqual(lastNotified, newValue)) {
            return;
        }
        this._lastNotified.set(path, newValue);

        // Notify exact path subscribers
        if (this.subscribers.has(path)) {
            this.subscribers.get(path).forEach((callback) => {
                try {
                    callback(newValue, oldValue, path);
                } catch (err) {
                    console.error(`State subscriber error [${path}]:`, err);
                }
            });
        }

        // Notify parent path subscribers (for nested updates)
        const parts = path.split(".");
        for (let i = parts.length - 1; i > 0; i--) {
            const parentPath = parts.slice(0, i).join(".");
            if (this.subscribers.has(parentPath)) {
                const parentValue = this.get(parentPath);
                this.subscribers.get(parentPath).forEach((callback) => {
                    try {
                        callback(parentValue, null, parentPath);
                    } catch (err) {
                        console.error(
                            `State subscriber error [${parentPath}]:`,
                            err,
                        );
                    }
                });
            }
        }

        // Notify wildcard subscribers
        if (this.subscribers.has("*")) {
            this.subscribers.get("*").forEach((callback) => {
                try {
                    callback(newValue, oldValue, path);
                } catch (err) {
                    console.error("State wildcard subscriber error:", err);
                }
            });
        }
    },

    /**
     * Update stats from server status (with smart diffing)
     */
    updateFromStatus(status) {
        if (!status) return false;

        // Track if anything actually changed
        let changed = false;

        // Update server info
        changed |= this.set("server.connected", true);
        changed |= this.set(
            "server.hostname",
            status.server_id || status.host || "",
        );
        changed |= this.set("server.version", status.version || "");

        if (status.started) {
            const startTime = new Date(status.started);
            // Only update if different (compare timestamps)
            const currentStart = this.get("server.startTime");
            if (
                !currentStart ||
                currentStart.getTime() !== startTime.getTime()
            ) {
                changed |= this.set("server.startTime", startTime);
            }
        }

        // Calculate stats
        let totalListeners = 0;
        let peakListeners = 0;
        let activeMounts = 0;

        const streams = [];

        if (status.mounts && Array.isArray(status.mounts)) {
            status.mounts.forEach((mount) => {
                totalListeners += mount.listeners || 0;
                peakListeners = Math.max(
                    peakListeners,
                    mount.peak || mount.peak_listeners || 0,
                );
                if (mount.active) {
                    activeMounts++;
                }

                streams.push({
                    path: mount.path || mount.mount,
                    name: mount.name || mount.stream_name || mount.path,
                    listeners: mount.listeners || 0,
                    peak: mount.peak || mount.peak_listeners || 0,
                    active: mount.active || false,
                    bitrate: mount.bitrate || 0,
                    contentType:
                        mount.content_type || mount.type || "audio/mpeg",
                    genre: mount.genre || "",
                    description: mount.description || "",
                    title: mount.title || mount.stream_title || "",
                    artist: mount.artist || "",
                    connected: mount.connected || 0,
                    metadata: mount.metadata || {},
                });
            });
        }

        // Update stats (these will only notify if changed due to deepEqual check)
        changed |= this.set("stats.totalListeners", totalListeners);

        // Peak listeners should only ever increase
        const currentPeak = this.get("stats.peakListeners") || 0;
        const newPeak = Math.max(currentPeak, peakListeners);
        changed |= this.set("stats.peakListeners", newPeak);

        changed |= this.set("stats.activeMounts", activeMounts);
        changed |= this.set("stats.totalMounts", streams.length);

        // For streams, do a smarter comparison to avoid unnecessary updates
        const currentStreams = this.get("streams") || [];
        if (!this.streamsEqual(currentStreams, streams)) {
            changed |= this.set("streams", streams);
        }

        // Update timestamp
        this.data._lastUpdate.status = Date.now();

        return changed;
    },

    /**
     * Compare streams arrays efficiently
     */
    streamsEqual(a, b) {
        if (a.length !== b.length) return false;

        for (let i = 0; i < a.length; i++) {
            const sa = a[i];
            const sb = b[i];

            // Compare key fields that affect display
            if (
                sa.path !== sb.path ||
                sa.listeners !== sb.listeners ||
                sa.peak !== sb.peak ||
                sa.active !== sb.active ||
                sa.bitrate !== sb.bitrate ||
                sa.name !== sb.name ||
                sa.title !== sb.title ||
                sa.artist !== sb.artist
            ) {
                return false;
            }

            // Deep compare metadata if present
            if (!this.deepEqual(sa.metadata, sb.metadata)) {
                return false;
            }
        }

        return true;
    },

    /**
     * Update from config response
     */
    updateFromConfig(config) {
        if (!config) return false;

        let changed = false;

        changed |= this.set("config.server", config.server || {});
        changed |= this.set("config.limits", config.limits || {});
        changed |= this.set("config.auth", {
            sourcePassword: config.auth?.source_password || "",
            adminUser: config.auth?.admin_user || "",
        });

        // Convert mounts object to array
        const mountsArray = Object.entries(config.mounts || {}).map(
            ([path, mount]) => ({
                path,
                ...mount,
            }),
        );

        changed |= this.set("mounts", mountsArray);

        // Update timestamp
        this.data._lastUpdate.config = Date.now();

        return changed;
    },

    /**
     * Check if cached data is still fresh
     */
    isFresh(type, maxAgeMs = 5000) {
        const lastUpdate = this.data._lastUpdate[type] || 0;
        return Date.now() - lastUpdate < maxAgeMs;
    },

    /**
     * Add log entry (with deduplication)
     */
    addLog(type, message, data = {}) {
        const logs = this.get("logs") || [];

        // Check for duplicate recent message (within 2 seconds)
        const recentDuplicate = logs.find(
            (log) =>
                log.message === message &&
                log.type === type &&
                Date.now() - log.time.getTime() < 2000,
        );

        if (recentDuplicate) {
            return; // Skip duplicate
        }

        const log = {
            id: Date.now() + Math.random(),
            time: new Date(),
            type,
            message,
            data,
        };

        logs.unshift(log);

        // Keep only last 500 logs
        if (logs.length > 500) {
            logs.length = 500;
        }

        this.set("logs", logs);
    },

    /**
     * Clear logs
     */
    clearLogs() {
        this.set("logs", []);
    },

    /**
     * Calculate uptime string
     */
    getUptimeString() {
        const startTime = this.get("server.startTime");
        if (!startTime) return "--:--:--";

        const seconds = Math.floor((Date.now() - startTime.getTime()) / 1000);
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = seconds % 60;

        return `${hours.toString().padStart(2, "0")}:${minutes.toString().padStart(2, "0")}:${secs.toString().padStart(2, "0")}`;
    },

    /**
     * Reset state to defaults
     */
    reset() {
        this._batchQueue.clear();
        if (this._batchTimer) {
            clearTimeout(this._batchTimer);
            this._batchTimer = null;
        }
        this._lastNotified.clear();

        this.data = {
            server: {
                connected: false,
                hostname: "",
                version: "",
                startTime: null,
                uptime: 0,
            },
            stats: {
                totalListeners: 0,
                peakListeners: 0,
                activeMounts: 0,
                totalMounts: 0,
                bandwidth: 0,
            },
            config: {
                server: {},
                limits: {},
                auth: {},
                mounts: {},
            },
            streams: [],
            mounts: [],
            listeners: [],
            logs: [],
            currentPage: "dashboard",
            loading: {
                global: false,
                streams: false,
                mounts: false,
                config: false,
            },
            _lastUpdate: {
                status: 0,
                config: 0,
                streams: 0,
            },
        };
    },
};

// Export for use in other modules
window.State = State;
