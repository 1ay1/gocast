# Sources Guide

Sources (also called "broadcasters" or "encoders") are clients that send audio to GoCast. This guide covers connecting various software to your GoCast server.

## Authentication

Sources authenticate using HTTP Basic Auth:

| Field | Value |
|-------|-------|
| Username | `source` (or empty) |
| Password | Source password or mount-specific password |

### Password Priority

1. **Mount-specific password** - If set in mount config, use this
2. **Global source password** - Fallback if no mount password
3. **Admin credentials** - Admin user/pass also works for sources

Find your passwords:
```bash
cat ~/.gocast/config.json | grep -E "(source_password|password)"
```

## Connection URL Format

```
icecast://[username]:[password]@[host]:[port]/[mount]
```

Example:
```
icecast://source:hackme@localhost:8000/live
```

## Supported Formats

| Format | MIME Type | Extension |
|--------|-----------|-----------|
| MP3 | `audio/mpeg` | .mp3 |
| Ogg Vorbis | `audio/ogg` | .ogg |
| Ogg Opus | `audio/ogg; codecs=opus` | .opus |
| AAC | `audio/aac` | .aac |
| FLAC | `audio/flac` | .flac |

## FFmpeg

FFmpeg is the most versatile tool for streaming to GoCast.

### Stream MP3

```bash
ffmpeg -re -i input.mp3 \
  -c:a libmp3lame -b:a 128k \
  -f mp3 \
  -content_type audio/mpeg \
  icecast://source:hackme@localhost:8000/live
```

### Stream from Microphone (Linux)

```bash
ffmpeg -f pulse -i default \
  -c:a libmp3lame -b:a 128k \
  -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### Stream from Microphone (macOS)

```bash
ffmpeg -f avfoundation -i ":0" \
  -c:a libmp3lame -b:a 128k \
  -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### Stream Opus (Better Quality at Low Bitrates)

```bash
ffmpeg -re -i input.mp3 \
  -c:a libopus -b:a 64k \
  -f ogg \
  -content_type "audio/ogg" \
  icecast://source:hackme@localhost:8000/live
```

### Stream Playlist

```bash
ffmpeg -re -f concat -safe 0 -i playlist.txt \
  -c:a libmp3lame -b:a 128k \
  -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

`playlist.txt`:
```
file '/path/to/song1.mp3'
file '/path/to/song2.mp3'
file '/path/to/song3.mp3'
```

### Stream with Metadata

```bash
ffmpeg -re -i input.mp3 \
  -c:a libmp3lame -b:a 128k \
  -metadata title="My Stream" \
  -metadata artist="DJ Name" \
  -f mp3 \
  -ice_name "My Radio" \
  -ice_description "The best music" \
  -ice_genre "Various" \
  icecast://source:hackme@localhost:8000/live
```

## Butt (Broadcast Using This Tool)

Butt is a free, cross-platform streaming tool with a simple GUI.

**Download:** https://danielnoethen.de/butt/

### Configuration

1. **Settings → Main → Server → Add**
2. Fill in:
   - Name: `My GoCast Server`
   - Type: `Icecast`
   - Address: `localhost`
   - Port: `8000`
   - Password: `hackme` (your source password)
   - Mount: `/live`
   - Icecast User: `source`

3. **Settings → Audio**
   - Codec: MP3 or Ogg/Opus
   - Bitrate: 128 kbps (or your preference)

4. Click **Play** to start broadcasting

## Mixxx

Mixxx is a free DJ software with built-in broadcasting.

**Download:** https://mixxx.org/

### Configuration

1. **Preferences → Live Broadcasting**
2. Enable: ✓ Enable Live Broadcasting
3. Connection:
   - Type: `Icecast 2`
   - Host: `localhost`
   - Port: `8000`
   - Mount: `/live`
   - Login: `source`
   - Password: `hackme`

4. Stream:
   - Format: MP3 or Ogg Vorbis
   - Bitrate: 128 kbps

5. Click **Connect** in the main window

## OBS Studio

OBS can stream audio (and video) to GoCast.

### Audio-Only Streaming

1. **Settings → Output → Recording**
   - Type: Custom Output (FFmpeg)
   - FFmpeg Output Type: Output to URL
   - URL: `icecast://source:hackme@localhost:8000/live`
   - Container Format: mp3
   - Audio Encoder: libmp3lame
   - Audio Bitrate: 128 kbps

2. Start Recording (this actually streams)

## Liquidsoap

Liquidsoap is a powerful audio streaming language.

### Basic Stream

```liquidsoap
# Input from playlist
source = playlist("/path/to/music")

# Output to GoCast
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  source
)
```

### With Fallback

```liquidsoap
# Main live input
live = input.http("http://your-source:8000/input")

# Fallback playlist
playlist = playlist("/path/to/music")

# Switch to playlist when live is down
source = fallback([live, playlist])

output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  source
)
```

## VLC

VLC can stream audio to GoCast.

### GUI Method

1. **Media → Stream**
2. Add your audio file/source
3. Click **Stream**
4. Next → Destinations: Add `Icecast`
5. Configure:
   - Address: `localhost`
   - Port: `8000`
   - Mount Point: `/live`
   - Login: `source`
   - Password: `hackme`

### Command Line

```bash
vlc input.mp3 --sout '#transcode{acodec=mp3,ab=128}:std{access=shout,mux=mp3,dst=source:hackme@localhost:8000/live}'
```

## Troubleshooting

### Connection Refused

```
Connection refused
```

- Check GoCast is running: `pgrep gocast`
- Verify port: `curl http://localhost:8000/`
- Check firewall rules

### Authentication Failed

```
401 Unauthorized
```

- Verify password in config: `cat ~/.gocast/config.json | grep password`
- Try using admin credentials
- Check mount-specific password if set

### Mount Already in Use

```
409 Conflict - Source already connected
```

Another source is already streaming to this mount. Either:
- Disconnect the existing source (Admin Panel → Streams → Disconnect)
- Use a different mount point

### Stream Cuts Out

- Increase `source_timeout` in config
- Check network stability
- Use a wired connection instead of WiFi

### No Audio / Silent Stream

- Verify your audio source is working
- Check FFmpeg/encoder logs for errors
- Try a test file known to work

## Best Practices

1. **Use appropriate bitrate** - 128kbps for music, 64kbps for speech
2. **Test locally first** - Stream to localhost before remote
3. **Monitor the connection** - Watch for reconnections/errors
4. **Use Opus for low bandwidth** - Better quality than MP3 at low bitrates
5. **Set metadata** - Helps listeners identify your stream