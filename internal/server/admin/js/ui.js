/**
 * GoCast Admin UI Utilities
 * Toast notifications, modals, and DOM helpers
 */

const UI = {
  // ===== Toast Notifications =====

  /**
   * Show a toast notification
   */
  toast(message, type = "info", duration = 4000) {
    const container = document.getElementById("toastContainer");
    if (!container) return;

    const toast = document.createElement("div");
    toast.className = `toast ${type}`;

    const icons = {
      success: "✓",
      error: "✕",
      warning: "⚠",
      info: "ℹ",
    };

    toast.innerHTML = `
            <span class="toast-icon">${icons[type] || icons.info}</span>
            <span class="toast-message">${this.escapeHtml(message)}</span>
        `;

    container.appendChild(toast);

    // Auto remove
    setTimeout(() => {
      toast.style.animation = "slideOut 0.3s ease forwards";
      setTimeout(() => toast.remove(), 300);
    }, duration);

    return toast;
  },

  success(message) {
    return this.toast(message, "success");
  },

  error(message) {
    return this.toast(message, "error", 6000);
  },

  warning(message) {
    return this.toast(message, "warning");
  },

  info(message) {
    return this.toast(message, "info");
  },

  // ===== Modal =====

  /**
   * Show a modal dialog
   */
  showModal(options = {}) {
    const overlay = document.getElementById("modalOverlay");
    const modal = document.getElementById("modal");
    const title = document.getElementById("modalTitle");
    const body = document.getElementById("modalBody");
    const footer = document.getElementById("modalFooter");

    if (!overlay || !modal) return;

    // Set content
    title.textContent = options.title || "Modal";
    body.innerHTML = options.body || "";

    // Set footer buttons
    footer.innerHTML = "";

    if (options.buttons) {
      options.buttons.forEach((btn) => {
        const button = document.createElement("button");
        button.className = `btn ${btn.class || "btn-secondary"}`;
        button.textContent = btn.text;
        button.onclick = () => {
          if (btn.onClick) {
            const result = btn.onClick();
            if (result !== false) {
              this.hideModal();
            }
          } else {
            this.hideModal();
          }
        };
        footer.appendChild(button);
      });
    } else {
      // Default close button
      const closeBtn = document.createElement("button");
      closeBtn.className = "btn btn-secondary";
      closeBtn.textContent = "Close";
      closeBtn.onclick = () => this.hideModal();
      footer.appendChild(closeBtn);
    }

    // Show modal
    overlay.classList.add("active");

    // Close on overlay click
    overlay.onclick = (e) => {
      if (e.target === overlay) {
        this.hideModal();
      }
    };

    // Close button
    document.getElementById("modalClose").onclick = () => this.hideModal();

    return modal;
  },

  /**
   * Hide the modal
   */
  hideModal() {
    const overlay = document.getElementById("modalOverlay");
    if (overlay) {
      overlay.classList.remove("active");
    }
  },

  /**
   * Show a confirmation dialog
   */
  confirm(message, options = {}) {
    return new Promise((resolve) => {
      this.showModal({
        title: options.title || "Confirm",
        body: `<p>${this.escapeHtml(message)}</p>`,
        buttons: [
          {
            text: options.cancelText || "Cancel",
            class: "btn-secondary",
            onClick: () => {
              resolve(false);
            },
          },
          {
            text: options.confirmText || "Confirm",
            class: options.danger ? "btn-danger" : "btn-primary",
            onClick: () => {
              resolve(true);
            },
          },
        ],
      });
    });
  },

  // ===== DOM Helpers =====

  /**
   * Get element by ID
   */
  $(id) {
    return document.getElementById(id);
  },

  /**
   * Query selector
   */
  $$(selector, parent = document) {
    return parent.querySelector(selector);
  },

  /**
   * Query selector all
   */
  $$$(selector, parent = document) {
    return parent.querySelectorAll(selector);
  },

  /**
   * Create element with attributes
   */
  createElement(tag, attrs = {}, children = []) {
    const el = document.createElement(tag);

    Object.entries(attrs).forEach(([key, value]) => {
      if (key === "className") {
        el.className = value;
      } else if (key === "innerHTML") {
        el.innerHTML = value;
      } else if (key === "textContent") {
        el.textContent = value;
      } else if (key.startsWith("on")) {
        el.addEventListener(key.slice(2).toLowerCase(), value);
      } else if (key === "dataset") {
        Object.entries(value).forEach(([k, v]) => {
          el.dataset[k] = v;
        });
      } else {
        el.setAttribute(key, value);
      }
    });

    children.forEach((child) => {
      if (typeof child === "string") {
        el.appendChild(document.createTextNode(child));
      } else if (child) {
        el.appendChild(child);
      }
    });

    return el;
  },

  /**
   * Escape HTML to prevent XSS
   */
  escapeHtml(str) {
    if (!str) return "";
    const div = document.createElement("div");
    div.textContent = str;
    return div.innerHTML;
  },

  /**
   * Format bytes to human readable
   */
  formatBytes(bytes, decimals = 1) {
    if (bytes === 0) return "0 B";

    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return (
      parseFloat((bytes / Math.pow(k, i)).toFixed(decimals)) + " " + sizes[i]
    );
  },

  /**
   * Format duration in seconds to HH:MM:SS
   */
  formatDuration(seconds) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);

    return [h, m, s].map((v) => v.toString().padStart(2, "0")).join(":");
  },

  /**
   * Format timestamp to readable string
   */
  formatTime(date) {
    if (!date) return "--:--:--";
    if (typeof date === "string") {
      date = new Date(date);
    }
    return date.toLocaleTimeString();
  },

  /**
   * Format date to readable string
   */
  formatDate(date) {
    if (!date) return "--";
    if (typeof date === "string") {
      date = new Date(date);
    }
    return date.toLocaleDateString() + " " + date.toLocaleTimeString();
  },

  /**
   * Debounce function calls
   */
  debounce(fn, delay) {
    let timeout;
    return (...args) => {
      clearTimeout(timeout);
      timeout = setTimeout(() => fn.apply(this, args), delay);
    };
  },

  /**
   * Throttle function calls
   */
  throttle(fn, limit) {
    let inThrottle;
    return (...args) => {
      if (!inThrottle) {
        fn.apply(this, args);
        inThrottle = true;
        setTimeout(() => (inThrottle = false), limit);
      }
    };
  },

  /**
   * Set loading state on an element
   */
  setLoading(element, loading) {
    if (typeof element === "string") {
      element = this.$(element);
    }
    if (!element) return;

    if (loading) {
      element.classList.add("loading");
      element.dataset.originalContent = element.innerHTML;
      element.innerHTML = '<div class="spinner"></div>';
    } else {
      element.classList.remove("loading");
      if (element.dataset.originalContent) {
        element.innerHTML = element.dataset.originalContent;
        delete element.dataset.originalContent;
      }
    }
  },

  /**
   * Show empty state in a container
   */
  showEmptyState(container, icon, title, text, action = null) {
    if (typeof container === "string") {
      container = this.$(container);
    }
    if (!container) return;

    let html = `
            <div class="empty-state">
                <div class="empty-icon">${icon}</div>
                <div class="empty-title">${this.escapeHtml(title)}</div>
                <div class="empty-text">${this.escapeHtml(text)}</div>
        `;

    if (action) {
      html += `<button class="btn btn-primary" id="emptyStateAction">${this.escapeHtml(action.text)}</button>`;
    }

    html += "</div>";
    container.innerHTML = html;

    if (action && action.onClick) {
      this.$("emptyStateAction").onclick = action.onClick;
    }
  },

  /**
   * Create a badge element
   */
  badge(text, type = "neutral") {
    return `<span class="badge badge-${type}">${this.escapeHtml(text)}</span>`;
  },

  /**
   * Create a progress bar
   */
  progressBar(percent, type = "") {
    const clampedPercent = Math.min(100, Math.max(0, percent));
    return `
            <div class="progress">
                <div class="progress-bar ${type}" style="width: ${clampedPercent}%"></div>
            </div>
        `;
  },

  /**
   * Toggle password field visibility
   */
  togglePassword(inputId, button) {
    const input = this.$(inputId);
    if (!input) return;

    if (input.type === "password") {
      input.type = "text";
      if (button) button.textContent = "🔒";
    } else {
      input.type = "password";
      if (button) button.textContent = "👁️";
    }
  },
};

// Export for use in other modules
window.UI = UI;
