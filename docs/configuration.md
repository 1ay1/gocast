# Configuration Reference

GoCast uses a single JSON configuration file for all settings. By default, this file is located at:

```
~/.gocast/config.json
```

## File Location

| Flag | Location |
|------|----------|
| Default | `~/.gocast/config.json` |
| Custom | `./gocast -data /path/to/dir` → `/path/to/dir/config.json` |

## Complete Configuration Example

```json
{
  "version": 1,
  "setup_complete": true,
  "server": {
    "hostname": "radio.example.com",
    "listen_address": "0.0.0.0",
    "port": 8000,
    "admin_root": "/admin",
    "location": "New York, USA",
    "server_id": "My Radio Server"
  },
  "limits": {
    "max_clients": 100,
    "max_sources": 10,
    "max_listeners_per_mount": 100,
    "queue_size": 131072,
    "burst_size": 2048,
    "client_timeout": 30,
    "header_timeout": 5,
    "source_timeout": 5
  },
  "auth": {
    "source_password": "broadcast-password",
    "admin_user": "admin",
    "admin_password": "secure-admin-password"
  },
  "logging": {
    "log_level": "info",
    "access_log": "",
    "error_log": "",
    "log_size": 10000
  },
  "mounts": {
    "/live": {
      "name": "/live",
      "password": "optional-mount-password",
      "max_listeners": 100,
      "genre": "Various",
      "description": "My Radio Stream",
      "url": "https://example.com",
      "bitrate": 128,
      "type": "audio/mpeg",
      "public": true,
      "stream_name": "Live Radio",
      "burst_size": 65536
    }
  },
  "admin": {
    "enabled": true
  },
  "directory": {
    "enabled": false,
    "yp_urls": [],
    "interval": 600
  },
  "ssl": {
    "enabled": false,
    "port": 8443,
    "auto_ssl": false,
    "auto_ssl_email": "",
    "cert_path": "",
    "key_path": ""
  }
}
```

## Configuration Sections

### Server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `hostname` | string | `"localhost"` | Public hostname of the server |
| `listen_address` | string | `"0.0.0.0"` | IP address to bind to |
| `port` | int | `8000` | HTTP port |
| `admin_root` | string | `"/admin"` | URL path for admin panel |
| `location` | string | `"Earth"` | Server location (displayed in status) |
| `server_id` | string | `"GoCast"` | Server identifier |

### Limits

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_clients` | int | `100` | Maximum total connections |
| `max_sources` | int | `10` | Maximum simultaneous source connections |
| `max_listeners_per_mount` | int | `100` | Maximum listeners per mount point |
| `queue_size` | int | `131072` | Buffer size in bytes (128KB) |
| `burst_size` | int | `2048` | Initial burst data for new listeners |
| `client_timeout` | int | `30` | Client timeout in seconds |
| `header_timeout` | int | `5` | HTTP header read timeout |
| `source_timeout` | int | `5` | Source connection timeout |

### Auth

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `source_password` | string | (generated) | Global password for source connections |
| `admin_user` | string | `"admin"` | Admin panel username |
| `admin_password` | string | (generated) | Admin panel password |

### Logging

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `log_level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |
| `access_log` | string | `""` | Path to access log file (empty = stdout) |
| `error_log` | string | `""` | Path to error log file (empty = stderr) |
| `log_size` | int | `10000` | Max log entries to keep in memory |

### Mounts

Each mount is keyed by its path (e.g., `/live`):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | (path) | Mount point name |
| `password` | string | `""` | Mount-specific source password (optional) |
| `max_listeners` | int | `100` | Max listeners for this mount |
| `genre` | string | `""` | Stream genre |
| `description` | string | `""` | Stream description |
| `url` | string | `""` | Associated website URL |
| `bitrate` | int | `128` | Stream bitrate in kbps |
| `type` | string | `"audio/mpeg"` | Content type (MIME) |
| `public` | bool | `true` | List in public directories |
| `stream_name` | string | `""` | Display name for the stream |
| `burst_size` | int | `65536` | Burst size for this mount |
| `hidden` | bool | `false` | Hide from status page |

### Admin

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable/disable admin panel |

### Directory

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable YP directory listings |
| `yp_urls` | array | `[]` | YP server URLs |
| `interval` | int | `600` | Update interval in seconds |

### SSL

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable HTTPS |
| `port` | int | `8443` | HTTPS port |
| `auto_ssl` | bool | `false` | Use Let's Encrypt AutoSSL |
| `auto_ssl_email` | string | `""` | Email for Let's Encrypt notifications |
| `cert_path` | string | `""` | Path to SSL certificate (manual mode) |
| `key_path` | string | `""` | Path to SSL private key (manual mode) |

## Hot Reload

Most configuration changes apply immediately without restart. To reload after editing the file manually:

```bash
# Option 1: Send SIGHUP signal
kill -HUP $(pgrep gocast)

# Option 2: Use admin panel
# Settings → Reload from Disk
```

## Backup & Recovery

GoCast automatically:
- Creates a backup before each save (`config.json.backup`)
- Backs up corrupted configs (`config.json.corrupted.TIMESTAMP`)
- Recovers from backup if main config is corrupted
- Validates and auto-fixes invalid values

## Finding Your Password

If you forgot your admin password:

```bash
cat ~/.gocast/config.json | grep admin_password
```

Or view the full auth section:

```bash
cat ~/.gocast/config.json | grep -A3 '"auth"'
```

## Validation

GoCast validates configuration on load and automatically fixes invalid values:

| Field | Invalid Value | Auto-Fixed To |
|-------|---------------|---------------|
| `port` | ≤0 or >65535 | 8000 |
| `ssl.port` | ≤0 or >65535 | 8443 |
| `max_clients` | ≤0 | 100 |
| `max_clients` | >100000 | 100000 |
| `max_sources` | ≤0 | 10 |
| `queue_size` | <1024 | 1024 |
| `queue_size` | >10MB | 10MB |
| `log_level` | invalid | "info" |
| `admin_user` | empty | "admin" |
| `admin_password` | empty | (generated) |
| `source_password` | empty | (generated) |