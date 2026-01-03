# GoCast Documentation

GoCast is a modern, high-performance audio streaming server written in Go. It's designed as a drop-in replacement for Icecast with a focus on simplicity and ease of use.

## Quick Start

```bash
# Download and run
./gocast

# First run shows admin credentials:
#   Admin Username: admin
#   Admin Password: <generated>
#
# Open http://localhost:8000/admin/ to configure
```

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](getting-started.md) | Installation and first steps |
| [Configuration](configuration.md) | Config file reference |
| [Admin Panel](admin-panel.md) | Web-based administration |
| [Sources](sources.md) | Connecting broadcasting software |
| [Listeners](listeners.md) | Client connections and playback |
| [SSL/HTTPS](ssl.md) | Securing your streams |
| [API Reference](api.md) | Admin REST API |

## Key Features

- **Zero Configuration** - Works out of the box with sensible defaults
- **Web Admin Panel** - Configure everything from your browser
- **Hot Reload** - Most settings apply immediately without restart
- **Auto Recovery** - Corrupted config? Automatic backup and recovery
- **Icecast Compatible** - Works with existing streaming software
- **Modern Security** - AutoSSL with Let's Encrypt, secure defaults

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      GoCast Server                       │
├─────────────────────────────────────────────────────────┤
│                                                          │
│   ┌──────────┐    ┌──────────┐    ┌──────────────────┐  │
│   │  Source  │───▶│  Mount   │───▶│    Listeners     │  │
│   │ (ffmpeg) │    │  (/live) │    │ (browsers/apps)  │  │
│   └──────────┘    └──────────┘    └──────────────────┘  │
│                                                          │
│   ┌──────────────────────────────────────────────────┐  │
│   │              Admin Panel (/admin/)                │  │
│   │  • Settings    • Mounts    • Listeners    • Logs │  │
│   └──────────────────────────────────────────────────┘  │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Configuration

All settings are stored in a single JSON file:

```
~/.gocast/config.json
```

You can:
1. **Edit via Admin Panel** (recommended) - Changes apply immediately
2. **Edit the file directly** - Send SIGHUP to reload

## Default Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8000 | HTTP | Main server (streams + admin) |
| 8443 | HTTPS | SSL/TLS (when enabled) |
| 80 | HTTP | AutoSSL verification (Let's Encrypt) |

## Support

- GitHub: https://github.com/gocast/gocast
- Issues: https://github.com/gocast/gocast/issues