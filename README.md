<div align="center">

<img src="assets/logo.svg" alt="GoCast" width="400">

# Icecast, but modern.

**Web UI • JSON config • Single binary • No XML**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Icecast Compatible](https://img.shields.io/badge/Icecast-Compatible-blue?style=flat-square)](https://icecast.org)

</div>

---

If you're happy editing XML and restarting Icecast, this project is not for you.

---

## Why GoCast exists

Icecast works.

Icecast UX does not.

- XML config files from 2003
- Restart the server to change anything
- Admin panel that looks like a CVS receipt
- Documentation scattered across wikis and mailing lists

GoCast keeps the protocol, fixes the experience.

**No XML. No restart. No pain.**

---

## 60 seconds to streaming

```bash
git clone https://github.com/1ay1/gocast.git && cd gocast && go build -o gocast ./cmd/gocast && ./gocast
```

That's it. Open `http://localhost:8000/admin/` and start streaming.

---

## The Dashboard

<div align="center">
<img src="assets/admin_ss.png" alt="GoCast Dashboard" width="100%">

*Create mounts. Change settings. Add SSL. All without touching a config file or restarting anything.*
</div>

---

## What Icecast users actually wanted

| Pain | GoCast |
|------|--------|
| XML config hell | **JSON + Web Dashboard** |
| Restart to apply changes | **Hot reload everything** |
| Basic admin page | **Modern dashboard** |
| Complex setup | **Single binary, zero dependencies** |
| Manual SSL setup | **One-click AutoSSL** |
| Edit config via SSH | **Configure from your browser** |

---

## Compatible with everything you already use

<table>
<tr>
<td align="center"><strong>ffmpeg</strong></td>
<td align="center"><strong>BUTT</strong></td>
<td align="center"><strong>Mixxx</strong></td>
<td align="center"><strong>Liquidsoap</strong></td>
<td align="center"><strong>VLC</strong></td>
</tr>
</table>

If it works with Icecast, it works with GoCast. Drop-in compatible.

---

## Stream in 3 commands

**Start the server:**
```bash
./gocast
```

**Connect your source:**
```bash
ffmpeg -re -i music.mp3 -c:a libmp3lame -b:a 320k -f mp3 \
  icecast://source:password@localhost:8000/live
```

**Listen:**
```bash
mpv http://localhost:8000/live
```

---

## Features that matter

- 🎛️ **Web Dashboard** — Create mounts, manage listeners, rotate passwords. No terminal needed.
- 🔄 **Hot Reload** — Change any setting without restarting. Ever.
- 🔒 **One-click SSL** — AutoSSL with Let's Encrypt built in.
- 📊 **Live Stats** — See listeners per mount in real time.
- 🎧 **All Formats** — MP3, Ogg, Opus, AAC, FLAC.
- 🐳 **Docker Ready** — `docker run` and you're done.

---

## Install

### Single Binary
```bash
git clone https://github.com/1ay1/gocast.git
cd gocast
go build -o gocast ./cmd/gocast
./gocast
```

### Docker
```bash
docker build -t gocast .
docker run -p 8000:8000 -v ~/.gocast:/root/.gocast gocast
```

---

## Documentation

| Guide | What you'll learn |
|-------|-------------------|
| [Getting Started](docs/getting-started.md) | Zero to streaming in 60 seconds |
| [Dashboard Guide](docs/admin-panel.md) | Configure everything from your browser |
| [Sources](docs/sources.md) | Connect ffmpeg, BUTT, Liquidsoap, etc. |
| [SSL Setup](docs/ssl.md) | One-click HTTPS with AutoSSL |
| [API Reference](docs/api.md) | REST API for automation |

---

## The uncomfortable truth

Your Icecast setup works fine.

You just hate touching it.

GoCast is for people who want streaming infrastructure they can actually manage without reading a wiki from 2008.

---

<div align="center">

## Ready to stop fighting XML?

**[⭐ Star on GitHub](https://github.com/1ay1/gocast)** if you've ever rage-quit an Icecast config.

[🐛 Report Bug](https://github.com/1ay1/gocast/issues) • [💡 Request Feature](https://github.com/1ay1/gocast/issues)

MIT License • Made with Go

</div>