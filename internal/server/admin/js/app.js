/**
 * GoCast Admin - Main Application Controller
 * Manages navigation, initialization, and app lifecycle
 */

const App = {
  // Current page
  currentPage: "dashboard",

  // Page params (for passing data between pages)
  _pageParams: null,

  // Page modules
  pages: {
    dashboard: DashboardPage,
    streams: StreamsPage,
    mounts: MountsPage,
    listeners: ListenersPage,
    settings: SettingsPage,
    logs: LogsPage,
  },

  // Page titles
  pageTitles: {
    dashboard: "Dashboard",
    streams: "Streams",
    mounts: "Mount Points",
    listeners: "Listeners",
    settings: "Settings",
    logs: "Logs",
  },

  /**
   * Initialize the application
   */
  async init() {
    console.log("GoCast Admin initializing...");

    // Setup navigation
    this.setupNavigation();

    // Setup keyboard shortcuts
    this.setupKeyboardShortcuts();

    // Setup refresh button
    const refreshBtn = UI.$("refreshBtn");
    if (refreshBtn) {
      refreshBtn.onclick = () => this.refreshCurrentPage();
    }

    // Connect to server
    await this.connect();

    // Navigate to initial page (from URL hash or default)
    const initialPage = window.location.hash.slice(1) || "dashboard";
    this.navigateTo(initialPage);

    // Start uptime ticker
    this.startUptimeTicker();

    console.log("GoCast Admin initialized");
  },

  /**
   * Connect to the server
   */
  async connect() {
    const statusDot = document.querySelector(".status-dot");
    const statusText = document.querySelector(".status-text");
    const connIndicator = UI.$("connectionIndicator");

    try {
      // Try to get initial status
      const status = await API.getStatus();

      // Update UI to show connected
      if (statusDot) {
        statusDot.classList.remove("offline");
        statusDot.classList.add("online");
      }
      if (statusText) {
        statusText.textContent = "Connected";
      }
      if (connIndicator) {
        connIndicator.classList.remove("disconnected");
        connIndicator.querySelector(".label").textContent = "Live";
      }

      // Update state
      State.set("server.connected", true);
      State.updateFromStatus(status);

      // Load initial config
      try {
        const config = await API.getConfig();
        State.updateFromConfig(config);
      } catch (err) {
        console.error("Failed to load config:", err);
      }

      // Try to connect SSE for real-time updates
      try {
        await API.connectSSE();

        // Subscribe to SSE stats events to keep state updated
        API.on("stats", (data) => {
          State.updateFromStatus(data);
        });
      } catch (err) {
        console.warn("SSE not available, falling back to polling:", err);
        API.startPolling(3000);
      }
    } catch (err) {
      console.error("Failed to connect:", err);

      // Update UI to show disconnected
      if (statusDot) {
        statusDot.classList.remove("online");
        statusDot.classList.add("offline");
      }
      if (statusText) {
        statusText.textContent = "Disconnected";
      }
      if (connIndicator) {
        connIndicator.classList.add("disconnected");
        connIndicator.querySelector(".label").textContent = "Offline";
      }

      State.set("server.connected", false);

      // Retry connection after delay
      setTimeout(() => this.connect(), 5000);
    }
  },

  /**
   * Setup navigation handlers
   */
  setupNavigation() {
    // Handle nav item clicks
    document.querySelectorAll(".nav-item").forEach((item) => {
      item.addEventListener("click", (e) => {
        e.preventDefault();
        const page = item.dataset.page;
        if (page) {
          this.navigateTo(page);
        }
      });
    });

    // Handle browser back/forward
    window.addEventListener("hashchange", () => {
      const page = window.location.hash.slice(1) || "dashboard";
      if (page !== this.currentPage) {
        this.navigateTo(page, null, false);
      }
    });
  },

  /**
   * Setup keyboard shortcuts
   */
  setupKeyboardShortcuts() {
    document.addEventListener("keydown", (e) => {
      // Escape to close modal
      if (e.key === "Escape") {
        UI.hideModal();
      }

      // Ctrl/Cmd + R to refresh (prevent default and do our refresh)
      if ((e.ctrlKey || e.metaKey) && e.key === "r") {
        e.preventDefault();
        this.refreshCurrentPage();
      }

      // Number keys to switch pages (1-6)
      if (
        e.key >= "1" &&
        e.key <= "6" &&
        !e.ctrlKey &&
        !e.metaKey &&
        !e.altKey
      ) {
        const target = e.target;
        if (
          target.tagName !== "INPUT" &&
          target.tagName !== "TEXTAREA" &&
          target.tagName !== "SELECT"
        ) {
          const pages = [
            "dashboard",
            "streams",
            "mounts",
            "listeners",
            "settings",
            "logs",
          ];
          const index = parseInt(e.key) - 1;
          if (pages[index]) {
            this.navigateTo(pages[index]);
          }
        }
      }
    });
  },

  /**
   * Navigate to a page
   */
  navigateTo(page, params = null, updateHash = true) {
    // Validate page exists
    if (!this.pages[page]) {
      console.error("Unknown page:", page);
      return;
    }

    // Destroy current page
    const currentPageModule = this.pages[this.currentPage];
    if (currentPageModule && typeof currentPageModule.destroy === "function") {
      currentPageModule.destroy();
    }

    // Store params
    this._pageParams = params;

    // Update current page
    this.currentPage = page;
    State.set("currentPage", page);

    // Update URL hash
    if (updateHash) {
      window.location.hash = page;
    }

    // Update nav active state
    document.querySelectorAll(".nav-item").forEach((item) => {
      item.classList.toggle("active", item.dataset.page === page);
    });

    // Update page title
    const titleEl = UI.$("pageTitle");
    if (titleEl) {
      titleEl.textContent = this.pageTitles[page] || page;
    }

    // Render new page
    const content = UI.$("content");
    const pageModule = this.pages[page];

    if (content && pageModule) {
      // Show loading state briefly
      content.innerHTML = `
        <div class="loading">
          <div class="spinner"></div>
          <p>Loading...</p>
        </div>
      `;

      // Render page content
      setTimeout(() => {
        content.innerHTML = pageModule.render();

        // Initialize page
        if (typeof pageModule.init === "function") {
          pageModule.init();
        }
      }, 50);
    }
  },

  /**
   * Get current page params
   */
  getPageParams() {
    return this._pageParams;
  },

  /**
   * Refresh current page
   */
  async refreshCurrentPage() {
    const pageModule = this.pages[this.currentPage];

    if (pageModule) {
      // Show brief loading indicator on refresh button
      const refreshBtn = UI.$("refreshBtn");
      if (refreshBtn) {
        refreshBtn.disabled = true;
        refreshBtn.querySelector("span").textContent = "⏳";
      }

      try {
        // Call page refresh if available
        if (typeof pageModule.refresh === "function") {
          await pageModule.refresh();
        } else if (typeof pageModule.init === "function") {
          // Fall back to re-init
          await pageModule.init();
        }

        UI.success("Refreshed");
      } catch (err) {
        UI.error("Refresh failed: " + err.message);
      } finally {
        // Restore button
        if (refreshBtn) {
          refreshBtn.disabled = false;
          refreshBtn.querySelector("span").textContent = "🔄";
        }
      }
    }
  },

  /**
   * Start uptime ticker
   */
  startUptimeTicker() {
    const updateDisplay = () => {
      const uptimeStr = State.getUptimeString();

      // Update sidebar uptime
      const uptimeEl = UI.$("uptime");
      if (uptimeEl) {
        uptimeEl.textContent = uptimeStr;
      }

      // Update dashboard uptime
      const serverUptimeEl = UI.$("serverUptime");
      if (serverUptimeEl) {
        serverUptimeEl.textContent = uptimeStr;
      }

      // Update version in sidebar
      const versionEl = UI.$("version");
      if (versionEl) {
        const version = State.get("server.version");
        versionEl.textContent = version || "dev";
      }
    };

    // Update immediately and then every second
    updateDisplay();
    setInterval(updateDisplay, 1000);

    // Also subscribe to version changes for immediate updates
    State.subscribe("server.version", (version) => {
      const versionEl = UI.$("version");
      if (versionEl) {
        versionEl.textContent = version || "dev";
      }
    });
  },
};

// Initialize app when DOM is ready
document.addEventListener("DOMContentLoaded", () => {
  App.init().catch((err) => {
    console.error("App initialization failed:", err);
  });
});

// Export for global access
window.App = App;
