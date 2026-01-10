# Admin API Reference

GoCast provides a REST API for administration and monitoring. All admin endpoints require HTTP Basic Authentication.

## Authentication

All `/admin/*` endpoints require Basic Auth:

```bash
curl -u admin:your-password http://localhost:8000/admin/config
```

Credentials are configured in `config.json`:
```json
{
  "auth": {
    "admin_user": "admin",
    "admin_password": "your-password"
  }
}
```

## Response Format

All API responses return JSON:

```json
{
  "success": true,
  "message": "Operation completed",
  "data": { ... }
}
```

Error responses:
```json
{
  "success": false,
  "error": "Error description"
}
```

---

## Configuration API

### Get Full Configuration

```
GET /admin/config
```

**Response:**
```json
{
  "success": true,
  "data": {
    "server": {
      "hostname": "localhost",
      "listen_address": "0.0.0.0",
      "port": 8000,
      "admin_root": "/admin",
      "location": "Earth",
      "server_id": "GoCast"
    },
    "ssl": {
      "enabled": false,
      "auto_ssl": false,
      "port": 8443
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
      "source_password": "hackme",
      "admin_user": "admin"
    },
    "logging": {
      "log_level": "info",
      "log_size": 10000
    },
    "directory": {
      "enabled": false,
      "interval": 600
    },
    "mounts": {
      "/live": {
        "path": "/live",
        "name": "/live",
        "max_listeners": 100,
        "bitrate": 128,
        "type": "audio/mpeg",
        "public": true
      }
    },
    "setup_complete": true,
    "config_path": "/home/user/.gocast/config.json"
  }
}
```

### Reload Configuration from Disk

```
POST /admin/config/reload
```

**Response:**
```json
{
  "success": true,
  "message": "Configuration reloaded from disk. Changes applied immediately."
}
```

### Reset to Defaults

```
POST /admin/config/reset
```

**Response:**
```json
{
  "success": true,
  "message": "Configuration reset to defaults. Changes applied immediately."
}
```

### Export Configuration

```
GET /admin/config/export
```

**Response:** Raw JSON config file (Content-Disposition: attachment)

---

## Server Configuration

### Update Server Settings

```
POST /admin/config/server
```

**Request Body:**
```json
{
  "hostname": "radio.example.com",
  "listen_address": "0.0.0.0",
  "port": 8000,
  "location": "New York",
  "server_id": "My Radio"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Server configuration updated. Changes applied immediately."
}
```

---

## SSL Configuration

### Get SSL Configuration

```
GET /admin/config/ssl
```

**Response:**
```json
{
  "success": true,
  "data": {
    "enabled": false,
    "auto_ssl": false,
    "auto_ssl_email": "",
    "port": 8443,
    "cert_path": "",
    "key_path": "",
    "hostname": "localhost"
  }
}
```

### Update SSL Configuration

```
POST /admin/config/ssl
```

**Request Body:**
```json
{
  "enabled": true,
  "auto_ssl": false,
  "port": 8443,
  "cert_path": "/etc/ssl/certs/server.crt",
  "key_path": "/etc/ssl/private/server.key"
}
```

### Enable AutoSSL

```
POST /admin/config/ssl/enable
```

**Request Body:**
```json
{
  "hostname": "radio.example.com",
  "email": "admin@example.com"
}
```

**Response:**
```json
{
  "success": true,
  "message": "AutoSSL enabled. Restart the server to obtain the certificate."
}
```

### Disable SSL

```
POST /admin/config/ssl/disable
```

**Response:**
```json
{
  "success": true,
  "message": "SSL disabled. Changes applied immediately."
}
```

---

## Limits Configuration

### Get Limits

```
GET /admin/config/limits
```

**Response:**
```json
{
  "success": true,
  "data": {
    "max_clients": 100,
    "max_sources": 10,
    "max_listeners_per_mount": 100,
    "queue_size": 131072,
    "burst_size": 2048,
    "client_timeout": 30,
    "header_timeout": 5,
    "source_timeout": 5
  }
}
```

### Update Limits

```
POST /admin/config/limits
```

**Request Body:**
```json
{
  "max_clients": 500,
  "max_sources": 20,
  "max_listeners_per_mount": 200,
  "queue_size": 262144,
  "burst_size": 4096,
  "client_timeout": 60,
  "header_timeout": 10,
  "source_timeout": 10
}
```

