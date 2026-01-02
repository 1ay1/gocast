# GoCast Documentation

Welcome to the GoCast documentation. GoCast is a modern, drop-in replacement for Icecast written in Go.

## Table of Contents

- [Getting Started](getting-started.md) - Installation and quick start guide
- [Configuration](configuration.md) - Complete configuration reference
- [Streaming Sources](sources.md) - How to connect source clients (FFmpeg, BUTT, Liquidsoap, etc.)
- [Listeners](listeners.md) - Listener connections and client compatibility
- [Admin API](admin-api.md) - Administration endpoints and management
- [Architecture](architecture.md) - Internal architecture and design
- [VIBE Config Format](vibe.md) - VIBE configuration format documentation
- [Roadmap](roadmap.md) - Planned features and implementation status

## Overview

GoCast is designed to be a modern replacement for Icecast with the following goals:

- **Full Icecast Compatibility** - Works with existing source clients and listeners
- **Modern Codebase** - Written in Go for performance and safety
- **Simple Configuration** - Uses the human-friendly VIBE config format
- **Easy Deployment** - Single binary, Docker support, minimal dependencies

## Quick Links

| Resource | Description |
|----------|-------------|
| [GitHub Repository](https://github.com/1ay1/gocast) | Source code and issues |
| [Configuration Example](../gocast.vibe) | Sample configuration file |
| [Dockerfile](../Dockerfile) | Docker container build |

## Features

### Streaming
- Multiple mount points
- MP3, Ogg Vorbis, Opus, AAC, FLAC support
- ICY metadata (now playing info)
- Configurable buffer and burst sizes

### Security
- Source authentication
- Admin authentication
- IP-based access control
- SSL/TLS support

### Monitoring
- Real-time statistics
- JSON/XML status API (Icecast compatible)
- Web-based admin interface
- Listener tracking

## License

GoCast is released under the MIT License.