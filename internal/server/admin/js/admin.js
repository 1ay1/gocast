// GoCast Admin Panel JavaScript

// ============== State ==============
let sessionToken = null;
let eventSource = null;
let startTime = Date.now();
let activityLogs = [];
let mountsData = {};
let peakListeners = 0;
let serverConfig = {};

// ============== Tab Navigation ==============
document.querySelectorAll(".nav-tab").forEach((tab) => {
  tab.addEventListener("click", () => {
    document
      .querySelectorAll(".nav-tab")
      .forEach((t) => t.classList.remove("active"));
    document
      .querySelectorAll(".tab-content")
      .forEach((c) => c.classList.remove("active"));
    tab.classList.add("active");
    document.getElementById("tab-" + tab.dataset.tab).classList.add("active");

    // Load data for specific tabs
    if (tab.dataset.tab === "settings") loadConfig();
    if (tab.dataset.tab === "mounts") loadMountsList();
  });
});

// ============== Toast Notifications ==============
function showToast(message, type = "info") {
  const container = document.getElementById("toastContainer");
  const toast = document.createElement("div");
  toast.className = `toast ${type}`;
  toast.innerHTML = `<span>${type === "success" ? "✅" : type === "error" ? "❌" : "ℹ️"}</span><span>${message}</span>`;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

// ============== SSE & Token Auth ==============
async function getSessionToken() {
  try {
    const response = await fetch("/admin/token", {
      credentials: "include",
    });
    if (response.ok) {
      const data = await response.json();
      sessionToken = data.token;
      return true;
    }
  } catch (e) {
    console.error("Failed to get session token:", e);
  }
  return false;
}

async function connectSSE() {
  if (!sessionToken) {
    const gotToken = await getSessionToken();
    if (!gotToken) {
      addLog("info", "Auth failed, using polling mode");
      startPolling();
      return;
    }
  }

  if (eventSource) eventSource.close();

  eventSource = new EventSource(`/events?token=${sessionToken}`);

  eventSource.onopen = () => {
    // Stop polling if it was running
    if (pollingInterval) {
      clearInterval(pollingInterval);
      pollingInterval = null;
    }
    document.getElementById("serverStatus").className = "status-dot";
    document.getElementById("serverStatusText").textContent = "Connected";
    addLog("info", "Real-time connection established");
  };

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.server_id) {
        const match = data.server_id.match(/GoCast\/(.+)/);
        if (match) {
          document.getElementById("serverVersion").textContent = "v" + match[1];
        }
      }
      updateDashboard(data);
    } catch (e) {
      console.error("Failed to parse SSE data:", e);
    }
  };

  eventSource.onerror = () => {
    document.getElementById("serverStatus").className = "status-dot offline";
    document.getElementById("serverStatusText").textContent = "Reconnecting...";
    eventSource.close();
    eventSource = null;
    sessionToken = null;
    setTimeout(connectSSE, 3000);
  };
}

let pollingInterval = null;
function startPolling() {
  if (pollingInterval) return;
  pollingInterval = setInterval(async () => {
    try {
      const response = await fetch("/status?format=json");
      const data = await response.json();
      if (data.icestats) {
        updateDashboard({
          source: data.icestats.source || [],
        });
      }
      document.getElementById("serverStatus").className = "status-dot";
      document.getElementById("serverStatusText").textContent =
        "Connected (polling)";
    } catch (e) {
      document.getElementById("serverStatus").className = "status-dot offline";
      document.getElementById("serverStatusText").textContent =
        "Connection Error";
    }
  }, 2000);
}