---

## Authentication Configuration

### Update Auth Settings

```
POST /admin/config/auth
```

**Request Body:**
```json
{
  "source_password": "new-source-password",
  "admin_user": "admin",
  "admin_password": "new-admin-password"
}
```

**Note:** Only include fields you want to change. Empty fields are ignored.

---

## Logging Configuration

### Update Logging Settings

```
POST /admin/config/logging
```

**Request Body:**
```json
{
  "log_level": "debug",
  "access_log": "/var/log/gocast/access.log",
  "error_log": "/var/log/gocast/error.log",
  "log_size": 5000
}
```

---

## Directory Configuration

### Update Directory Settings

```
POST /admin/config/directory
```

**Request Body:**
```json
{
  "enabled": true,
  "yp_urls": ["http://dir.xiph.org/cgi-bin/yp-cgi"],
  "interval": 300
}
```

---

## Mount Configuration

### List All Mounts

```
GET /admin/config/mounts
```

**Response:**
```json
{
  "success": true,
  "data": {
    "/live": {
      "path": "/live",
      "name": "/live",
      "max_listeners": 100,
      "genre": "Various",
      "description": "Live Stream",
      "bitrate": 128,
      "type": "audio/mpeg",
      "public": true,
      "stream_name": "My Radio",
      "burst_size": 65536
    }
  }
}
```

### Get Specific Mount

```
GET /admin/config/mounts/live
```

**Response:**
```json
{
  "success": true,
  "data": {
    "path": "/live",
    "name": "/live",
    "max_listeners": 100,
    ...
  }
}
```

### Create Mount

```
POST /admin/config/mounts
```

**Request Body:**
```json
{
  "path": "/radio",
  "stream_name": "My Radio Station",
  "password": "mount-specific-password",
  "max_listeners": 200,
  "genre": "Rock",
  "description": "The best rock music",
  "bitrate": 192,
  "type": "audio/mpeg",
  "public": true,
  "burst_size": 65536
}
```

**Response:**
```json
{
  "success": true,
  "message": "Mount /radio created. Changes applied immediately."
}
```

### Update Mount

```
PUT /admin/config/mounts/radio
```

**Request Body:**
```json
{
  "stream_name": "Updated Name",
  "max_listeners": 300,
  "bitrate": 256
}
```

**Note:** Password is preserved if not included in the update.

### Delete Mount

```
DELETE /admin/config/mounts/radio
```

**Response:**
```json
{
  "success": true,
  "message": "Mount /radio deleted. Changes applied immediately."
}
```

---

## Server Statistics

### Get Server Stats

```
GET /admin/stats
```

**Response:**
```json
{
  "server_id": "GoCast",
  "version": "1.0.0",
  "started": "2024-01-01T00:00:00Z",
  "uptime": 3600,
  "total_listeners": 42,
  "total_sources": 2,
  "mounts": [
    {
      "path": "/live",
      "active": true,
      "listeners": 30,
      "peak_listeners": 50,
      "source_ip": "192.168.1.100",
      "bitrate": 128,
      "content_type": "audio/mpeg"
    }
  ]
}
```

---

## Listener Management

### List Listeners

```
GET /admin/listclients?mount=/live
```

**Response:**
```json
{
  "mount": "/live",
  "listeners": [
    {
      "id": "abc123",
      "ip": "192.168.1.50",
      "user_agent": "VLC/3.0.16",
      "connected_at": "2024-01-01T12:00:00Z",
      "bytes_sent": 1048576
    }
  ]
}
```

### Kick Listener

```
POST /admin/killclient?mount=/live&id=abc123
```

**Response:**
```json
{
  "success": true,
  "message": "Client disconnected"
}
```

### Move Listeners

```
POST /admin/moveclients?from=/live&to=/backup
```

**Response:**
```json
{
  "success": true,
  "message": "Moved 10 listeners from /live to /backup"
}
```

---

## Source Management

### Kill Source

```
POST /admin/killsource?mount=/live
```

**Response:**
```json
{
  "success": true,
  "message": "Source disconnected from /live"
}
```

---

## Real-Time Events (SSE)

### Subscribe to Events

```
GET /admin/events
```

