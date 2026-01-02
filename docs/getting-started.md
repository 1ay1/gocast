# Getting Started with GoCast

GoCast is a modern, drop-in replacement for Icecast written in Go. This guide will help you get up and running in minutes.

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/1ay1/gocast.git
cd gocast

# Build
go build -o gocast ./cmd/gocast

# Run
./gocast
```

### Using Docker

```bash
# Build image
docker build -t gocast .

# Run container
docker run -p 8000:8000 gocast
```

### Using Docker Compose

```bash
docker-compose up -d
```

## Quick Start

### 1. Start the Server

```bash
./gocast
```

You'll see the GoCast banner and the server will start on port 8000 by default.

### 2. Stream Audio (Source)

Use any Icecast-compatible source client:

#### FFmpeg
```bash
ffmpeg -re -i your_audio.mp3 -c:a libmp3lame -b:a 128k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

#### BUTT (Broadcast Using This Tool)
1. Open BUTT settings
2. Server Type: Icecast
3. Address: `localhost`
4. Port: `8000`
5. Password: `hackme`
6. Mount: `/live`

#### Liquidsoap
```liquidsoap
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  source
)
```

### 3. Listen to the Stream

#### Web Browser
Open `http://localhost:8000/live` in any browser or media player.

#### VLC
```bash
vlc http://localhost:8000/live
```

#### mpv
```bash
mpv http://localhost:8000/live
```

#### curl
```bash
curl http://localhost:8000/live -o recording.mp3
```

## Default Configuration

GoCast comes with sensible defaults:

| Setting | Default Value |
|---------|---------------|
| HTTP Port | 8000 |
| Source Password | `hackme` |
| Admin User | `admin` |
| Admin Password | `hackme` |
| Max Clients | 100 |
| Max Sources | 10 |

## Web Interfaces

### Status Page
`http://localhost:8000/` - Shows all active streams and listener counts

### Admin Panel
`http://localhost:8000/admin/` - Server administration (requires login)

### API Endpoints
- `http://localhost:8000/status?format=json` - JSON status
- `http://localhost:8000/status?format=xml` - XML status (Icecast compatible)

## Custom Configuration

Create a `gocast.vibe` file:

```vibe
server {
    hostname myradio.example.com
    port 8000
}

auth {
    source_password your_secure_password
    admin_user admin
    admin_password your_admin_password
}

mounts {
    live {
        stream_name "My Radio Station"
        genre "Music"
        description "24/7 Live Radio"
        max_listeners 500
    }
}
```

Run with your config:
```bash
./gocast -config gocast.vibe
```

## Next Steps

- [Configuration Reference](configuration.md) - Full configuration options
- [Streaming Guide](streaming.md) - Detailed streaming setup
- [API Reference](api.md) - REST API documentation
- [Admin Guide](admin.md) - Server administration