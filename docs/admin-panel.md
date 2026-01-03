# Admin Panel Guide

The GoCast Admin Panel is a web-based interface for managing your streaming server. All settings can be configured from here without editing files.

## Accessing the Admin Panel

**URL:** `http://localhost:8000/admin/`

**Credentials:**
- Username: `admin` (or your configured admin_user)
- Password: Found in `~/.gocast/config.json` under `auth.admin_password`

## Dashboard

The dashboard provides a real-time overview of your server:

- **Active Streams** - Currently broadcasting mount points
- **Total Listeners** - Connected listeners across all mounts
- **Server Uptime** - How long the server has been running
- **Bandwidth** - Current data transfer rate

### Live Statistics

Statistics update in real-time via Server-Sent Events (SSE). No page refresh needed.

## Pages

### Streams

View all mount points and their status:

- **Live Streams** - Mounts with active sources
- **Listener Count** - Per-mount listener statistics
- **Stream Metadata** - Title, artist, and other info
- **Actions** - Disconnect source, view details

### Mounts

Configure mount points:

| Setting | Description |
|---------|-------------|
| Path | URL path (e.g., `/live`) |
| Stream Name | Display name |
| Password | Mount-specific source password |
| Max Listeners | Listener limit for this mount |
| Genre | Stream genre |
| Description | Stream description |
| Bitrate | Stream bitrate (kbps) |
| Content Type | MIME type (audio/mpeg, audio/ogg, etc.) |
| Public | List in directories |
| Burst Size | Initial data sent to new listeners |

#### Add a Mount

1. Click **Add Mount**
2. Enter mount path (e.g., `/radio`)
3. Configure settings
4. Click **Create Mount**

Changes apply immediately.

### Listeners

View and manage connected listeners:

- **IP Address** - Client IP
- **User Agent** - Client software
- **Connected** - Connection duration
- **Bytes Sent** - Data transferred
- **Mount** - Which stream they're listening to

#### Actions

- **Kick Listener** - Disconnect a specific listener
- **Move Listeners** - Move all listeners to another mount

### Logs

View server logs in real-time:

- **Access Logs** - HTTP requests
- **Activity Logs** - Connections, disconnections, events
- **Filter by Type** - Info, Warning, Error

Logs stream in real-time. Use the pause button to stop scrolling.

### Settings

Configure all server settings organized by category:

#### Server Tab

| Setting | Description |
|---------|-------------|
| Hostname | Public hostname (used for SSL and directories) |
| Listen Address | IP to bind to (0.0.0.0 for all) |
| Port | HTTP port (default: 8000) |
| Location | Server location (display only) |
| Server ID | Server identifier |

#### SSL Tab

**AutoSSL (Recommended):**
1. Enter your domain name
2. Enter email (optional, for expiry notifications)
3. Click **Enable AutoSSL**
4. Restart GoCast

**Manual SSL:**
1. Expand "Manual SSL Configuration"
2. Enter SSL port (default: 8443)
3. Enter paths to certificate and key files
4. Click **Save Manual SSL Settings**
5. Restart GoCast

#### Limits Tab

| Setting | Description |
|---------|-------------|
| Max Clients | Total connection limit |
| Max Sources | Maximum simultaneous broadcasters |
| Max Listeners Per Mount | Per-mount listener limit |
| Queue Size | Buffer size (bytes) |
| Burst Size | Initial data for new listeners |
| Client Timeout | Listener timeout (seconds) |
| Header Timeout | HTTP header timeout |
| Source Timeout | Source connection timeout |

**Presets:** Use Low/Balanced/High presets for quick configuration.

#### Authentication Tab

| Setting | Description |
|---------|-------------|
| Source Password | Global password for broadcasters |
| Admin Username | Admin panel login |
| Admin Password | Admin panel password |

**Note:** Changing admin credentials requires re-login.

#### Logging Tab

| Setting | Description |
|---------|-------------|
| Log Level | debug, info, warn, error |
| Access Log | File path (empty = console) |
| Error Log | File path (empty = console) |
| Log Size | Max entries in memory |

#### Directory Tab

Configure YP (Yellow Pages) directory listings:

| Setting | Description |
|---------|-------------|
| Enabled | Enable directory listings |
| YP URLs | Directory server URLs |
| Interval | Update interval (seconds) |

## Toolbar Actions

### Reload from Disk

Reloads `config.json` from disk. Useful after manual file edits.

### Export Config

Downloads the current configuration as a JSON file.

### Reset to Defaults

Resets all settings to defaults (preserves credentials).

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save current settings |
| `Esc` | Close modal |

## Mobile Support

The admin panel is responsive and works on mobile devices. Some features may have a simplified interface on smaller screens.

## Troubleshooting

### Can't Access Admin Panel

1. Check GoCast is running: `pgrep gocast`
2. Verify port: `curl http://localhost:8000/`
3. Check credentials in config file

### Changes Not Applying

Most changes apply immediately. For SSL changes, restart is required:

```bash
# Restart GoCast
pkill gocast && ./gocast
```

### Forgot Admin Password

```bash
cat ~/.gocast/config.json | grep admin_password
```

Or reset it by editing the config file:

```bash
# Edit config
nano ~/.gocast/config.json

# Change admin_password, then reload
kill -HUP $(pgrep gocast)
```