**Headers:**
```
Accept: text/event-stream
```

**Event Types:**

```
event: stats
data: {"total_listeners": 42, "mounts": [...]}

event: activity
data: {"type": "listener_connect", "mount": "/live", "ip": "192.168.1.50"}

event: log
data: {"level": "info", "message": "Source connected", "time": "..."}
```

---

## Status Page (Public)

### Get Server Status

```
GET /status?format=json
```

**No authentication required.** Perfect for radio station UIs.

**Response:**
```json
{
  "server_id": "My Radio Station",
  "version": "1.0.0",
  "started": "2024-01-01T12:00:00Z",
  "uptime": 3600,
  "total_bytes_sent": 1048576,
  "total_listeners": 42,
  "server": {
    "id": "My Radio Station",
    "version": "1.0.0",
    "hostname": "radio.example.com",
    "uptime": 3600,
    "total_bytes_sent": 1048576,
    "total_listeners": 42
  },
  "mounts": [
    {
      "path": "/live",
      "stream_url": "https://radio.example.com:8443/live",
      "active": true,
      "listeners": 42,
      "peak": 100,
      "bytes_sent": 524288,
      "content_type": "audio/mpeg",
      "stream_start": "2024-01-01T10:00:00Z",
      "stream_duration": 7200,
      "name": "My Radio Station",
      "bitrate": 128,
      "genre": "Rock",
      "description": "The best rock music 24/7",
      "public": true,
      "metadata": {
        "stream_title": "Artist Name - Song Title",
        "artist": "Artist Name",
        "title": "Song Title",
        "album": "Album Name",
        "url": "https://radio.example.com"
      },
      "history": [
        {
          "artist": "Current Artist",
          "title": "Current Song",
          "album": "Current Album",
          "started_at": "2024-01-01T12:05:00Z"
        },
        {
          "artist": "Previous Artist",
          "title": "Previous Song",
          "started_at": "2024-01-01T12:01:00Z"
        }
      ]
    }
  ]
}
```

**Key Fields for Radio UIs:**

| Field | Description |
|-------|-------------|
| `stream_url` | Direct playable stream URL (use in audio player) |
| `stream_start` | When the source connected (show "Live since...") |
| `stream_duration` | Seconds the stream has been live |
| `metadata.stream_title` | Currently playing (ICY title) |
| `metadata.artist` | Artist name |
| `metadata.title` | Song title |
| `metadata.album` | Album name (for album art lookup) |
| `history` | Last 20 tracks played (newest first) |

**Accept: text/xml**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<icestats>
  <source mount="/live">
    <listeners>42</listeners>
  </source>
</icestats>
```

**Accept: text/html** (default)
Returns HTML status page.


---

## Error Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad Request - Invalid parameters |
| 401 | Unauthorized - Invalid credentials |
| 404 | Not Found - Mount or resource doesn't exist |
| 409 | Conflict - Resource already exists |
| 500 | Internal Server Error |
| 503 | Service Unavailable - Server overloaded |

---

## Rate Limiting

The API does not currently implement rate limiting. For production use, consider placing a reverse proxy in front of GoCast with appropriate rate limits.

---

## Examples

### cURL: Get and Update Config

```bash
# Get current config
curl -u admin:password http://localhost:8000/admin/config

# Update limits
curl -u admin:password -X POST \
  -H "Content-Type: application/json" \
  -d '{"max_clients": 500}' \
  http://localhost:8000/admin/config/limits

# Create a mount
curl -u admin:password -X POST \
  -H "Content-Type: application/json" \
  -d '{"path": "/radio", "stream_name": "My Radio", "max_listeners": 100}' \
  http://localhost:8000/admin/config/mounts
```

### Python Example

```python
import requests
from requests.auth import HTTPBasicAuth

auth = HTTPBasicAuth('admin', 'password')
base_url = 'http://localhost:8000/admin'

# Get config
response = requests.get(f'{base_url}/config', auth=auth)
config = response.json()['data']

# Update limits
requests.post(f'{base_url}/config/limits', 
    auth=auth,
    json={'max_clients': 500}
)

# Create mount
requests.post(f'{base_url}/config/mounts',
    auth=auth,
    json={
        'path': '/radio',
        'stream_name': 'My Radio',
        'max_listeners': 100
    }
)
```