// ============== Dashboard Updates ==============
function updateDashboard(data) {
  const sources = data.source || [];
  let totalListeners = 0,
    activeMounts = 0,
    totalBandwidth = 0;

  sources.forEach((source) => {
    totalListeners += source.listeners || 0;
    peakListeners = Math.max(peakListeners, source.peak || 0, totalListeners);
    if (source.active) {
      activeMounts++;
      totalBandwidth += ((source.listeners || 0) * (source.bitrate || 128)) / 8;
    }

    const prev = mountsData[source.mount];
    if (prev) {
      if (prev.listeners !== source.listeners) {
        const diff = source.listeners - prev.listeners;
        if (diff > 0)
          addLog("connect", `+${diff} listener(s) on ${source.mount}`);
        else if (diff < 0)
          addLog(
            "disconnect",
            `${Math.abs(diff)} listener(s) left ${source.mount}`,
          );
      }
      if (!prev.active && source.active)
        addLog("source", `Source connected: ${source.mount}`);
      else if (prev.active && !source.active)
        addLog("source", `Source disconnected: ${source.mount}`);
    }
    mountsData[source.mount] = { ...source };
  });

  updateText("totalListeners", totalListeners);
  updateText("activeMounts", activeMounts);
  updateText("peakListeners", peakListeners);
  updateText("bandwidth", Math.round(totalBandwidth));

  renderMounts(sources);
}

function updateText(id, value) {
  const el = document.getElementById(id);
  if (el && el.textContent !== String(value)) el.textContent = value;
}

// ============== Mount Rendering ==============
function renderMounts(sources) {
  const container = document.getElementById("mountsContainer");
  if (!sources || sources.length === 0) {
    container.innerHTML =
      '<div class="mount-card"><p style="text-align: center; color: var(--text-muted); padding: 2rem;">No active streams</p></div>';
    return;
  }

  sources.sort((a, b) => (b.active ? 1 : 0) - (a.active ? 1 : 0));

  const existingCards = {};
  container.querySelectorAll(".mount-card[data-mount]").forEach((card) => {
    existingCards[card.dataset.mount] = card;
  });

  const seenMounts = new Set();

  sources.forEach((source) => {
    seenMounts.add(source.mount);
    let card = existingCards[source.mount];
    if (card) {
      updateMountCard(card, source);
    } else {
      card = createMountCard(source);
      container.appendChild(card);
    }
  });

  Object.keys(existingCards).forEach((mount) => {
    if (!seenMounts.has(mount)) existingCards[mount].remove();
  });

  const loading = container.querySelector(".mount-card:not([data-mount])");
  if (loading && sources.length > 0) loading.remove();
}

function createMountCard(source) {
  const card = document.createElement("div");
  card.className = `mount-card ${source.active ? "live" : ""}`;
  card.dataset.mount = source.mount;
  const nowPlaying = getNowPlaying(source);

  card.innerHTML = `
        <div class="mount-header">
            <span class="mount-name">${source.mount}</span>
            <span class="mount-status ${source.active ? "live" : "offline"}">
                <span class="status-dot ${source.active ? "" : "offline"}"></span>
                <span class="status-label">${source.active ? "LIVE" : "OFFLINE"}</span>
            </span>
        </div>
        <div class="visualizer ${source.active ? "" : "paused"}">
            <div class="visualizer-bar"></div><div class="visualizer-bar"></div>
            <div class="visualizer-bar"></div><div class="visualizer-bar"></div>
            <div class="visualizer-bar"></div><div class="visualizer-bar"></div>
            <div class="visualizer-bar"></div><div class="visualizer-bar"></div>
        </div>
        <div class="mount-meta">
            <div class="mount-meta-item">
                <div class="mount-meta-value" data-field="listeners">${source.listeners || 0}</div>
                <div class="mount-meta-label">Listeners</div>
            </div>
            <div class="mount-meta-item">
                <div class="mount-meta-value" data-field="peak">${source.peak || 0}</div>
                <div class="mount-meta-label">Peak</div>
            </div>
            <div class="mount-meta-item">
                <div class="mount-meta-value" data-field="bitrate">${source.bitrate || 128}</div>
                <div class="mount-meta-label">Kbps</div>
            </div>
        </div>
        <div class="now-playing">
            <span class="now-playing-icon">🎵</span>
            <div class="now-playing-text">
                <div class="now-playing-label">${source.active ? "Now Playing" : "Status"}</div>
                <div class="now-playing-title" data-field="nowplaying">${source.active ? nowPlaying : "No source connected"}</div>
            </div>
        </div>
        <div class="mount-genre" data-field="genre" style="display: ${source.genre ? "block" : "none"}">Genre: ${source.genre || ""}</div>
        <div class="mount-actions">
            <button class="btn btn-secondary btn-listen" ${!source.active ? "disabled" : ""}>▶️ Listen</button>
            <button class="btn btn-secondary btn-copy">📋 Copy URL</button>
            <button class="btn btn-danger btn-kill" ${!source.active ? "disabled" : ""}>⏹️ Kill</button>
        </div>
    `;

  const listenBtn = card.querySelector(".btn-listen");
  const copyBtn = card.querySelector(".btn-copy");
  const killBtn = card.querySelector(".btn-kill");

  // Use addEventListener with once-style guard to prevent double clicks
  listenBtn.addEventListener(
    "click",
    (e) => {
      e.preventDefault();
      e.stopPropagation();
      window.open(source.mount, "_blank");
    },
    { once: false },
  );

  copyBtn.addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    copyToClipboard(location.origin + source.mount, e);
  });

  killBtn.addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    killSource(source.mount);
  });

  return card;
}

