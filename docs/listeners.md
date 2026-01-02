# Listener Guide

This document covers how listeners connect to GoCast streams, client compatibility, and advanced features like ICY metadata.

## Connecting to a Stream

### Basic URL Format

```
http://hostname:port/mountpoint
```

Example:
```
http://localhost:8000/live
```

### HTTPS (SSL/TLS)

If SSL is enabled:
```
https://hostname:ssl_port/mountpoint
```

## Compatible Players

GoCast works with any media player that supports HTTP audio streaming.

### Desktop Players

| Player | Command |
|--------|---------|
| VLC | `vlc http://localhost:8000/live` |
| mpv | `mpv http://localhost:8000/live` |
| ffplay | `ffplay http://localhost:8000/live` |
| Audacious | Open URL in playlist |
| Clementine | Add stream URL |
| foobar2000 | File → Open URL |

### Command Line

```bash
# Stream with mpv
mpv http://localhost:8000/live

# Stream with VLC (no GUI)
cvlc http://localhost:8000/live

# Record stream with curl
curl http://localhost:8000/live -o recording.mp3

# Record stream with ffmpeg
ffmpeg -i http://localhost:8000/live -c copy recording.mp3
```

### Mobile Apps

- **Android**: VLC for Android, Radiodroid, Simple Radio
- **iOS**: VLC for iOS, TuneIn, Radio Apps

### Web Browsers

HTML5 audio works in modern browsers:

```html
<audio src="http://localhost:8000/live" controls autoplay></audio>
```

JavaScript example:

```javascript
const audio = new Audio('http://localhost:8000/live');
audio.play();
```

## ICY Metadata

ICY (I Can Yell) metadata provides real-time stream information like the current song title.

### Requesting Metadata

Add the `Icy-MetaData: 1` header to receive inline metadata:

```bash
curl -H "Icy-MetaData: 1" http://localhost:8000/live
```

### Metadata Format

When metadata is enabled, the server sends:
- `icy-metaint` header indicating the metadata interval (usually 16000 bytes)
- Inline metadata blocks every `icy-metaint` bytes

### Response Headers

```
HTTP/1.1 200 OK
Content-Type: audio/mpeg
icy-name: My Radio Station
icy-description: 24/7 Music
icy-genre: Rock
icy-url: http://example.com
icy-br: 128
icy-pub: 1
icy-metaint: 16000
```

### Parsing Metadata

Metadata block format:
1. 1 byte: block size (multiply by 16 for actual length)
2. N bytes: metadata string (null-padded)

Example metadata content:
```
StreamTitle='Pink Floyd - Money';
```

### JavaScript Metadata Parser

```javascript
class ICYMetadataReader {
  constructor(url, metadataCallback) {
    this.url = url;
    this.onMetadata = metadataCallback;
    this.metaInt = 0;
    this.byteCount = 0;
  }

  async connect() {
    const response = await fetch(this.url, {
      headers: { 'Icy-MetaData': '1' }
    });
    
    this.metaInt = parseInt(response.headers.get('icy-metaint') || '0');
    // Process stream...
  }
}
```

## CORS Support

GoCast includes CORS headers for web-based players:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Origin, Accept, Content-Type, Icy-MetaData
Access-Control-Expose-Headers: icy-metaint, icy-name, icy-description, icy-genre, icy-url, icy-br
```

This allows JavaScript applications to stream directly from GoCast.

## Connection Limits

### Per-Mount Limits

Each mount can have its own listener limit:

```vibe
mounts {
    live {
        max_listeners 500
    }
}
```

### Global Limits

```vibe
limits {
    max_clients 1000
}
```

### Listen Duration Limits

Limit how long a listener can stay connected:

```vibe
mounts {
    live {
        max_listener_duration 3600  # 1 hour
    }
}
```

## IP-Based Access Control

### Whitelist (Allow Only)

```vibe
mounts {
    private {
        allowed_ips [
            192.168.1.*
            10.0.0.*
        ]
    }
}
```

### Blacklist (Deny)

```vibe
mounts {
    live {
        denied_ips [
            123.45.67.89
            spam.example.com
        ]
    }
}
```

## Burst Buffer

When a listener connects, they receive a "burst" of recent audio data to minimize buffering delay.

```vibe
limits {
    burst_size 65535  # 64KB burst
}
```

Per-mount override:

```vibe
mounts {
    live {
        burst_size 131072  # 128KB burst for this mount
    }
}
```

## Fallback Mounts

When a source disconnects, listeners can be redirected to a fallback mount:

```vibe
mounts {
    live {
        fallback /fallback
    }
    
    fallback {
        stream_name "Fallback Stream"
        hidden true
    }
}
```

## Monitoring Listeners

### Status API

```bash
# Get listener count
curl -s http://localhost:8000/status?format=json | jq '.icestats.source[].listeners'
```

### Admin API

List all listeners on a mount:

```bash
curl -u admin:hackme http://localhost:8000/admin/listclients?mount=/live
```

### Kick a Listener

```bash
curl -u admin:hackme "http://localhost:8000/admin/killclient?mount=/live&id=LISTENER_ID"
```

## Troubleshooting

### "Stream not available"

- No source is currently streaming
- Check if source is connected: `curl http://localhost:8000/status?format=json`

### Buffering Issues

- Increase `burst_size` for faster initial playback
- Check network bandwidth
- Reduce stream bitrate

### Connection Refused

- Server not running
- Firewall blocking port 8000
- Wrong hostname/port

### No Audio

- Check player volume
- Verify stream format is supported by player
- Try a different player (VLC, mpv)

### Metadata Not Showing

- Player must support ICY metadata
- Ensure `Icy-MetaData: 1` header is sent
- Check `icy-metaint` response header