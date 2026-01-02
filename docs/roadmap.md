# GoCast Roadmap

> **Mission**: Make GoCast the definitive modern replacement for Icecast by solving every pain point that has frustrated streaming operators for years.

This document outlines planned features, prioritized by impact and organized into implementation phases. Each feature addresses real problems that Icecast users face daily.

---

## Table of Contents

- [Phase 1: Modern Streaming Protocols](#phase-1-modern-streaming-protocols)
- [Phase 2: Real-time Monitoring & Observability](#phase-2-real-time-monitoring--observability)
- [Phase 3: Zero-Downtime Operations](#phase-3-zero-downtime-operations)
- [Phase 4: Authentication & Access Control](#phase-4-authentication--access-control)
- [Phase 5: Content Management](#phase-5-content-management)
- [Phase 6: Clustering & High Availability](#phase-6-clustering--high-availability)
- [Phase 7: Monetization & Engagement](#phase-7-monetization--engagement)
- [Phase 8: Developer Experience](#phase-8-developer-experience)
- [Quick Wins](#quick-wins)
- [Implementation Status](#implementation-status)

---

## Phase 1: Modern Streaming Protocols

### HLS (HTTP Live Streaming) Support

**Pain Point**: Icecast only supports the legacy Icecast/Shoutcast protocol. Modern browsers cannot play streams natively without plugins or external players. Mobile browsers are completely unsupported.

**Solution**: Native HLS output that converts incoming streams to `.m3u8` playlists with `.ts` segments.

**Features**:
- Automatic HLS transcoding from any input format
- Configurable segment duration (default: 6 seconds)
- Configurable playlist length (default: 3 segments)
- Low-latency HLS (LL-HLS) option for near-real-time playback
- Per-mount HLS enable/disable
- Automatic cleanup of old segments

**Configuration**:
```vibe
mounts {
    live {
        hls {
            enabled true
            segment_duration 6
            playlist_length 3
            low_latency false
            path /hls/live
        }
    }
}
```

**Endpoints**:
```
GET /hls/live/playlist.m3u8  → HLS playlist
GET /hls/live/segment_0.ts   → Media segment
```

**Why This Matters**: 
- Works in every modern browser without plugins
- Native iOS/Android support
- CDN-friendly (cacheable segments)
- Industry standard for video/audio streaming

---

### DASH (Dynamic Adaptive Streaming over HTTP) Support

**Pain Point**: Some platforms prefer DASH over HLS. No adaptive bitrate support in Icecast.

**Solution**: Native MPEG-DASH output with MPD manifests.

**Features**:
- Automatic DASH packaging
- Adaptive bitrate support (multiple quality levels)
- Compatible with dash.js and other DASH players

**Configuration**:
```vibe
mounts {
    live {
        dash {
            enabled true
            segment_duration 4
            path /dash/live
        }
    }
}
```

---

### WebSocket Audio Streaming

**Pain Point**: WebSocket-based players are becoming common for web apps, but Icecast doesn't support them.

**Solution**: WebSocket endpoint that streams audio data for custom web players.

**Features**:
- Binary WebSocket frames with audio data
- JSON metadata frames for now-playing updates
- Reconnection handling
- Optional Web Audio API-ready format (PCM)

**Endpoint**:
```
WS /ws/live  → WebSocket audio stream
```

---

## Phase 2: Real-time Monitoring & Observability

### WebSocket Real-time Dashboard

**Pain Point**: Icecast admins must constantly refresh pages to see current stats. No live updates.

**Solution**: WebSocket-powered admin dashboard with real-time updates.

**Features**:
- Live listener count (updates instantly)
- Real-time "now playing" display
- Live bytes transferred graphs
- Source connect/disconnect notifications
- Listener join/leave events (optional)
- Per-mount real-time stats

**Endpoints**:
```
WS  /admin/ws/stats     → Real-time stats stream
GET /admin/dashboard    → Modern web dashboard
```

**Dashboard Features**:
- Dark/light theme
- Mobile responsive
- Customizable widgets
- Historical graphs (last hour/day)
- Export to CSV/JSON

---

### Prometheus Metrics Endpoint

**Pain Point**: Icecast doesn't integrate with modern monitoring stacks. No way to set up alerts or dashboards in Grafana.

**Solution**: Native Prometheus metrics export.

**Metrics Exposed**:
```prometheus
# Server metrics
gocast_uptime_seconds
gocast_total_connections
gocast_active_connections

# Mount metrics
gocast_mount_listeners{mount="/live"}
gocast_mount_peak_listeners{mount="/live"}
gocast_mount_bytes_sent_total{mount="/live"}
gocast_mount_bytes_received_total{mount="/live"}
gocast_mount_source_connected{mount="/live"}
gocast_mount_source_uptime_seconds{mount="/live"}

# Listener metrics
gocast_listeners_total
gocast_listeners_by_country{country="US"}
gocast_listener_duration_seconds_bucket{le="60"}

# Performance metrics
gocast_buffer_usage_bytes{mount="/live"}
gocast_buffer_overruns_total{mount="/live"}
```

**Endpoint**:
```
GET /metrics  → Prometheus format
```

**Configuration**:
```vibe
metrics {
    enabled true
    path /metrics
    # Optional: require auth for metrics
    auth_required false
}
```

---

### Geo-IP Listener Statistics

**Pain Point**: Radio stations want to know where their audience is located. Icecast provides no geographic data.

**Solution**: Built-in GeoIP lookup for listener locations.

**Features**:
- Country/region/city detection
- Real-time geographic breakdown in dashboard
- Historical geographic data
- Privacy-friendly (configurable precision)
- Supports MaxMind GeoLite2 database

**Configuration**:
```vibe
geoip {
    enabled true
    database /etc/gocast/GeoLite2-City.mmdb
    precision city  # country, region, or city
}
```

**API Response**:
```json
{
  "mount": "/live",
  "listeners_by_country": {
    "US": 150,
    "UK": 45,
    "DE": 32,
    "CA": 28
  },
  "listeners_by_city": {
    "New York": 42,
    "London": 38,
    "Berlin": 25
  }
}
```

---

### Structured Logging

**Pain Point**: Icecast logs are hard to parse and analyze. No JSON logging for log aggregators.

**Solution**: Structured JSON logging option.

**Features**:
- JSON log format for ELK/Loki/Splunk
- Log levels (debug, info, warn, error)
- Request IDs for tracing
- Configurable log fields

**Configuration**:
```vibe
logging {
    format json  # or "text"
    level info
    include_request_id true
    output stdout  # or file path
}
```

---

## Phase 3: Zero-Downtime Operations

### Hot Configuration Reload

**Pain Point**: Icecast requires a full restart for any configuration change. This disconnects all listeners and sources.

**Solution**: SIGHUP-triggered config reload without dropping connections.

**Features**:
- Reload config without restart
- Add/remove mounts dynamically
- Update authentication credentials
- Change limits on the fly
- Validation before applying changes
- Rollback on invalid config

**Usage**:
```bash
# Reload configuration
kill -HUP $(pidof gocast)

# Or via admin API
curl -X POST http://admin:pass@localhost:8000/admin/reload
```

**What Can Be Reloaded**:
- ✅ Mount configurations
- ✅ Authentication passwords
- ✅ Listener limits
- ✅ Logging settings
- ⚠️ Port changes (requires restart)
- ⚠️ SSL certificates (graceful rotation)

---

### Graceful Shutdown

**Pain Point**: Stopping Icecast abruptly disconnects everyone.

**Solution**: Graceful shutdown with configurable drain period.

**Features**:
- Stop accepting new connections
- Allow current streams to finish
- Configurable drain timeout
- Status endpoint shows "draining" state

**Configuration**:
```vibe
server {
    shutdown_timeout 30s
    drain_listeners true
}
```

---

### Health Check Endpoint

**Pain Point**: Load balancers and Kubernetes need health checks. Icecast doesn't provide them.

**Solution**: Dedicated health endpoints.

**Endpoints**:
```
GET /health        → Basic health (200 OK or 503)
GET /health/live   → Liveness probe
GET /health/ready  → Readiness probe (checks dependencies)
```

**Response**:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "24h15m",
  "checks": {
    "mounts": "ok",
    "memory": "ok",
    "disk": "ok"
  }
}
```

---

## Phase 4: Authentication & Access Control

### Listener Authentication

**Pain Point**: Icecast has no way to restrict listener access. No subscriber-only streams, no paywalls.

**Solution**: Multiple authentication backends for listeners.

**Authentication Methods**:

#### Token-based Authentication
```vibe
mounts {
    premium {
        auth {
            type token
            secret "your-jwt-secret"
            # Tokens can include: expiry, listener_id, allowed_mounts
        }
    }
}
```

**Usage**:
```
GET /premium?token=eyJhbGciOiJIUzI1NiIs...
```

#### HTTP Callback Authentication
```vibe
mounts {
    premium {
        auth {
            type http
            url "https://your-api.com/auth/validate"
            timeout 5s
            # Your API receives: ip, mount, user_agent, headers
            # Return 200 to allow, 401/403 to deny
        }
    }
}
```

#### IP Allowlist/Denylist
```vibe
mounts {
    internal {
        auth {
            type ip
            allowed [192.168.1.0/24, 10.0.0.0/8]
            denied [192.168.1.100]
        }
    }
}
```

#### Username/Password (Basic Auth)
```vibe
mounts {
    members {
        auth {
            type basic
            users {
                alice "password123"
                bob "secret456"
            }
            # Or use file
            users_file /etc/gocast/listeners.txt
        }
    }
}
```

---

### API Keys for Programmatic Access

**Pain Point**: Sharing admin credentials is insecure. No scoped access control.

**Solution**: API key system with granular permissions.

**Features**:
- Create/revoke API keys
- Scope keys to specific actions
- Key expiration
- Usage tracking

**Configuration**:
```vibe
api_keys {
    stats_reader {
        key "gck_abc123..."
        permissions [read_stats, read_mounts]
        expires "2025-12-31"
    }
    
    full_admin {
        key "gck_xyz789..."
        permissions [*]
    }
}
```

---

### OAuth2/OIDC Integration

**Pain Point**: No way to integrate with existing identity providers.

**Solution**: OAuth2/OpenID Connect support.

**Features**:
- Support for Google, GitHub, Auth0, Keycloak, etc.
- JWT validation
- Claims-based mount access
- Admin panel SSO

**Configuration**:
```vibe
oauth {
    enabled true
    provider auth0
    client_id "your-client-id"
    client_secret "your-client-secret"
    issuer "https://your-tenant.auth0.com/"
}
```

---

## Phase 5: Content Management

### Auto-Recording & Archiving

**Pain Point**: Icecast can dump to files, but there's no scheduled recording, rotation, or management.

**Solution**: Built-in recording with scheduling and rotation.

**Features**:
- Continuous recording
- Scheduled recording (cron-like)
- Automatic file rotation (hourly/daily)
- Configurable format (keep source format or transcode)
- Storage limits with auto-cleanup
- Upload to S3/GCS/B2 (optional)

**Configuration**:
```vibe
mounts {
    live {
        recording {
            enabled true
            path /var/gocast/recordings
            format mp3  # or "source" to keep original
            rotation hourly  # or daily, size:100MB
            max_age 30d
            
            # Optional cloud upload
            upload {
                type s3
                bucket my-radio-archive
                region us-east-1
            }
        }
    }
}
```

**API**:
```
GET  /admin/recordings             → List recordings
GET  /admin/recordings/:id         → Download recording
DELETE /admin/recordings/:id       → Delete recording
POST /admin/recordings/start       → Start manual recording
POST /admin/recordings/stop        → Stop manual recording
```

---

### Stream Transcoding

**Pain Point**: Icecast serves exactly what the source sends. No way to offer multiple bitrates or formats.

**Solution**: Built-in transcoding to multiple outputs.

**Features**:
- Multiple bitrate outputs from single source
- Format conversion (MP3, AAC, Opus, Ogg)
- Configurable quality settings
- Low CPU mode (uses libav/FFmpeg)

**Configuration**:
```vibe
mounts {
    live {
        transcoding {
            enabled true
            outputs {
                high {
                    path /live-320
                    format mp3
                    bitrate 320k
                }
                medium {
                    path /live-128
                    format mp3
                    bitrate 128k
                }
                low {
                    path /live-64
                    format aac
                    bitrate 64k
                }
            }
        }
    }
}
```

---

### Fallback & Scheduled Switching

**Pain Point**: Icecast fallback only works when source disconnects. No way to schedule switches.

**Solution**: Advanced fallback with scheduling.

**Features**:
- Multiple fallback sources (cascade)
- Scheduled mount switching (cron-like)
- Intro/outro audio on source change
- API-triggered switches

**Configuration**:
```vibe
mounts {
    live {
        fallback {
            # Cascade fallback
            mounts [/backup, /emergency]
            
            # Play file when nothing available
            file /var/gocast/silence.mp3
            loop true
        }
        
        schedule {
            # Switch to automation at midnight
            "0 0 * * *" {
                source /automation
            }
            # Switch back to live at 6am
            "0 6 * * *" {
                source /live
                intro /var/gocast/jingles/morning.mp3
            }
        }
    }
}
```

---

### Album Art & Enhanced Metadata

**Pain Point**: ICY metadata only supports text. Modern players expect album artwork.

**Solution**: Extended metadata with image support.

**Features**:
- Album art URL in metadata
- Embedded artwork extraction (ID3)
- Artwork caching and resizing
- JSON metadata endpoint

**Endpoint**:
```
GET /live/metadata.json
```

**Response**:
```json
{
  "title": "Bohemian Rhapsody",
  "artist": "Queen",
  "album": "A Night at the Opera",
  "artwork": "http://localhost:8000/live/artwork.jpg",
  "duration": 354,
  "genre": "Rock",
  "year": 1975
}
```

**Configuration**:
```vibe
mounts {
    live {
        metadata {
            artwork_enabled true
            artwork_cache /var/gocast/artwork
            artwork_size 300x300
        }
    }
}
```

---

## Phase 6: Clustering & High Availability

### Relay Auto-Discovery

**Pain Point**: Setting up Icecast relays is manual and error-prone. No automatic failover.

**Solution**: Automatic relay discovery and failover.

**Features**:
- Automatic relay registration
- Health-based failover
- Geographic relay routing
- Relay cascading
- Bandwidth-aware distribution

**Configuration**:

**Primary Server**:
```vibe
cluster {
    role primary
    advertise "radio.example.com:8000"
    secret "cluster-secret-key"
}
```

**Relay Server**:
```vibe
cluster {
    role relay
    primary "radio.example.com:8000"
    secret "cluster-secret-key"
    region "us-west"
    
    # Auto-relay all public mounts
    auto_relay true
    
    # Or specify mounts
    mounts [/live, /music]
}
```

---

### Multi-Server Sync

**Pain Point**: No way to share state across multiple Icecast servers.

**Solution**: Distributed state sync for clusters.

**Features**:
- Synchronized listener counts
- Shared metadata across nodes
- Centralized statistics
- Leader election for management

**Configuration**:
```vibe
cluster {
    sync {
        enabled true
        nodes [
            "node1.example.com:8000"
            "node2.example.com:8000"
            "node3.example.com:8000"
        ]
        # Using Redis for state
        backend redis
        redis_url "redis://localhost:6379"
    }
}
```

---

### Geographic Load Balancing

**Pain Point**: No way to route listeners to nearest server.

**Solution**: Built-in geographic routing.

**Features**:
- Redirect listeners to nearest relay
- Latency-based routing
- Anycast DNS integration tips
- Manual region overrides

---

## Phase 7: Monetization & Engagement

### Pre-roll & Mid-roll Ad Insertion

**Pain Point**: No native way to insert ads or announcements.

**Solution**: Server-side ad insertion (SSAI).

**Features**:
- Pre-roll audio on listener connect
- Scheduled mid-roll insertion
- Per-listener targeting (with auth)
- VAST/VMAP compatibility
- Skip protection

**Configuration**:
```vibe
mounts {
    live {
        ads {
            preroll {
                enabled true
                files [/var/gocast/ads/ad1.mp3, /var/gocast/ads/ad2.mp3]
                rotation random
            }
            
            midroll {
                enabled true
                interval 15m
                max_duration 30s
                # Or use external ad server
                vast_url "https://ads.example.com/vast"
            }
        }
    }
}
```

---

### Webhooks & Notifications

**Pain Point**: No way to get notified of events. No integration with external systems.

**Solution**: Configurable webhooks for all events.

**Events**:
- `source.connect` - Source started streaming
- `source.disconnect` - Source stopped
- `source.metadata` - Metadata updated
- `listener.connect` - New listener
- `listener.disconnect` - Listener left
- `mount.created` - New mount added
- `stats.milestone` - Listener count milestone (100, 1000, etc.)

**Configuration**:
```vibe
webhooks {
    discord {
        url "https://discord.com/api/webhooks/..."
        events [source.connect, source.disconnect, stats.milestone]
        format discord  # or slack, generic
    }
    
    custom {
        url "https://your-api.com/webhook"
        events [*]
        headers {
            Authorization "Bearer your-token"
        }
        retry 3
        timeout 10s
    }
}
```

**Payload Example**:
```json
{
  "event": "source.connect",
  "timestamp": "2024-01-15T10:30:00Z",
  "mount": "/live",
  "data": {
    "source_ip": "192.168.1.100",
    "user_agent": "BUTT/0.1.34",
    "content_type": "audio/mpeg"
  }
}
```

---

### Embeddable Player Widget

**Pain Point**: Station operators want an easy way to embed players on their websites.

**Solution**: Ready-to-use embeddable player.

**Features**:
- Customizable colors/themes
- Responsive design
- Now playing display
- Volume control
- Mobile-friendly
- Copy-paste embed code

**Endpoint**:
```
GET /embed/live         → Full player
GET /embed/live/mini    → Mini player
GET /embed/live/code    → HTML embed code
```

**Embed Code**:
```html
<iframe 
  src="https://radio.example.com/embed/live" 
  width="300" 
  height="150" 
  frameborder="0">
</iframe>
```

**Customization**:
```
/embed/live?theme=dark&color=ff6600&autoplay=false
```

---

## Phase 8: Developer Experience

### Full REST API

**Pain Point**: Icecast admin API is limited and inconsistent. Many operations impossible via API.

**Solution**: Comprehensive REST API for everything.

**Endpoints**:

```
# Server
GET    /api/v1/server/status
GET    /api/v1/server/config
POST   /api/v1/server/reload

# Mounts
GET    /api/v1/mounts
POST   /api/v1/mounts
GET    /api/v1/mounts/:mount
PUT    /api/v1/mounts/:mount
DELETE /api/v1/mounts/:mount
POST   /api/v1/mounts/:mount/metadata

# Sources
GET    /api/v1/mounts/:mount/source
DELETE /api/v1/mounts/:mount/source

# Listeners
GET    /api/v1/mounts/:mount/listeners
GET    /api/v1/mounts/:mount/listeners/:id
DELETE /api/v1/mounts/:mount/listeners/:id

# Statistics
GET    /api/v1/stats
GET    /api/v1/stats/history
GET    /api/v1/stats/geo

# Recordings
GET    /api/v1/recordings
POST   /api/v1/recordings/start
POST   /api/v1/recordings/stop
DELETE /api/v1/recordings/:id
```

**Response Format**:
```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "abc123"
  }
}
```

---

### SDK & Client Libraries

**Pain Point**: Integrating with Icecast requires manual HTTP calls.

**Solution**: Official client libraries.

**Languages**:
- Go (reference implementation)
- Python
- JavaScript/TypeScript
- PHP
- Ruby

**Example (Python)**:
```python
from gocast import GoCastClient

client = GoCastClient("http://localhost:8000", api_key="gck_...")

# Get stats
stats = client.get_stats()
print(f"Total listeners: {stats.total_listeners}")

# Update metadata
client.mounts.update_metadata("/live", title="Now Playing: Great Song")

# List listeners
for listener in client.mounts.get_listeners("/live"):
    print(f"{listener.ip} - connected {listener.duration}s")
```

---

### OpenAPI/Swagger Documentation

**Pain Point**: Icecast API documentation is incomplete and outdated.

**Solution**: Auto-generated OpenAPI spec with Swagger UI.

**Endpoints**:
```
GET /api/v1/openapi.json  → OpenAPI 3.0 spec
GET /api/v1/docs          → Swagger UI
```

---

### CLI Tool

**Pain Point**: Managing Icecast from command line is cumbersome.

**Solution**: Powerful CLI for all operations.

**Commands**:
```bash
# Server management
gocast serve                    # Start server
gocast config check             # Validate config
gocast config reload            # Hot reload

# Mount management
gocast mount list               # List mounts
gocast mount create /new        # Create mount
gocast mount delete /old        # Delete mount
gocast mount stats /live        # Show mount stats

# Listener management
gocast listeners /live          # List listeners
gocast kick /live <id>          # Kick listener

# Source management
gocast source kill /live        # Kill source

# Statistics
gocast stats                    # Show stats
gocast stats --watch            # Live updating stats
gocast stats export --format csv > stats.csv

# Recordings
gocast record start /live       # Start recording
gocast record stop /live        # Stop recording
gocast record list              # List recordings
```

---

## Quick Wins

These are low-effort, high-value improvements that can be implemented quickly:

### 1. JSON Configuration Alternative
Some users prefer JSON over learning VIBE. Support both.

```bash
gocast --config config.json
gocast --config config.vibe
```

### 2. Better Error Messages
Replace cryptic errors with actionable messages.

```
# Before
Error: connection refused

# After  
Error: Could not connect to source
  → Mount /live is not accepting connections
  → Check that source_password is correct
  → Verify the mount exists in configuration
  → See: https://gocast.dev/docs/troubleshooting#source-refused
```

### 3. Docker Improvements
- Health check in Dockerfile
- Non-root user
- Smaller image (Alpine-based)
- docker-compose with Prometheus/Grafana

### 4. Systemd Service File
Ready-to-use service file for Linux servers.

### 5. Request Logging Improvements
- Request IDs for tracing
- Configurable log format
- Access log rotation

### 6. CORS Improvements
- Configurable origins
- Preflight caching
- Credentials support

---

## Implementation Status

| Feature | Status | Priority | Complexity | Target Version |
|---------|--------|----------|------------|----------------|
| **Phase 1: Modern Protocols** |
| HLS Support | 🔴 Not Started | P0 | High | v0.2 |
| DASH Support | 🔴 Not Started | P1 | High | v0.3 |
| WebSocket Streaming | 🔴 Not Started | P1 | Medium | v0.2 |
| **Phase 2: Monitoring** |
| WebSocket Dashboard | 🔴 Not Started | P0 | Medium | v0.2 |
| Prometheus Metrics | 🔴 Not Started | P0 | Low | v0.2 |
| Geo-IP Stats | 🔴 Not Started | P1 | Medium | v0.3 |
| Structured Logging | 🔴 Not Started | P2 | Low | v0.2 |
| **Phase 3: Operations** |
| Hot Config Reload | 🔴 Not Started | P0 | Medium | v0.2 |
| Graceful Shutdown | 🔴 Not Started | P1 | Low | v0.2 |
| Health Endpoints | 🔴 Not Started | P0 | Low | v0.1.1 |
| **Phase 4: Auth** |
| Listener Auth (Token) | 🔴 Not Started | P0 | Medium | v0.2 |
| Listener Auth (HTTP) | 🔴 Not Started | P1 | Medium | v0.2 |
| API Keys | 🔴 Not Started | P1 | Medium | v0.3 |
| OAuth2/OIDC | 🔴 Not Started | P2 | High | v0.4 |
| **Phase 5: Content** |
| Auto-Recording | 🔴 Not Started | P1 | Medium | v0.3 |
| Transcoding | 🔴 Not Started | P2 | High | v0.4 |
| Scheduled Switching | 🔴 Not Started | P2 | Medium | v0.3 |
| Album Art | 🔴 Not Started | P2 | Medium | v0.3 |
| **Phase 6: Clustering** |
| Relay Auto-Discovery | 🔴 Not Started | P2 | High | v0.4 |
| Multi-Server Sync | 🔴 Not Started | P3 | High | v0.5 |
| Geo Load Balancing | 🔴 Not Started | P3 | High | v0.5 |
| **Phase 7: Monetization** |
| Ad Insertion | 🔴 Not Started | P2 | High | v0.4 |
| Webhooks | 🔴 Not Started | P1 | Low | v0.2 |
| Embed Player | 🔴 Not Started | P1 | Medium | v0.3 |
| **Phase 8: Developer** |
| Full REST API | 🔴 Not Started | P0 | Medium | v0.2 |
| OpenAPI Docs | 🔴 Not Started | P1 | Low | v0.2 |
| CLI Tool | 🔴 Not Started | P2 | Medium | v0.3 |
| SDKs | 🔴 Not Started | P3 | Medium | v0.4+ |
| **Quick Wins** |
| JSON Config | 🔴 Not Started | P2 | Low | v0.1.1 |
| Better Errors | 🔴 Not Started | P1 | Low | v0.1.1 |
| Health Check | 🔴 Not Started | P0 | Low | v0.1.1 |

**Legend**:
- 🔴 Not Started
- 🟡 In Progress  
- 🟢 Complete
- P0 = Critical, P1 = High, P2 = Medium, P3 = Low

---

## Version Milestones

### v0.1.1 - Polish Release
- Health check endpoints
- Better error messages
- JSON config support
- Docker improvements

### v0.2 - Modern Streaming
- HLS support
- WebSocket dashboard
- Prometheus metrics
- Hot config reload
- Listener token auth
- Webhooks
- REST API v1

### v0.3 - Content & Engagement
- DASH support
- Auto-recording
- Album art metadata
- Embeddable player
- Geo-IP stats
- Scheduled switching
- CLI tool

### v0.4 - Enterprise Features
- Transcoding
- Ad insertion
- OAuth2/OIDC
- Relay auto-discovery
- API keys
- SDKs

### v0.5 - Scale
- Multi-server sync
- Geographic load balancing
- Advanced clustering

---

## Contributing

Want to help implement these features? Check out:

1. [CONTRIBUTING.md](../CONTRIBUTING.md) - Contribution guidelines
2. [Architecture](architecture.md) - Understanding the codebase
3. [GitHub Issues](https://github.com/1ay1/gocast/issues) - Current tasks

Features are tagged with `good-first-issue` for newcomers.

---

## Feedback

Have ideas for features not listed here? 

- Open a [GitHub Issue](https://github.com/1ay1/gocast/issues/new)
- Join our [Discord](https://discord.gg/gocast)
- Tweet us [@golocast](https://twitter.com/gocast)

---

*Last updated: 2024*