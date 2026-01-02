# GoCast Admin API Reference

GoCast provides a comprehensive admin API compatible with Icecast. All admin endpoints require HTTP Basic Authentication.

## Authentication

All admin endpoints require authentication:

```bash
curl -u admin:hackme http://localhost:8000/admin/stats
```

Default credentials:
- **Username:** `admin`
- **Password:** `hackme`

## Endpoints

### GET /admin/

Admin dashboard HTML page showing server overview and quick links.

**Example:**
```bash
curl -u admin:hackme http://localhost:8000/admin/
```

### GET /admin/stats

Returns server statistics in XML format (Icecast compatible).

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme http://localhost:8000/admin/stats
```

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<icestats>
  <admin>/admin</admin>
  <host>localhost</host>
  <location>Earth</location>
  <server_id>GoCast/1.0.0</server_id>
  <server_start>2024-01-15T10:30:00Z</server_start>
  <source>
    <mount>/live</mount>
    <listeners>42</listeners>
    <peak_listeners>128</peak_listeners>
    <genre>Rock</genre>
    <server_name>My Radio</server_name>
    <server_description>24/7 Music</server_description>
    <server_type>audio/mpeg</server_type>
    <title>Artist - Song Title</title>
    <total_bytes_read>1234567890</total_bytes_read>
  </source>
</icestats>
```

### GET /admin/listmounts

Lists all configured mount points.

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme http://localhost:8000/admin/listmounts
```

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<icestats>
  <source>
    <mount>/live</mount>
    <listeners>10</listeners>
    <connected>true</connected>
    <content-type>audio/mpeg</content-type>
  </source>
  <source>
    <mount>/backup</mount>
    <listeners>0</listeners>
    <connected>false</connected>
    <content-type>audio/mpeg</content-type>
  </source>
</icestats>
```

### GET /admin/listclients

Lists all connected listeners for a specific mount.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `mount` | Yes | Mount point path (e.g., `/live`) |

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme "http://localhost:8000/admin/listclients?mount=/live"
```

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<icestats>
  <source mount="/live">
    <listener>
      <ID>550e8400-e29b-41d4-a716-446655440000</ID>
      <IP>192.168.1.100</IP>
      <UserAgent>VLC/3.0.18</UserAgent>
      <Connected>3600</Connected>
    </listener>
    <listener>
      <ID>6ba7b810-9dad-11d1-80b4-00c04fd430c8</ID>
      <IP>10.0.0.50</IP>
      <UserAgent>mpv/0.35.1</UserAgent>
      <Connected>1800</Connected>
    </listener>
  </source>
</icestats>
```

### GET /admin/metadata

Updates stream metadata (now playing information).

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `mount` | Yes | Mount point path |
| `mode` | Yes | Must be `updinfo` |
| `song` | Yes | New song/track title |

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme "http://localhost:8000/admin/metadata?mount=/live&mode=updinfo&song=Pink%20Floyd%20-%20Money"
```

**Response:**
```xml
<?xml version="1.0"?>
<iceresponse>
  <message>Metadata update successful</message>
  <return>1</return>
</iceresponse>
```

**Alternative with source credentials:**
```bash
curl -u source:hackme "http://localhost:8000/admin/metadata?mount=/live&mode=updinfo&song=New%20Song"
```

### GET /admin/killclient

Disconnects a specific listener.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `mount` | Yes | Mount point path |
| `id` | Yes | Listener ID (from listclients) |

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme "http://localhost:8000/admin/killclient?mount=/live&id=550e8400-e29b-41d4-a716-446655440000"
```

**Response:**
```xml
<?xml version="1.0"?>
<iceresponse>
  <message>Client killed</message>
  <return>1</return>
</iceresponse>
```

### GET /admin/killsource

Disconnects the source from a mount point.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `mount` | Yes | Mount point path |

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme "http://localhost:8000/admin/killsource?mount=/live"
```

**Response:**
```xml
<?xml version="1.0"?>
<iceresponse>
  <message>Source killed</message>
  <return>1</return>
</iceresponse>
```

### GET /admin/moveclients

Moves all clients from one mount to another.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `mount` | Yes | Source mount point |
| `destination` | Yes | Destination mount point |

**Response:** `application/xml`

**Example:**
```bash
curl -u admin:hackme "http://localhost:8000/admin/moveclients?mount=/live&destination=/backup"
```

## Public Status Endpoints

These endpoints don't require authentication:

### GET /status

Returns server status. Format determined by query parameter.

**Parameters:**
| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `format` | `html`, `json`, `xml` | `html` | Response format |

**Examples:**

```bash
# HTML (default)
curl http://localhost:8000/status

# JSON
curl http://localhost:8000/status?format=json

# XML
curl http://localhost:8000/status?format=xml
```

**JSON Response:**
```json
{
  "icestats": {
    "admin": "/admin",
    "host": "localhost",
    "location": "Earth",
    "server_id": "GoCast/1.0.0",
    "server_start": "",
    "source": [
      {
        "listenurl": "http://localhost:8000/live",
        "listeners": 42,
        "peak_listeners": 128,
        "audio_info": "",
        "genre": "Rock",
        "server_description": "24/7 Music",
        "server_name": "My Radio",
        "server_type": "audio/mpeg",
        "stream_start": "",
        "title": "Artist - Song"
      }
    ]
  }
}
```

### GET /

Root path returns the HTML status page.

## Response Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 400 | Bad Request - Missing required parameter |
| 401 | Unauthorized - Invalid or missing credentials |
| 403 | Forbidden - Admin interface disabled |
| 404 | Not Found - Mount or client not found |
| 409 | Conflict - Source already connected |
| 503 | Service Unavailable - Server at capacity |

## Scripting Examples

### Monitor Listener Count

```bash
#!/bin/bash
while true; do
  COUNT=$(curl -s "http://localhost:8000/status?format=json" | \
    jq '.icestats.source[0].listeners')
  echo "$(date): $COUNT listeners"
  sleep 60
done
```

### Auto-Update Metadata

```bash
#!/bin/bash
# Update metadata from a playlist file
while IFS= read -r song; do
  curl -s -u admin:hackme \
    "http://localhost:8000/admin/metadata?mount=/live&mode=updinfo&song=$(echo $song | jq -sRr @uri)"
  sleep 180  # 3 minutes per song
done < playlist.txt
```

### Kick Idle Listeners

```bash
#!/bin/bash
# Kick listeners connected for more than 1 hour
curl -s -u admin:hackme "http://localhost:8000/admin/listclients?mount=/live" | \
  xmllint --xpath "//listener[Connected > 3600]/ID/text()" - 2>/dev/null | \
  while read id; do
    curl -s -u admin:hackme "http://localhost:8000/admin/killclient?mount=/live&id=$id"
  done
```

## Compatibility Notes

The admin API is designed to be compatible with Icecast 2.x clients and monitoring tools:

- XML responses match Icecast format
- Same endpoint paths as Icecast
- Same parameter names
- Same authentication method

Tools tested:
- Icecast admin scripts
- Liquidsoap
- Azuracast
- Centova Cast
- Various monitoring plugins