function updateMountCard(card, source) {
  const isLive = source.active;
  card.classList.toggle("live", isLive);

  const statusEl = card.querySelector(".mount-status");
  if (statusEl) {
    statusEl.className = `mount-status ${isLive ? "live" : "offline"}`;
    const dot = statusEl.querySelector(".status-dot");
    if (dot) dot.className = `status-dot ${isLive ? "" : "offline"}`;
    const label = statusEl.querySelector(".status-label");
    if (label) label.textContent = isLive ? "LIVE" : "OFFLINE";
  }

  const viz = card.querySelector(".visualizer");
  if (viz) viz.classList.toggle("paused", !isLive);

  const fields = {
    listeners: source.listeners || 0,
    peak: source.peak || 0,
    bitrate: source.bitrate || 128,
    nowplaying: isLive ? getNowPlaying(source) : "No source connected",
    genre: source.genre || "",
  };

  Object.entries(fields).forEach(([field, value]) => {
    const el = card.querySelector(`[data-field="${field}"]`);
    if (el && el.textContent !== String(value)) el.textContent = value;
  });

  const genreEl = card.querySelector(".mount-genre");
  if (genreEl) genreEl.style.display = source.genre ? "block" : "none";

  const npLabel = card.querySelector(".now-playing-label");
  if (npLabel) npLabel.textContent = isLive ? "Now Playing" : "Status";

  const listenBtn = card.querySelector(".btn-listen");
  const killBtn = card.querySelector(".btn-kill");
  if (listenBtn) listenBtn.disabled = !isLive;
  if (killBtn) killBtn.disabled = !isLive;
}

function getNowPlaying(source) {
  const title = source.title || source.name || "";
  const artist = source.artist || "";
  const album = source.album || "";

  if (!title || title === "Live Stream on " + source.mount)
    return "Live Stream";

  let text = title;
  if (artist) text += ` - ${artist}`;
  if (album) text += ` (${album})`;
  return text;
}

