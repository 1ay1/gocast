/**
 * GoCast Admin - Mounts Page
 * Mount point configuration and management
 */

const MountsPage = {
  // Current mounts data
  _mounts: [],

  // Edit mode
  _editingMount: null,

  /**
   * Render the mounts page
   */
  render() {
    return `
            <div class="flex justify-between items-center mb-3">
                <h2 style="margin: 0;">Mount Point Configuration</h2>
                <button class="btn btn-primary" onclick="MountsPage.showAddModal()">
                    ➕ Add Mount
                </button>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">📻 Configured Mounts</h3>
                    <button class="btn btn-sm btn-secondary" onclick="MountsPage.refresh()">
                        🔄 Refresh
                    </button>
                </div>
                <div class="card-body" id="mountsContainer">
                    <div class="loading">
                        <div class="spinner"></div>
                        <p>Loading mounts...</p>
                    </div>
                </div>
            </div>
        `;
  },

  /**
   * Initialize the page
   */
  async init() {
    await this.refresh();

    // Check if we should open edit modal for a specific mount
    const params = App.getPageParams();
    if (params && params.edit) {
      this.showEditModal(params.edit);
    }
  },

  /**
   * Clean up when leaving page
   */
  destroy() {
    this._editingMount = null;
  },

  /**
   * Refresh mounts data
   */
  async refresh() {
    try {
      // Get both config and live status to merge data
      const [config, status] = await Promise.all([
        API.getConfig(),
        API.getStatus().catch(() => ({ mounts: [] })),
      ]);

      // Create a map of live mount data for quick lookup
      const liveData = {};
      (status.mounts || []).forEach((m) => {
        liveData[m.path] = m;
      });

      // Merge config with live data
      this._mounts = Object.entries(config.mounts || {}).map(
        ([path, mount]) => {
          const live = liveData[path] || {};
          return {
            path,
            ...mount,
            // Override with live data when available
            active: live.active || false,
            listeners: live.listeners || 0,
            peak: live.peak || 0,
            // Store both config and live bitrate for comparison
            configBitrate: mount.bitrate || 128,
            // Use live bitrate if active, otherwise use configured
            bitrate: live.active && live.bitrate ? live.bitrate : mount.bitrate,
            // Use live name/genre if available
            liveName: live.name || null,
            liveGenre: live.genre || null,
          };
        },
      );
      this.renderMounts();
    } catch (err) {
      console.error("Mounts refresh error:", err);
      UI.error("Failed to load mounts: " + err.message);
    }
  },

  /**
   * Render mounts table
   */
  renderMounts() {
    const container = UI.$("mountsContainer");
    if (!container) return;

    if (this._mounts.length === 0) {
      container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">📻</div>
                    <div class="empty-title">No Mount Points Configured</div>
                    <div class="empty-text">Add mount points to enable streaming</div>
                    <button class="btn btn-primary" onclick="MountsPage.showAddModal()">
                        ➕ Add Mount
                    </button>
                </div>
            `;
      return;
    }

    container.innerHTML = `
            <div class="table-container">
                <table class="table">
                    <thead>
                        <tr>
                            <th>Mount Path</th>
                            <th>Name</th>
                            <th>Type</th>
                            <th>Max Listeners</th>
                            <th>Bitrate</th>
                            <th>Public</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${this._mounts.map((mount) => this.renderMountRow(mount)).join("")}
                    </tbody>
                </table>
            </div>
        `;
  },

  /**
   * Render a single mount row
   */
  renderMountRow(mount) {
    const path = mount.path || mount.name;
    const name = mount.stream_name || mount.name || path;
    const type = mount.type || "audio/mpeg";
    const maxListeners = mount.max_listeners || 100;
    const bitrate = mount.bitrate || 128;
    const isPublic = mount.public !== false;
    const isActive = mount.active || false;
    const listeners = mount.listeners || 0;
    // Check if bitrate is from live source or config
    const isLiveBitrate = isActive && mount.bitrate !== mount.configBitrate;

    return `
            <tr>
                <td class="mono">
                    ${UI.escapeHtml(path)}
                    ${isActive ? '<span style="margin-left: 6px;">🔴</span>' : ""}
                </td>
                <td>${UI.escapeHtml(mount.liveName || name)}</td>
                <td>${UI.escapeHtml(this.formatContentType(type))}</td>
                <td>${isActive ? `${listeners} / ${maxListeners}` : maxListeners}</td>
                <td>${bitrate} kbps${isLiveBitrate ? ' <span title="From live source" style="opacity:0.6">📡</span>' : ""}</td>
                <td>${isPublic ? UI.badge("Public", "success") : UI.badge("Private", "neutral")}</td>
                <td>
                    <div class="flex gap-1">
                        <button class="btn btn-sm btn-secondary" onclick="MountsPage.showEditModal('${path}')" title="Edit">
                            ✏️
                        </button>
                        <button class="btn btn-sm btn-danger" onclick="MountsPage.deleteMount('${path}')" title="Delete">
                            🗑️
                        </button>
                    </div>
                </td>
            </tr>
        `;
  },

  /**
   * Format content type to friendly name
   */
  formatContentType(type) {
    const types = {
      "audio/mpeg": "MP3",
      "audio/ogg": "OGG",
      "audio/aac": "AAC",
      "audio/opus": "Opus",
      "audio/flac": "FLAC",
    };
    return types[type] || type;
  },

  /**
   * Show add mount modal
   */
  showAddModal() {
    this._editingMount = null;

    UI.showModal({
      title: "Add Mount Point",
      body: this.renderMountForm({}),
      buttons: [
        {
          text: "Cancel",
          class: "btn-secondary",
        },
        {
          text: "Create Mount",
          class: "btn-primary",
          onClick: () => this.saveMount(),
        },
      ],
    });
  },

  /**
   * Show edit mount modal
   */
  showEditModal(path) {
    const mount = this._mounts.find((m) => m.path === path);
    if (!mount) {
      UI.error("Mount not found");
      return;
    }

    this._editingMount = path;

    UI.showModal({
      title: `Edit Mount: ${path}`,
      body: this.renderMountForm(mount),
      buttons: [
        {
          text: "Cancel",
          class: "btn-secondary",
        },
        {
          text: "Save Changes",
          class: "btn-primary",
          onClick: () => this.saveMount(),
        },
      ],
    });
  },

  /**
   * Render mount form
   */
  renderMountForm(mount) {
    const isEdit = !!this._editingMount;

    return `
            <div class="form-group">
                <label class="form-label">Mount Path *</label>
                <input type="text"
                       id="mountPath"
                       class="form-input"
                       placeholder="/stream"
                       value="${UI.escapeHtml(mount.path || "")}"
                       ${isEdit ? "disabled" : ""}>
                <span class="form-hint">URL path for the stream (e.g., /live, /radio)</span>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label class="form-label">Stream Name</label>
                    <input type="text"
                           id="mountName"
                           class="form-input"
                           placeholder="My Radio Stream"
                           value="${UI.escapeHtml(mount.stream_name || mount.name || "")}">
                </div>

                <div class="form-group">
                    <label class="form-label">Genre</label>
                    <input type="text"
                           id="mountGenre"
                           class="form-input"
                           placeholder="Various"
                           value="${UI.escapeHtml(mount.genre || "")}">
                </div>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label class="form-label">Content Type</label>
                    <select id="mountType" class="form-select">
                        <option value="audio/mpeg" ${mount.type === "audio/mpeg" ? "selected" : ""}>MP3</option>
                        <option value="audio/ogg" ${mount.type === "audio/ogg" ? "selected" : ""}>OGG Vorbis</option>
                        <option value="audio/aac" ${mount.type === "audio/aac" ? "selected" : ""}>AAC</option>
                        <option value="audio/opus" ${mount.type === "audio/opus" ? "selected" : ""}>Opus</option>
                    </select>
                </div>

                <div class="form-group">
                    <label class="form-label">Bitrate (kbps)</label>
                    <input type="number"
                           id="mountBitrate"
                           class="form-input"
                           placeholder="128"
                           value="${mount.bitrate || 128}">
                    <span class="form-hint">Default for directory listings. Actual bitrate is set by source client.</span>
                </div>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label class="form-label">Max Listeners</label>
                    <input type="number"
                           id="mountMaxListeners"
                           class="form-input"
                           placeholder="100"
                           value="${mount.max_listeners || 100}">
                </div>

                <div class="form-group">
                    <label class="form-label">Burst Size (bytes)</label>
                    <input type="number"
                           id="mountBurstSize"
                           class="form-input"
                           placeholder="65536"
                           value="${mount.burst_size || 65536}">
                    <span class="form-hint">Initial data sent to new listeners</span>
                </div>
            </div>

            <div class="form-group">
                <label class="form-label">Description</label>
                <input type="text"
                       id="mountDescription"
                       class="form-input"
                       placeholder="Stream description"
                       value="${UI.escapeHtml(mount.description || "")}">
            </div>

            <div class="form-group">
                <label class="form-label">Stream URL</label>
                <input type="text"
                       id="mountUrl"
                       class="form-input"
                       placeholder="https://example.com"
                       value="${UI.escapeHtml(mount.url || "")}">
                <span class="form-hint">Website URL for the stream</span>
            </div>

            <div class="form-group">
                <label class="form-label">Source Password</label>
                <input type="password"
                       id="mountPassword"
                       class="form-input"
                       placeholder="${isEdit ? "(unchanged)" : "Leave empty for default"}">
                <span class="form-hint">Password for source clients. Leave empty to use server default.</span>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label class="form-checkbox">
                        <input type="checkbox" id="mountPublic" ${mount.public !== false ? "checked" : ""}>
                        <span>Public (listed in directories)</span>
                    </label>
                </div>

                <div class="form-group">
                    <label class="form-checkbox">
                        <input type="checkbox" id="mountHidden" ${mount.hidden ? "checked" : ""}>
                        <span>Hidden (not shown in status)</span>
                    </label>
                </div>
            </div>
        `;
  },

  /**
   * Save mount (create or update)
   */
  async saveMount() {
    const isEdit = !!this._editingMount;

    // Get form values
    const path = isEdit ? this._editingMount : UI.$("mountPath").value.trim();
    const name = UI.$("mountName").value.trim();
    const genre = UI.$("mountGenre").value.trim();
    const type = UI.$("mountType").value;
    const bitrate = parseInt(UI.$("mountBitrate").value) || 128;
    const maxListeners = parseInt(UI.$("mountMaxListeners").value) || 100;
    const burstSize = parseInt(UI.$("mountBurstSize").value) || 65536;
    const description = UI.$("mountDescription").value.trim();
    const url = UI.$("mountUrl").value.trim();
    const password = UI.$("mountPassword").value;
    const isPublic = UI.$("mountPublic").checked;
    const hidden = UI.$("mountHidden").checked;

    // Validate
    if (!path) {
      UI.error("Mount path is required");
      return false;
    }

    // Ensure path starts with /
    const mountPath = path.startsWith("/") ? path : "/" + path;

    // Build mount config
    const mountConfig = {
      path: mountPath,
      name: mountPath,
      stream_name: name || mountPath,
      genre: genre,
      type: type,
      bitrate: bitrate,
      max_listeners: maxListeners,
      burst_size: burstSize,
      description: description,
      url: url,
      public: isPublic,
      hidden: hidden,
    };

    // Only include password if provided
    if (password) {
      mountConfig.password = password;
    }

    try {
      if (isEdit) {
        await API.updateMount(mountPath, mountConfig);
        UI.success(`Mount ${mountPath} updated`);
      } else {
        await API.createMount(mountConfig);
        UI.success(`Mount ${mountPath} created`);
      }

      this._editingMount = null;
      UI.hideModal();
      await this.refresh();
      return true;
    } catch (err) {
      UI.error("Failed to save mount: " + err.message);
      return false;
    }
  },

  /**
   * Delete a mount
   */
  async deleteMount(path) {
    const confirmed = await UI.confirm(
      `Are you sure you want to delete the mount point "${path}"? This cannot be undone.`,
      {
        title: "Delete Mount",
        confirmText: "Delete",
        danger: true,
      },
    );

    if (confirmed) {
      try {
        await API.deleteMount(path);
        UI.success(`Mount ${path} deleted`);
        await this.refresh();
      } catch (err) {
        UI.error("Failed to delete mount: " + err.message);
      }
    }
  },
};

// Export for use in app
window.MountsPage = MountsPage;
