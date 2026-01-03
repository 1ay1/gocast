# Listeners Guide

Listeners are clients that connect to GoCast to receive audio streams. This guide covers how listeners connect and how to optimize the listening experience.

## Connecting to a Stream

### Direct URL

The simplest way to listen is via direct URL:

```
http://your-server:8000/mount-path
```

Examples:
```
http://localhost:8000/live
http://radio.example.com:8000/music
https://radio.example.com:8443/live  (with SSL)
```

### Supported Players

GoCast works with any player that supports HTTP audio streams:

| Player | Platform | Notes |
|--------|----------|-------|
| VLC | All | Excellent compatibility |
| foobar2000 | Windows | Lightweight option |
| Clementine | Linux/Mac/Win | Good metadata support |
| iTunes | Mac/Windows | Works with MP3 streams |
| Winamp | Windows | Classic player |
| mpv | All | Command-line player |
| Browser | All | Most browsers play directly |

### Command Line Players

**mpv:**
```bash
mpv http://localhost:8000/live
```

**ffplay:**
```bash
ffplay http://localhost:8000/live
```

**VLC:**
```bash
vlc http://localhost:8000/live
```

**curl (save to file):**
```bash
curl http://localhost:8000/live > stream.mp3
```

### Browser Playback

Most modern browsers can play streams directly. Simply navigate to:
```
http://localhost:8000/live
```

For embedded playback in a webpage:
```html
<audio controls autoplay>
  <source src="http://localhost:8000/live" type="audio/mpeg">
  Your browser does not support audio playback.
</audio>
```

## Stream Metadata

GoCast supports ICY metadata, which allows players to display:

- **Stream Title** - Current song/show name
- **Artist** - Artist name
- **Album** - Album name
- **Genre** - Stream genre
- **Description** - Stream description

### Requesting Metadata

Players request metadata by sending the header:
```
Icy-MetaData: 1
```

GoCast responds with:
```
icy-metaint: 16000
```

This indicates metadata is embedded every 16000 bytes.

### Metadata Update

Sources can update metadata in real-time. Listeners see updates within seconds.

## Playlist Files

GoCast can serve playlist files for easy one-click listening.

### M3U Playlist

Request format: `/mount-path.m3u`

```
http://localhost:8000/live.m3u
```

Contents:
```m3u
#EXTM3U
#EXTINF:-1,Live Stream
http://localhost:8000/live
```

### PLS Playlist

Request format: `/mount-path.pls`

```
http://localhost:8000/live.pls
```

Contents:
```ini
[playlist]
NumberOfEntries=1
File1=http://localhost:8000/live
Title1=Live Stream
Length1=-1
Version=2
```

### XSPF Playlist

Request format: `/mount-path.xspf`

```
http://localhost:8000/live.xspf
```

## Listener Limits

### Per-Mount Limits

Each mount can have its own listener limit:

```json
{
  "mounts": {
    "/live": {
      "max_listeners": 100
    }
  }
}
```

When the limit is reached, new listeners receive:
```
HTTP 503 Service Unavailable
```

### Global Limits

The server has a global client limit:

```json
{
  "limits": {
    "max_clients": 100,
    "max_listeners_per_mount": 100
  }
}
```

## Connection Behavior

### Burst on Connect

When a listener connects, GoCast sends a "burst" of buffered data immediately. This reduces initial buffering time.

Configure burst size per-mount:
```json
{
  "mounts": {
    "/live": {
      "burst_size": 65536
    }
  }
}
```

### Client Timeout

Idle listeners are disconnected after the timeout period:

```json
{
  "limits": {
    "client_timeout": 30
  }
}
```

### Buffering

GoCast buffers audio data to handle network fluctuations:

```json
{
  "limits": {
    "queue_size": 131072
  }
}
```

Larger buffers = more tolerance for slow connections, but more memory usage.

## CORS (Cross-Origin Requests)

GoCast allows cross-origin requests by default, enabling browser-based players on different domains.

Response headers:
```
Access-Control-Allow-Origin: *
```

## Status Page

View current listeners and stream status:

```
http://localhost:8000/status
```

Formats available:
- HTML: `http://localhost:8000/status` (default)
- JSON: `http://localhost:8000/status` (Accept: application/json)
- XML: `http://localhost:8000/status` (Accept: text/xml)

### JSON Status Example

```json
{
  "server_id": "GoCast",
  "version": "1.0.0",
  "uptime": 3600,
  "mounts": [
    {
      "path": "/live",
      "active": true,
      "listeners": 42,
      "peak": 100,
      "name": "My Radio",
      "genre": "Various",
      "bitrate": 128
    }
  ]
}
```

## Managing Listeners

### Via Admin Panel

1. Go to **Listeners** page
2. View all connected listeners
3. Click **Kick** to disconnect a listener
4. Use **Move** to move listeners to another mount

### Via API

**List listeners:**
```bash
curl -u admin:password http://localhost:8000/admin/listclients?mount=/live
```

**Kick a listener:**
```bash
curl -u admin:password -X POST \
  "http://localhost:8000/admin/killclient?mount=/live&id=LISTENER_ID"
```

## Troubleshooting

### Can't Connect

1. Check server is running: `curl http://localhost:8000/`
2. Verify mount exists and has active source
3. Check listener limits aren't reached
4. Verify firewall allows connections

### Buffering / Stuttering

- Increase `queue_size` in config
- Use a lower bitrate stream
- Check network connectivity
- Try a wired connection

### No Audio

- Verify source is streaming (check admin panel)
- Check player volume
- Try a different player
- Verify stream format is supported

### Wrong Metadata

- Metadata updates may take a few seconds
- Check source is sending metadata
- Some players cache old metadata

## Best Practices

1. **Use playlists** - Provide .m3u links for easy access
2. **Monitor limits** - Watch listener counts in admin panel
3. **Appropriate bitrate** - Balance quality vs. bandwidth
4. **Provide fallback** - Consider a "no source" message
5. **Test on multiple players** - Ensure broad compatibility