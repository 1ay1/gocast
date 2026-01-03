<div align="center">

<img src="assets/logo.svg" alt="GoCast" width="400">

### 🎵 The Modern Audio Streaming Server You've Been Waiting For

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![Icecast Compatible](https://img.shields.io/badge/Icecast-Compatible-blue?style=for-the-badge)](https://icecast.org)
[![Stars](https://img.shields.io/github/stars/1ay1/gocast?style=for-the-badge&logo=github&color=yellow)](https://github.com/1ay1/gocast)

**🚀 Stream audio to thousands of listeners with a single binary. No dependencies. No complexity. Just works.**

[🎯 Quick Start](#-quick-start) •
[📖 Documentation](docs/) •
[⚙️ Configuration](#-configuration) •
[🔌 API Reference](docs/api.md)

---

### ✨ Beautiful, Powerful Admin Panel

<img src="assets/admin_ss.png" alt="GoCast Admin Panel" width="100%">

*Configure everything from your browser — mounts, SSL, authentication, and more. No restarts needed!*

</div>

---

## ⚡ Why GoCast?

> **"Finally, an Icecast replacement that doesn't make me want to cry."** — Every radio operator, probably

| Feature | Icecast | GoCast |
|---------|---------|--------|
| Language | C | **Go** 🦫 |
| Config Format | XML 😱 | **JSON + Web UI** 🎛️ |
| Memory Safety | Manual | **Automatic** ✅ |
| Single Binary | ❌ | **✅ Zero Dependencies** |
| Web Admin Panel | Basic | **Modern & Full Featured** 🔥 |
| Hot Reload Config | ❌ | **✅ No Restarts** |
| Docker Ready | Requires setup | **Native** 🐳 |
| CORS Support | Manual | **Built-in** |
| Setup Time | Hours | **60 Seconds** ⚡ |

## ✨ Features

- 🔌 **100% Icecast Compatible** - Works with FFmpeg, BUTT, Liquidsoap, Mixxx, and all Icecast clients
- 🎧 **Multi-Format Support** - MP3, Ogg Vorbis, Opus, AAC, FLAC, and more
- 📊 **ICY Metadata** - Real-time "Now Playing" updates to all listeners
- 🔀 **Multiple Mounts** - Host unlimited streams on a single server
- 🛡️ **Built-in Security** - Authentication, IP filtering, SSL/TLS with **free AutoSSL**
- 📈 **Live Statistics** - JSON/XML API compatible with existing tools
- 🎛️ **Gorgeous Admin Panel** - Configure everything from your browser — no restart needed!
- 🔄 **Hot Reload** - All settings apply immediately without restart
- 🐳 **Docker Ready** - Deploy anywhere in seconds
- 🧠 **Smart Defaults** - Works out of the box, customize when you need to

## 🚀 Quick Start

### ⚡ One-liner Install (60 seconds to streaming!)

```bash
git clone https://github.com/1ay1/gocast.git && cd gocast && go build -o gocast ./cmd/gocast && ./gocast
```

> 💡 **Finding your password later:** `cat ~/.gocast/config.json | grep admin_password`

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

[GoCast] Starting in zero-config mode...
╔════════════════════════════════════════════════════════════╗
║              GOCAST FIRST-RUN SETUP                        ║
╠════════════════════════════════════════════════════════════╣
║  Admin Username: admin                                     ║
║  Admin Password: xK9mP2vL8nQ4wR6j                          ║
║                                                            ║
║  SAVE THIS PASSWORD - IT WON'T BE SHOWN AGAIN!             ║
║                                                            ║
║  Open admin panel to complete setup and configure SSL      ║
╚════════════════════════════════════════════════════════════╝
[GoCast] GoCast is running on http://localhost:8000
[GoCast] Admin panel: http://localhost:8000/admin/
```

### Start Streaming

```bash
# Stream with FFmpeg
ffmpeg -re -i music.mp3 -c:a libmp3lame -b:a 320k -f mp3 \
  icecast://source:YOUR_SOURCE_PASSWORD@localhost:8000/live

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
docker run -p 8000:8000 -v ~/.gocast:/root/.gocast gocast
```

### Docker Compose

```bash
docker-compose up -d
```

## 🔧 Configuration

### How It Works

GoCast stores all configuration in a single JSON file (`~/.gocast/config.json`). 

**All settings are managed through the Web Admin Panel** - just like Openfire, phpMyAdmin, or other modern server software. No need to edit config files manually!

### First Run

On first start, GoCast will:
1. **Generate secure credentials** - Admin and source passwords shown once in console
2. **Create a default `/live` mount** - Ready for streaming immediately
3. **Start the admin panel** - Available at `http://localhost:8000/admin/`

### Admin Panel Features

The web-based admin panel lets you configure everything:

- **Server Settings** - Hostname, port, location, server ID
- **SSL/TLS** - One-click AutoSSL with Let's Encrypt, or manual certificates
- **Limits** - Max clients, sources, buffer sizes, timeouts
- **Authentication** - Admin and source passwords
- **Mount Points** - Create, edit, delete streams with full metadata
- **Logging** - Log level, file paths, buffer sizes
- **Directory** - Yellow Pages / directory listing settings

**All changes apply immediately** - no server restart required!

### Manual Configuration (Power Users)

If you prefer, you can edit the config file directly:

**Location:** `~/.gocast/config.json`

```json
{
  "version": 1,
  "setup_complete": true,
  "server": {
    "hostname": "radio.example.com",
    "listen_address": "0.0.0.0",
    "port": 8000,
    "location": "New York, USA",
    "server_id": "MyRadio"
  },
  "ssl": {
    "enabled": true,
    "auto_ssl": true,
    "port": 8443,
    "auto_ssl_email": "admin@example.com"
  },
  "limits": {
    "max_clients": 500,
    "max_sources": 10,
    "queue_size": 131072,
    "burst_size": 65536
  },
  "auth": {
    "source_password": "your-source-password",
    "admin_user": "admin",
    "admin_password": "your-admin-password"
  },
  "mounts": {
    "/live": {
      "name": "/live",
      "max_listeners": 100,
      "genre": "Various",
      "description": "24/7 Radio",
      "public": true
    }
  }
}
```

After editing, use one of these methods to reload:
- **Admin Panel:** Click "Reload from Disk" button
- **Signal:** Send `SIGHUP` to the process (`kill -HUP <pid>`)

### Command Line Options

```
./gocast [OPTIONS]

OPTIONS:
    -data <dir>    Data directory for config (default: ~/.gocast)
    -version       Show version information
    -help          Show help message

SIGNALS:
    SIGINT, SIGTERM   Graceful shutdown
    SIGHUP            Hot reload configuration
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
4. Password: *(your source password)*
5. Mount: `/live`

### Liquidsoap

```liquidsoap
output.icecast(%mp3(bitrate=320),
  host="localhost", port=8000,
  password="your-source-password", mount="/live",
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
http://localhost:8000/admin/    → Web interface (full configuration)
```

### Admin API

```bash
# Update now playing
curl -u admin:password "http://localhost:8000/admin/metadata?mount=/live&mode=updinfo&song=Artist%20-%20Song"

# List listeners
curl -u admin:password "http://localhost:8000/admin/listclients?mount=/live"

# Kick a listener
curl -u admin:password "http://localhost:8000/admin/killclient?mount=/live&id=UUID"

# Get current config
curl -u admin:password "http://localhost:8000/admin/config"
```

📖 [Full API Reference →](docs/api.md)

## 📚 Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Installation and first steps |
| [Configuration](docs/configuration.md) | Complete config reference |
| [Admin Panel](docs/admin-panel.md) | Web-based administration |
| [Sources](docs/sources.md) | FFmpeg, BUTT, Liquidsoap, etc. |
| [Listeners](docs/listeners.md) | Client compatibility and features |
| [SSL/HTTPS](docs/ssl.md) | Securing your streams |
| [API Reference](docs/api.md) | REST API documentation |

## 🏗️ Project Structure

```
gocast/
├── cmd/gocast/          # Application entry point
├── internal/
│   ├── auth/           # Authentication
│   ├── config/         # Configuration management (JSON)
│   ├── server/         # HTTP server & admin panel
│   ├── source/         # Source client handling
│   ├── stats/          # Statistics collection
│   └── stream/         # Buffer & mount management
├── docs/               # Documentation
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

---

<div align="center">

## 💖 Love GoCast? Give us a star!

**⭐ Star this repo if GoCast helps you stream!**

Your support helps us keep building awesome open source software.

Made with ❤️ and Go

[🐛 Report Bug](https://github.com/1ay1/gocast/issues) •
[💡 Request Feature](https://github.com/1ay1/gocast/issues) •
[⭐ Star on GitHub](https://github.com/1ay1/gocast)

</div>
