/**
 * GoCast Admin API Module
 * Handles all communication with the GoCast server
 */

const API = {
  // Base paths
  basePath: "",
  adminPath: "/admin",

  // Auth token for SSE
  token: null,

  // Event source for SSE
  eventSource: null,

  // Event listeners
  listeners: new Map(),

  /**
   * Make an authenticated API request
   */
  async request(endpoint, options = {}) {
    const url = `${this.adminPath}${endpoint}`;

    const defaults = {
      headers: {
        "Content-Type": "application/json",
      },
      credentials: "include", // Include basic auth
    };

    const config = { ...defaults, ...options };

    if (options.body && typeof options.body === "object") {
      config.body = JSON.stringify(options.body);
    }

    try {
      const response = await fetch(url, config);

      if (response.status === 401) {
        throw new Error("Authentication required");
      }

      if (!response.ok) {
        const error = await response
          .json()
          .catch(() => ({ error: response.statusText }));
        throw new Error(error.error || error.message || "Request failed");
      }

      // Check if response is JSON
      const contentType = response.headers.get("content-type");
      if (contentType && contentType.includes("application/json")) {
        return await response.json();
      }

      return await response.text();
    } catch (err) {
      console.error(`API Error [${endpoint}]:`, err);
      throw err;
    }
  },

  /**
   * GET request
   */
  async get(endpoint) {
    return this.request(endpoint, { method: "GET" });
  },

  /**
   * POST request
   */
  async post(endpoint, data) {
    return this.request(endpoint, { method: "POST", body: data });
  },

  /**
   * PUT request
   */
  async put(endpoint, data) {
    return this.request(endpoint, { method: "PUT", body: data });
  },

  /**
   * DELETE request
   */
  async delete(endpoint) {
    return this.request(endpoint, { method: "DELETE" });
  },

  // ===== Config Endpoints =====

  /**
   * Get full configuration
   */
  async getConfig() {
    const result = await this.get("/config");
    return result.data || result;
  },

  /**
   * Update server configuration
   */
  async updateServerConfig(config) {
    return this.post("/config/server", config);
  },

  /**
   * Update limits configuration
   */
  async updateLimitsConfig(config) {
    return this.post("/config/limits", config);
  },

  /**
   * Update auth configuration
   */
  async updateAuthConfig(config) {
    return this.post("/config/auth", config);
  },

  /**
   * Reset configuration to defaults
   */
  async resetConfig() {
    return this.post("/config/reset", {});
  },

  /**
   * Export configuration
   */
  async exportConfig() {
    const data = await this.get("/config/export");
    return data;
  },

  // ===== Mount Endpoints =====

  /**
   * Get all mounts
   */
  async getMounts() {
    const result = await this.get("/config/mounts");
    return result.data || result;
  },

  /**
   * Get a specific mount
   */
  async getMount(path) {
    const encodedPath = encodeURIComponent(path);
    const result = await this.get(`/config/mounts${encodedPath}`);
    return result.data || result;
  },

  /**
   * Create a new mount
   */
  async createMount(mount) {
    return this.post("/config/mounts", mount);
  },

  /**
   * Update an existing mount
   */
  async updateMount(path, mount) {
    const encodedPath = encodeURIComponent(path);
    return this.put(`/config/mounts${encodedPath}`, mount);
  },

  /**
   * Delete a mount
   */
  async deleteMount(path) {
    const encodedPath = encodeURIComponent(path);
    return this.delete(`/config/mounts${encodedPath}`);
  },

  // ===== Status Endpoints =====

  /**
   * Get server status
   */
  async getStatus() {
    try {
      const response = await fetch("/status", {
        headers: {
          Accept: "application/json",
        },
      });
      if (!response.ok) throw new Error("Status request failed");
      return await response.json();
    } catch (err) {
      console.error("Status error:", err);
      throw err;
    }
  },

  /**
   * Get admin stats (XML parsed to JSON)
   */
  async getStats() {
    return this.get("/stats");
  },

  // ===== Listener Management =====

  /**
   * List clients on a mount
   */
  async listClients(mountPath) {
    const encodedPath = encodeURIComponent(mountPath);
    return this.get(`/listclients?mount=${encodedPath}`);
  },

  /**
   * Kill a specific client
   */
  async killClient(mountPath, clientId) {
    const params = new URLSearchParams({
      mount: mountPath,
      id: clientId,
    });
    return this.get(`/killclient?${params}`);
  },

  /**
   * Kill source on a mount
   */
  async killSource(mountPath) {
    const encodedPath = encodeURIComponent(mountPath);
    return this.get(`/killsource?mount=${encodedPath}`);
  },

  /**
   * Move clients between mounts
   */
  async moveClients(sourceMountPath, destMountPath) {
    const params = new URLSearchParams({
      mount: sourceMountPath,
      destination: destMountPath,
    });
    return this.get(`/moveclients?${params}`);
  },

  // ===== SSE (Server-Sent Events) =====

  /**
   * Get auth token for SSE connection
   */
  async getToken() {
    try {
      const response = await fetch(`${this.adminPath}/token`, {
        credentials: "include",
      });
      if (!response.ok) throw new Error("Token request failed");
      const data = await response.json();
      this.token = data.token;
      return this.token;
    } catch (err) {
      console.error("Token error:", err);
      throw err;
    }
  },

  /**
   * Connect to SSE events
   */
  async connectSSE() {
    // Get token first
    if (!this.token) {
      await this.getToken();
    }

    // Close existing connection
    this.disconnectSSE();

    return new Promise((resolve, reject) => {
      const url = `/events?token=${this.token}`;
      this.eventSource = new EventSource(url);

      this.eventSource.onopen = () => {
        console.log("SSE connected");
        this.emit("connected");
        resolve();
      };

      this.eventSource.onerror = (err) => {
        console.error("SSE error:", err);
        this.emit("disconnected");

        // Try to reconnect after delay
        setTimeout(() => {
          if (this.eventSource) {
            this.token = null;
            this.connectSSE().catch(console.error);
          }
        }, 5000);
      };

      this.eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          this.emit("message", data);

          // Emit specific event types
          if (data.type) {
            this.emit(data.type, data);
          }
        } catch (err) {
          console.error("SSE parse error:", err);
        }
      };

      // Handle specific event types
      ["stats", "listener", "source", "metadata", "config"].forEach((type) => {
        this.eventSource.addEventListener(type, (event) => {
          try {
            const data = JSON.parse(event.data);
            this.emit(type, data);
          } catch (err) {
            console.error(`SSE ${type} parse error:`, err);
          }
        });
      });
    });
  },

  /**
   * Disconnect SSE
   */
  disconnectSSE() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  },

  /**
   * Subscribe to events
   */
  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event).add(callback);

    // Return unsubscribe function
    return () => {
      this.listeners.get(event).delete(callback);
    };
  },

  /**
   * Emit event to listeners
   */
  emit(event, data) {
    if (this.listeners.has(event)) {
      this.listeners.get(event).forEach((callback) => {
        try {
          callback(data);
        } catch (err) {
          console.error(`Event listener error [${event}]:`, err);
        }
      });
    }
  },

  /**
   * Polling fallback for status updates
   */
  startPolling(interval = 2000) {
    this.stopPolling();

    const poll = async () => {
      try {
        const status = await this.getStatus();
        this.emit("stats", status);
      } catch (err) {
        this.emit("error", err);
      }
    };

    poll(); // Initial poll
    this._pollInterval = setInterval(poll, interval);
  },

  /**
   * Stop polling
   */
  stopPolling() {
    if (this._pollInterval) {
      clearInterval(this._pollInterval);
      this._pollInterval = null;
    }
  },
};

// Export for use in other modules
window.API = API;
