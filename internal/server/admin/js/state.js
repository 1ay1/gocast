/**
 * GoCast Admin State Management
 * Centralized state store with reactive updates
 */

const State = {
    // Application state
    data: {
        // Server info
        server: {
            connected: false,
            hostname: '',
            version: '',
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
        currentPage: 'dashboard',

        // Loading states
        loading: {
            global: false,
            streams: false,
            mounts: false,
            config: false,
        },
    },

    // Subscribers for state changes
    subscribers: new Map(),

    /**
     * Get a value from state using dot notation
     */
    get(path) {
        return path.split('.').reduce((obj, key) => {
            return obj && obj[key] !== undefined ? obj[key] : undefined;
        }, this.data);
    },

    /**
     * Set a value in state using dot notation
     */
    set(path, value) {
        const keys = path.split('.');
        const lastKey = keys.pop();
        const target = keys.reduce((obj, key) => {
            if (obj[key] === undefined) {
                obj[key] = {};
            }
            return obj[key];
        }, this.data);

        const oldValue = target[lastKey];
        target[lastKey] = value;

        // Notify subscribers
        this.notify(path, value, oldValue);
    },

    /**
     * Update multiple values at once
     */
    update(updates) {
        Object.entries(updates).forEach(([path, value]) => {
            this.set(path, value);
        });
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
     * Notify subscribers of state changes
     */
    notify(path, newValue, oldValue) {
        // Notify exact path subscribers
        if (this.subscribers.has(path)) {
            this.subscribers.get(path).forEach(callback => {
                try {
                    callback(newValue, oldValue, path);
                } catch (err) {
                    console.error(`State subscriber error [${path}]:`, err);
                }
            });
        }

        // Notify parent path subscribers (for nested updates)
        const parts = path.split('.');
        for (let i = parts.length - 1; i > 0; i--) {
            const parentPath = parts.slice(0, i).join('.');
            if (this.subscribers.has(parentPath)) {
                const parentValue = this.get(parentPath);
                this.subscribers.get(parentPath).forEach(callback => {
                    try {
                        callback(parentValue, null, parentPath);
                    } catch (err) {
                        console.error(`State subscriber error [${parentPath}]:`, err);
                    }
                });
            }
        }

        // Notify wildcard subscribers
        if (this.subscribers.has('*')) {
            this.subscribers.get('*').forEach(callback => {
                try {
                    callback(newValue, oldValue, path);
                } catch (err) {
                    console.error('State wildcard subscriber error:', err);
                }
            });
        }
    },

    /**
     * Update stats from server status
     */
    updateFromStatus(status) {
        if (!status) return;

        // Update server info
        this.set('server.connected', true);
        this.set('server.hostname', status.server_id || status.host || '');
        this.set('server.version', status.version || '');

        if (status.started) {
            this.set('server.startTime', new Date(status.started));
        }

        // Update stats
        let totalListeners = 0;
        let peakListeners = 0;
        let activeMounts = 0;

        const streams = [];

        if (status.mounts && Array.isArray(status.mounts)) {
            status.mounts.forEach(mount => {
                totalListeners += mount.listeners || 0;
                peakListeners = Math.max(peakListeners, mount.peak_listeners || 0);
                if (mount.active) {
                    activeMounts++;
                }

                streams.push({
                    path: mount.path || mount.mount,
                    name: mount.name || mount.stream_name || mount.path,
                    listeners: mount.listeners || 0,
                    peakListeners: mount.peak_listeners || 0,
                    active: mount.active || false,
                    bitrate: mount.bitrate || 0,
                    contentType: mount.content_type || 'audio/mpeg',
                    genre: mount.genre || '',
                    description: mount.description || '',
                    title: mount.title || mount.stream_title || '',
                    artist: mount.artist || '',
                    connected: mount.connected || 0,
                });
            });
        }

        this.set('stats.totalListeners', totalListeners);
        this.set('stats.peakListeners', Math.max(this.get('stats.peakListeners') || 0, peakListeners));
        this.set('stats.activeMounts', activeMounts);
        this.set('stats.totalMounts', streams.length);
        this.set('streams', streams);
    },

    /**
     * Update from config response
     */
    updateFromConfig(config) {
        if (!config) return;

        this.set('config.server', config.server || {});
        this.set('config.limits', config.limits || {});
        this.set('config.auth', {
            sourcePassword: config.auth?.source_password || '',
            adminUser: config.auth?.admin_user || '',
        });

        // Convert mounts object to array
        const mountsArray = Object.entries(config.mounts || {}).map(([path, mount]) => ({
            path,
            ...mount,
        }));

        this.set('mounts', mountsArray);
    },

    /**
     * Add log entry
     */
    addLog(type, message, data = {}) {
        const log = {
            id: Date.now() + Math.random(),
            time: new Date(),
            type,
            message,
            data,
        };

        const logs = this.get('logs') || [];
        logs.unshift(log);

        // Keep only last 500 logs
        if (logs.length > 500) {
            logs.length = 500;
        }

        this.set('logs', logs);
    },

    /**
     * Clear logs
     */
    clearLogs() {
        this.set('logs', []);
    },

    /**
     * Calculate uptime string
     */
    getUptimeString() {
        const startTime = this.get('server.startTime');
        if (!startTime) return '--:--:--';

        const seconds = Math.floor((Date.now() - startTime.getTime()) / 1000);
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = seconds % 60;

        return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    },

    /**
     * Reset state to defaults
     */
    reset() {
        this.data = {
            server: {
                connected: false,
                hostname: '',
                version: '',
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
            currentPage: 'dashboard',
            loading: {
                global: false,
                streams: false,
                mounts: false,
                config: false,
            },
        };
    },
};

// Export for use in other modules
window.State = State;
