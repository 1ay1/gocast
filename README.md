# GoCast 🎵

A modern, drop-in replacement for Icecast written in Go. GoCast provides a high-performance audio streaming server with full compatibility with existing Icecast source clients and listeners.

## Features

- **Full Icecast Compatibility** - Works with existing source clients (Liquidsoap, BUTT, Mixxx, etc.) and all audio players
- **Multiple Format Support** - MP3, Ogg Vorbis, Opus, AAC, FLAC, and more
- **ICY Metadata** - Full support for in-stream metadata (now playing info)
- **Multiple Mount Points** - Host multiple streams on a single server
- **Admin Interface** - Web-based administration panel
- **Statistics API** - JSON/XML compatible with Icecast stats format
- **SSL/TLS Support** - Secure streaming with HTTPS
- **CORS Support** - Built-in support for web-based players
- **Low Resource Usage** - Efficient Go implementation
- **Modern Configuration** - Uses the human-friendly VIBE config format

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/gocast/gocast.git
cd gocast

# Build
go build -o gocast ./cmd/gocast

# Run with default configuration
./gocast
```

### Docker

```bash
docker run -p 8000:8000 gocast/gocast
```

### Configuration

GoCast uses the [VIBE](https://github.com/1ay1/vibe) configuration format. Create a `gocast.vibe` file:

```vibe
# GoCast Configuration

server {
    hostname localhost
    port 8000
    location "My Radio Station"
}

auth {
    source_password your_source_password
    admin_user admin
    admin_password your_admin_password
}

limits {
    max_clients 100
    max_sources 10
}

mounts {
    live {
        stream_name "Live Stream"
        genre "Music"
        description "24/7 Live Radio"
        type audio/mpeg
        public true
    }
}
```

Run GoCast with your configuration:

```bash
./gocast -config gocast.vibe
```

## Streaming to GoCast

### Using BUTT (Broadcast Using This Tool)

1. Open BUTT settings
2. Set Server Type to "Icecast"
3. Address: `localhost`
4. Port: `8000`
5. Password: Your source password
6. Mount: `/live`

### Using Liquidsoap

```liquidsoap
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="your_source_password",
  mount="/live",
  source
)
```

### Using FFmpeg

```bash
ffmpeg -re -i input.mp3 -c:a libmp3lame -b:a 128k \
  -f mp3 icecast://source:your_source_password@localhost:8000/live
```

### Using cURL (HTTP PUT)

```bash
cat audio.mp3 | curl -X PUT -H "Authorization: Basic $(echo -n source:password | base64)" \
  -H "Content-Type: audio/mpeg" \
  --data-binary @- http://localhost:8000/live
```

## Listening

### Direct URL

```
http://localhost:8000/live
```

### With Metadata Support

Add the `Icy-MetaData: 1` header to receive inline metadata:

```bash
curl -H "Icy-MetaData: 1" http://localhost:8000/live
```

### Web Players

GoCast includes CORS headers for web-based players:

```html
<audio src="http://localhost:8000/live" controls></audio>
```

## API Endpoints

### Status

| Endpoint | Description |
|----------|-------------|
| `GET /` | HTML status page |
| `GET /status` | HTML status page |
| `GET /status?format=json` | JSON status |
| `GET /status?format=xml` | XML status (Icecast compatible) |

### Admin (requires authentication)

| Endpoint | Description |
|----------|-------------|
| `GET /admin/` | Admin dashboard |
| `GET /admin/stats` | Server statistics (XML) |
| `GET /admin/listmounts` | List all mounts |
| `GET /admin/listclients?mount=/live` | List listeners |
| `GET /admin/metadata?mount=/live&mode=updinfo&song=Title` | Update metadata |
| `GET /admin/killclient?mount=/live&id=xxx` | Disconnect listener |
| `GET /admin/killsource?mount=/live` | Disconnect source |

## Configuration Reference

### Server Section

```vibe
server {
    hostname localhost        # Server hostname
    listen 0.0.0.0           # Listen address
    port 8000                # HTTP port
    ssl_port 8443            # HTTPS port
    location "Earth"         # Server location
    server_id GoCast         # Server identifier
    admin_root /admin        # Admin URL path

    ssl {
        enabled false
        cert /path/to/cert.crt
        key /path/to/key.key
    }
}
```

### Authentication Section

```vibe
auth {
    source_password hackme    # Default source password
    relay_password ""         # Relay connection password
    admin_user admin          # Admin username
    admin_password hackme     # Admin password
}
```

### Limits Section

```vibe
limits {
    max_clients 100              # Max total connections
    max_sources 10               # Max source connections
    max_listeners_per_mount 100  # Max listeners per mount
    queue_size 524288            # Buffer size (bytes)
    burst_size 65535             # Initial burst (bytes)
    client_timeout 30            # Listener timeout (seconds)
    header_timeout 15            # Header read timeout
    source_timeout 10            # Source timeout
}
```

### Mount Section

```vibe
mounts {
    live {
        password secret           # Mount-specific password
        max_listeners 100
        fallback /fallback        # Fallback mount
        stream_name "My Stream"
        genre "Various"
        description "Description"
        url "http://example.com"
        bitrate 128
        type audio/mpeg
        public true
        hidden false
        burst_size 65535
        max_listener_duration 0   # 0 = unlimited
        allowed_ips [192.168.1.*]
        denied_ips [10.0.0.1]
        dump_file /path/to/dump.mp3
    }
}
```

## Signals

| Signal | Action |
|--------|--------|
| `SIGINT` / `SIGTERM` | Graceful shutdown |
| `SIGHUP` | Reload configuration |

## Comparison with Icecast

| Feature | GoCast | Icecast |
|---------|--------|---------|
| Language | Go | C |
| Config Format | VIBE | XML |
| Memory Safety | Yes | Manual |
| Concurrency | Goroutines | Threads |
| Hot Reload | Partial | No |
| CORS | Built-in | Manual |
| Docker | Native | Requires setup |
| WebSocket | Planned | No |

## Building from Source

### Requirements

- Go 1.22 or later

### Build

```bash
# Standard build
go build -o gocast ./cmd/gocast

# With optimizations
go build -ldflags="-s -w" -o gocast ./cmd/gocast

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o gocast-linux ./cmd/gocast
```

### Testing

```bash
go test ./...
```

## Project Structure

```
gocast/
├── cmd/gocast/           # Main application
├── internal/
│   ├── config/          # Configuration handling
│   ├── server/          # HTTP server & listeners
│   ├── source/          # Source client handling
│   ├── stream/          # Stream buffer & mounts
│   ├── stats/           # Statistics collection
│   └── auth/            # Authentication
├── pkg/vibe/            # VIBE config parser
├── web/
│   ├── templates/       # HTML templates
│   └── static/          # Static assets
├── gocast.vibe          # Example configuration
└── README.md
```

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting a PR.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Acknowledgments

- Inspired by [Icecast](https://icecast.org/)
- Configuration format: [VIBE](https://github.com/1ay1/vibe)
- Thanks to all contributors!

---

**GoCast** - *Stream with confidence* 🎵