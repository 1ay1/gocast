# Getting Started with GoCast

This guide will help you get GoCast up and running in minutes.

## Installation

### Option 1: Download Binary

```bash
# Download the latest release
curl -LO https://github.com/gocast/gocast/releases/latest/download/gocast-linux-amd64
chmod +x gocast-linux-amd64
mv gocast-linux-amd64 /usr/local/bin/gocast
```

### Option 2: Build from Source

```bash
git clone https://github.com/gocast/gocast.git
cd gocast
go build -o gocast ./cmd/gocast
```

### Option 3: Docker

```bash
docker run -p 8000:8000 -v gocast-data:/data gocast/gocast
```

## First Run

Simply run GoCast:

```bash
./gocast
```

On first start, you'll see:

```
╔════════════════════════════════════════════════════════════╗
║              GOCAST FIRST-RUN SETUP                        ║
╠════════════════════════════════════════════════════════════╣
║  Admin Username: admin                                     ║
║  Admin Password: xK9mP2nQ7vL4    <-- SAVE THIS!           ║
║                                                            ║
║  ⚠️  SAVE THIS PASSWORD - IT WON'T BE SHOWN AGAIN!         ║
╚════════════════════════════════════════════════════════════╝

GoCast is running on http://localhost:8000
Admin panel: http://localhost:8000/admin/
```

**Important:** Save the admin password! You can also find it later in:
```bash
cat ~/.gocast/config.json | grep admin_password
```

## Access the Admin Panel

Open your browser to: **http://localhost:8000/admin/**

Log in with:
- Username: `admin`
- Password: (the password shown on first run)

## Connect a Source (Broadcaster)

Use any Icecast-compatible software to stream audio:

### Using FFmpeg

```bash
ffmpeg -re -i music.mp3 \
  -c:a libmp3lame -b:a 128k \
  -content_type audio/mpeg \
  -f mp3 icecast://source:hackme@localhost:8000/live
```

### Using Butt (Broadcast Using This Tool)

1. Open Butt
2. Settings → Main → Server:
   - Type: Icecast
   - Address: localhost
   - Port: 8000
   - Password: `hackme` (or your source password)
   - Mount: `/live`

### Using Mixxx

1. Preferences → Live Broadcasting
2. Type: Icecast 2
3. Host: localhost
4. Port: 8000
5. Mount: /live
6. Login: source
7. Password: (your source password)

## Listen to the Stream

Once a source is connected, listeners can tune in:

### Direct URL
```
http://localhost:8000/live
```

### In VLC
File → Open Network Stream → `http://localhost:8000/live`

### In Browser
Most browsers can play the stream directly by visiting the URL.

## Configuration

All settings are in `~/.gocast/config.json`:

```json
{
  "server": {
    "hostname": "localhost",
    "port": 8000
  },
  "auth": {
    "source_password": "hackme",
    "admin_user": "admin",
    "admin_password": "your-password"
  },
  "mounts": {
    "/live": {
      "name": "/live",
      "password": "optional-mount-specific-password",
      "max_listeners": 100,
      "bitrate": 128,
      "type": "audio/mpeg"
    }
  }
}
```

### Change Settings

**Option 1: Admin Panel (Recommended)**
- Go to http://localhost:8000/admin/
- Navigate to Settings
- Changes apply immediately

**Option 2: Edit Config File**
```bash
nano ~/.gocast/config.json
# After editing, reload without restart:
kill -HUP $(pgrep gocast)
```

## Custom Data Directory

By default, GoCast stores data in `~/.gocast/`. To use a different location:

```bash
./gocast -data /var/lib/gocast
```

## Next Steps

- [Configuration Reference](configuration.md) - Full config file documentation
- [Admin Panel Guide](admin-panel.md) - Using the web interface
- [SSL Setup](ssl.md) - Enable HTTPS with AutoSSL
- [Sources Guide](sources.md) - Detailed source connection info