// ============== Actions ==============
async function killSource(mount) {
  if (!confirm(`Kill source on ${mount}?`)) return;
  try {
    const response = await fetch(
      `/admin/killsource?mount=${encodeURIComponent(mount)}`,
      {
        credentials: "include",
      },
    );
    if (response.ok) {
      showToast(`Source killed: ${mount}`, "success");
      addLog("source", `Killed source: ${mount}`);
    } else {
      showToast("Failed to kill source", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

function copyToClipboard(text, e) {
  navigator.clipboard
    .writeText(text)
    .then(() => {
      const btn = e.currentTarget;
      const original = btn.innerHTML;
      btn.innerHTML = "✓ Copied!";
      setTimeout(() => (btn.innerHTML = original), 1500);
    })
    .catch(() => prompt("Copy URL:", text));
}

// ============== Activity Log ==============
function addLog(type, message) {
  const time = new Date().toTimeString().split(" ")[0];
  activityLogs.unshift({ time, type, message });
  if (activityLogs.length > 100) activityLogs.pop();
  renderLogs();
}

function renderLogs() {
  const html =
    activityLogs.length === 0
      ? '<div class="log-entry"><span class="log-time">--:--:--</span><span>No activity yet</span></div>'
      : activityLogs
          .map(
            (log) => `
                <div class="log-entry">
                    <span class="log-time">${log.time}</span>
                    <span class="log-type ${log.type}">${log.type}</span>
                    <span>${log.message}</span>
                </div>
            `,
          )
          .join("");

  document.getElementById("activityLog").innerHTML = html;
  document.getElementById("fullActivityLog").innerHTML = html;
}

function clearLogs() {
  activityLogs = [];
  renderLogs();
  showToast("Logs cleared", "success");
}

// ============== Configuration ==============
async function loadConfig() {
  try {
    const response = await fetch("/admin/config", {
      credentials: "include",
    });
    if (!response.ok) throw new Error("Failed to load config");
    const data = await response.json();
    if (data.success && data.data) {
      serverConfig = data.data;
      populateConfigForm(data.data);
      if (data.data.has_overrides) {
        document.getElementById("settingsAlert").innerHTML = `
                    <div class="alert alert-info">
                        <span>ℹ️</span>
                        <span>Runtime overrides active. Last modified: ${new Date(data.data.last_modified).toLocaleString()}</span>
                    </div>
                `;
      } else {
        document.getElementById("settingsAlert").innerHTML = "";
      }
    }
  } catch (e) {
    showToast("Failed to load configuration: " + e.message, "error");
  }
}

function populateConfigForm(cfg) {
  document.getElementById("cfg-hostname").value = cfg.server?.hostname || "";
  document.getElementById("cfg-location").value = cfg.server?.location || "";
  document.getElementById("cfg-server-id").value = cfg.server?.server_id || "";
  document.getElementById("cfg-port").value = cfg.server?.port || 8000;

  document.getElementById("cfg-max-clients").value =
    cfg.limits?.max_clients || 100;
  document.getElementById("cfg-max-sources").value =
    cfg.limits?.max_sources || 10;
  document.getElementById("cfg-max-listeners").value =
    cfg.limits?.max_listeners_per_mount || 100;
  document.getElementById("cfg-queue-size").value =
    cfg.limits?.queue_size || 131072;
  document.getElementById("cfg-burst-size").value =
    cfg.limits?.burst_size || 2048;

  document.getElementById("cfg-source-password").value =
    cfg.auth?.source_password || "";
  document.getElementById("cfg-admin-user").value = cfg.auth?.admin_user || "";
  document.getElementById("cfg-admin-password").value = "";
}

async function saveServerSettings() {
  const data = {
    hostname: document.getElementById("cfg-hostname").value,
    location: document.getElementById("cfg-location").value,
    server_id: document.getElementById("cfg-server-id").value,
  };

  try {
    const response = await fetch("/admin/config/server", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });
    const result = await response.json();
    if (result.success) {
      showToast("Server settings saved", "success");
      addLog("config", "Server settings updated");
      loadConfig();
    } else {
      showToast(result.error || "Failed to save", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

async function saveLimitsSettings() {
  const data = {
    max_clients:
      parseInt(document.getElementById("cfg-max-clients").value) || 100,
    max_sources:
      parseInt(document.getElementById("cfg-max-sources").value) || 10,
    max_listeners_per_mount:
      parseInt(document.getElementById("cfg-max-listeners").value) || 100,
    queue_size:
      parseInt(document.getElementById("cfg-queue-size").value) || 131072,
    burst_size:
      parseInt(document.getElementById("cfg-burst-size").value) || 2048,
  };

  try {
    const response = await fetch("/admin/config/limits", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });
    const result = await response.json();
    if (result.success) {
      showToast("Limits saved", "success");
      addLog("config", "Resource limits updated");
      loadConfig();
    } else {
      showToast(result.error || "Failed to save", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

async function saveAuthSettings() {
  const data = {
    source_password: document.getElementById("cfg-source-password").value,
    admin_user: document.getElementById("cfg-admin-user").value,
    admin_password:
      document.getElementById("cfg-admin-password").value || undefined,
  };

  try {
    const response = await fetch("/admin/config/auth", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });
    const result = await response.json();
    if (result.success) {
      showToast("Auth settings saved", "success");
      addLog("config", "Authentication updated");
      if (data.admin_password) {
        showToast(
          "Admin password changed. You may need to re-login.",
          "warning",
        );
      }
      loadConfig();
    } else {
      showToast(result.error || "Failed to save", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

async function resetConfig() {
  if (
    !confirm(
      "Reset all configuration to defaults? This will remove all runtime changes.",
    )
  )
    return;
  try {
    const response = await fetch("/admin/config/reset", {
      method: "POST",
      credentials: "include",
    });
    const result = await response.json();
    if (result.success) {
      showToast("Configuration reset to defaults", "success");
      addLog("config", "Configuration reset to defaults");
      loadConfig();
    } else {
      showToast(result.error || "Failed to reset", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

async function exportConfig() {
  window.open("/admin/config/export", "_blank");
}

// ============== Mounts Management ==============
async function loadMountsList() {
  try {
    const response = await fetch("/admin/config/mounts", {
      credentials: "include",
    });
    if (!response.ok) throw new Error("Failed to load mounts");
    const data = await response.json();
    if (data.success && data.data) {
      renderMountsList(data.data);
    }
  } catch (e) {
    document.getElementById("mountsList").innerHTML =
      `<p style="color: var(--accent-red);">Failed to load mounts: ${e.message}</p>`;
  }
}

function renderMountsList(mounts) {
  const container = document.getElementById("mountsList");
  if (!mounts || mounts.length === 0) {
    container.innerHTML =
      '<p style="text-align: center; color: var(--text-muted); padding: 2rem;">No mounts configured. Click "Add Mount" to create one.</p>';
    return;
  }

  container.innerHTML = mounts
    .map(
      (mount) => `
            <div class="mount-list-item">
                <div class="mount-list-info">
                    <span class="mount-list-path">${mount.path}</span>
                    <span class="mount-list-desc">${mount.description || mount.stream_name || "No description"}</span>
                </div>
                <div class="mount-list-actions">
                    <button class="btn btn-secondary" onclick="editMount('${mount.path}')">✏️ Edit</button>
                    <button class="btn btn-danger" onclick="deleteMount('${mount.path}')">🗑️ Delete</button>
                </div>
            </div>
        `,
    )
    .join("");
}

function showAddMountModal() {
  document.getElementById("mount-edit-mode").value = "create";
  document.getElementById("mountModalTitle").textContent = "Add Mount";
  document.getElementById("mount-path").value = "/";
  document.getElementById("mount-path").disabled = false;
  document.getElementById("mount-name").value = "";
  document.getElementById("mount-password").value = "";
  document.getElementById("mount-max-listeners").value = "100";
  document.getElementById("mount-bitrate").value = "128";
  document.getElementById("mount-type").value = "audio/mpeg";
  document.getElementById("mount-genre").value = "";
  document.getElementById("mount-url").value = "";
  document.getElementById("mount-description").value = "";
  document.getElementById("mount-public").checked = true;
  document.getElementById("mount-hidden").checked = false;
  document.getElementById("mountModal").classList.add("active");
}

async function editMount(path) {
  try {
    const response = await fetch(`/admin/config/mounts${path}`, {
      credentials: "include",
    });
    if (!response.ok) throw new Error("Mount not found");
    const data = await response.json();
    if (data.success && data.data) {
      const mount = data.data;
      document.getElementById("mount-edit-mode").value = "update";
      document.getElementById("mountModalTitle").textContent = "Edit Mount";
      document.getElementById("mount-path").value = mount.path;
      document.getElementById("mount-path").disabled = true;
      document.getElementById("mount-name").value = mount.stream_name || "";
      document.getElementById("mount-password").value = "";
      document.getElementById("mount-max-listeners").value =
        mount.max_listeners || 100;
      document.getElementById("mount-bitrate").value = mount.bitrate || 128;
      document.getElementById("mount-type").value = mount.type || "audio/mpeg";
      document.getElementById("mount-genre").value = mount.genre || "";
      document.getElementById("mount-url").value = mount.url || "";
      document.getElementById("mount-description").value =
        mount.description || "";
      document.getElementById("mount-public").checked = mount.public !== false;
      document.getElementById("mount-hidden").checked = mount.hidden === true;
      document.getElementById("mountModal").classList.add("active");
    }
  } catch (e) {
    showToast("Failed to load mount: " + e.message, "error");
  }
}

function closeMountModal() {
  document.getElementById("mountModal").classList.remove("active");
}

async function saveMount() {
  const mode = document.getElementById("mount-edit-mode").value;
  const path = document.getElementById("mount-path").value;

  if (!path || path === "/") {
    showToast("Mount path is required", "error");
    return;
  }

  const data = {
    path: path,
    stream_name: document.getElementById("mount-name").value,
    password: document.getElementById("mount-password").value,
    max_listeners:
      parseInt(document.getElementById("mount-max-listeners").value) || 100,
    bitrate: parseInt(document.getElementById("mount-bitrate").value) || 128,
    type: document.getElementById("mount-type").value,
    genre: document.getElementById("mount-genre").value,
    url: document.getElementById("mount-url").value,
    description: document.getElementById("mount-description").value,
    public: document.getElementById("mount-public").checked,
    hidden: document.getElementById("mount-hidden").checked,
  };

  try {
    const url =
      mode === "create"
        ? "/admin/config/mounts"
        : `/admin/config/mounts${path}`;
    const method = mode === "create" ? "POST" : "PUT";

    const response = await fetch(url, {
      method: method,
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });
    const result = await response.json();
    if (result.success) {
      showToast(
        mode === "create" ? "Mount created" : "Mount updated",
        "success",
      );
      addLog(
        "config",
        `Mount ${path} ${mode === "create" ? "created" : "updated"}`,
      );
      closeMountModal();
      loadMountsList();
    } else {
      showToast(result.error || "Failed to save mount", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

async function deleteMount(path) {
  if (!confirm(`Delete mount ${path}? This cannot be undone.`)) return;
  try {
    const response = await fetch(`/admin/config/mounts${path}`, {
      method: "DELETE",
      credentials: "include",
    });
    const result = await response.json();
    if (result.success) {
      showToast("Mount deleted", "success");
      addLog("config", `Mount ${path} deleted`);
      loadMountsList();
    } else {
      showToast(result.error || "Failed to delete mount", "error");
    }
  } catch (e) {
    showToast("Error: " + e.message, "error");
  }
}

// ============== Uptime ==============
function updateUptime() {
  const elapsed = Math.floor((Date.now() - startTime) / 1000);
  const h = String(Math.floor(elapsed / 3600)).padStart(2, "0");
  const m = String(Math.floor((elapsed % 3600) / 60)).padStart(2, "0");
  const s = String(elapsed % 60).padStart(2, "0");
  document.getElementById("uptime").textContent = `${h}:${m}:${s}`;
}

// ============== Initialize ==============
setInterval(updateUptime, 1000);
connectSSE();
addLog("info", "Admin panel initialized");
