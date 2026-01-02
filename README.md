<div align="center">

```
   ██████╗  ██████╗  ██████╗ █████╗ ███████╗████████╗
  ██╔════╝ ██╔═══██╗██╔════╝██╔══██╗██╔════╝╚══██╔══╝
  ██║  ███╗██║   ██║██║     ███████║███████╗   ██║
  ██║   ██║██║   ██║██║     ██╔══██║╚════██║   ██║
  ╚██████╔╝╚██████╔╝╚██████╗██║  ██║███████║   ██║
   ╚═════╝  ╚═════╝  ╚═════╝╚═╝  ╚═╝╚══════╝   ╚═╝
```

# 🎵 GoCast

### A Modern, Drop-in Replacement for Icecast

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![Icecast Compatible](https://img.shields.io/badge/Icecast-Compatible-blue?style=for-the-badge)](https://icecast.org)

**Stream audio to thousands of listeners with a single binary. No dependencies. No complexity.**

[Getting Started](#-quick-start) •
[Documentation](docs/) •
[Configuration](#-configuration) •
[API Reference](docs/admin-api.md)

</div>

---

## ⚡ Why GoCast?

| Feature | Icecast | GoCast |
|---------|---------|--------|
| Language | C | **Go** |
| Config Format | XML 😱 | **[VIBE](https://github.com/1ay1/vibe)** 🌊 |
| Memory Safety | Manual | **Automatic** |
| Single Binary | ❌ | **✅** |
| Docker Ready | Requires setup | **Native** |
| CORS Support | Manual | **Built-in** |
| Modern Codebase | 20+ years old | **Fresh & Clean** |

## ✨ Features

- 🔌 **100% Icecast Compatible** - Works with FFmpeg, BUTT, Liquidsoap, Mixxx, and all Icecast clients
- 🎧 **Multi-Format Support** - MP3, Ogg Vorbis, Opus, AAC, FLAC, and more
- 📊 **ICY Metadata** - Real-time "Now Playing" updates to all listeners
- 🔀 **Multiple Mounts** - Host unlimited streams on a single server
- 🛡️ **Built-in Security** - Authentication, IP filtering, SSL/TLS
- 📈 **Live Statistics** - JSON/XML API compatible with existing tools
- 🎛️ **Web Admin Panel** - Manage everything from your browser
- 🐳 **Docker Ready** - Deploy anywhere in seconds

## 🚀 Quick Start

### One-liner Install

```bash
git clone https://github.com/1ay1/gocast.git && cd gocast && go build -o gocast ./cmd/gocast && ./gocast
```

### What You'll See

```
   ██████╗  ██████╗  ██████╗ █████╗ ███████╗████████╗
  ██╔════╝ ██╔═══██╗██╔════╝██╔══██╗██╔════╝╚══██╔══╝
  ██║  ███╗██║   ██║██║     ███████║███████╗   ██║
  ██║   ██║██║   ██║██║     ██╔══██║╚════██║   ██║
  ╚██████╔╝╚██████╔╝╚██████╗██║  ██║███████║   ██║
   ╚═════╝  ╚═════╝  ╚═════╝╚═╝  ╚═╝╚══════╝   ╚═╝

  Modern Icecast Replacement - v1.0.0
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[GoCast] Loading configuration from gocast.vibe
[GoCast] GoCast is running on http://localhost:8000
[GoCast] Admin interface: http://localhost:8000/admin/
[GoCast] Starting GoCast HTTP server on 0.0.0.0:8000
```

### Start Streaming

```bash
# Stream with FFmpeg
ffmpeg -re -i music.mp3 -c:a libmp3lame -b:a 320k -f mp3 \
  icecast://source:hackme@localhost:8000/live

# Listen
mpv http://localhost:8000/live
```

**That's it!** 🎉

## 📦 Installation

### From Source

```bash
git clone https://github.com/1ay1/gocast.git
cd gocast
go build -o gocast ./cmd/gocast
```

### Docker

```bash
docker build -t gocast .
docker run -p 8000:8000 gocast
```

### Docker Compose

```bash
docker-compose up -d
```

## 🔧 Configuration

GoCast uses [VIBE](https://github.com/1ay1/vibe) - a human-friendly config format. No more XML nightmares!

```vibe
# gocast.vibe - Simple and clean!

server {
    hostname myradio.example.com
    port 8000
}

auth {
    source_password super_secret_password
    admin_user admin
    admin_password admin_password
}

mounts {
    live {
        stream_name "My Awesome Radio"
        genre "Electronic"
        description "24/7 Best Beats"
        max_listeners 1000
        bitrate 320
    }
}
```

📖 [Full Configuration Reference →](docs/configuration.md)

## 🎙️ Connect Your Source

### FFmpeg

```bash
ffmpeg -re -i playlist.m3u -c:a libmp3lame -b:a 320k -f mp3 \
  icecast://source:password@localhost:8000/live
```

### BUTT (Broadcast Using This Tool)

1. Server Type: **Icecast**
2. Address: `localhost`
3. Port: `8000`
4. Password: `hackme`
5. Mount: `/live`

### Liquidsoap

```liquidsoap
output.icecast(%mp3(bitrate=320),
  host="localhost", port=8000,
  password="hackme", mount="/live",
  source)
```

📖 [All Source Clients →](docs/sources.md)

## 👂 Listen

| Player | Command |
|--------|---------|
| **Browser** | `http://localhost:8000/live` |
| **VLC** | `vlc http://localhost:8000/live` |
| **mpv** | `mpv http://localhost:8000/live` |
| **curl** | `curl http://localhost:8000/live -o recording.mp3` |

## 📊 API & Monitoring

### Status Page
```
http://localhost:8000/          → HTML status page
http://localhost:8000/status?format=json  → JSON API
http://localhost:8000/status?format=xml   → XML (Icecast compatible)
```

### Admin Panel
```
http://localhost:8000/admin/    → Web interface
```

### Admin API

```bash
# Update now playing
curl -u admin:hackme "http://localhost:8000/admin/metadata?mount=/live&mode=updinfo&song=Artist%20-%20Song"

# List listeners
curl -u admin:hackme "http://localhost:8000/admin/listclients?mount=/live"

# Kick a listener
curl -u admin:hackme "http://localhost:8000/admin/killclient?mount=/live&id=UUID"
```

📖 [Full API Reference →](docs/admin-api.md)

## 📚 Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Installation and first steps |
| [Configuration](docs/configuration.md) | Complete config reference |
| [Streaming Sources](docs/sources.md) | FFmpeg, BUTT, Liquidsoap, etc. |
| [Listeners](docs/listeners.md) | Client compatibility and features |
| [Admin API](docs/admin-api.md) | REST API documentation |
| [Architecture](docs/architecture.md) | Internal design and data flow |
| [VIBE Format](docs/vibe.md) | Configuration format guide |

## 🏗️ Project Structure

```
gocast/
├── cmd/gocast/          # Application entry point
├── internal/
│   ├── auth/           # Authentication
│   ├── config/         # Configuration parsing
│   ├── server/         # HTTP server & routing
│   ├── source/         # Source client handling
│   ├── stats/          # Statistics collection
│   └── stream/         # Buffer & mount management
├── pkg/vibe/           # VIBE config parser
├── docs/               # Documentation
├── gocast.vibe         # Example configuration
├── Dockerfile          # Container build
└── docker-compose.yml  # Container orchestration
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📜 License

MIT License - see [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- Inspired by [Icecast](https://icecast.org/) - the original open source streaming server
- Configuration powered by [VIBE](https://github.com/1ay1/vibe) - human-friendly config format

---

<div align="center">

**⭐ Star this repo if GoCast helps you stream!**

Made with ❤️ and Go

[Report Bug](https://github.com/1ay1/gocast/issues) •
[Request Feature](https://github.com/1ay1/gocast/issues)

</div>