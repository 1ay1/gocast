# Streaming Sources

This guide covers how to connect source clients to GoCast. GoCast is compatible with any Icecast-compatible source client.

## Supported Protocols

GoCast supports two methods for source connections:

1. **HTTP PUT** - Modern method used by most clients
2. **SOURCE** - Legacy Icecast method (deprecated but supported)

## Authentication

Sources authenticate using HTTP Basic Authentication:

- **Username**: `source` (or leave empty)
- **Password**: Your source password (default: `hackme`)

## FFmpeg

FFmpeg is a versatile tool for streaming audio and video.

### Basic MP3 Stream

```bash
ffmpeg -re -i input.mp3 -c:a libmp3lame -b:a 128k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### High Quality MP3 (320kbps)

```bash
ffmpeg -re -i input.flac -c:a libmp3lame -b:a 320k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### Ogg Vorbis Stream

```bash
ffmpeg -re -i input.mp3 -c:a libvorbis -b:a 192k -f ogg \
  icecast://source:hackme@localhost:8000/live
```

### Ogg Opus Stream

```bash
ffmpeg -re -i input.mp3 -c:a libopus -b:a 128k -f ogg \
  icecast://source:hackme@localhost:8000/live
```

### Stream with Metadata

```bash
ffmpeg -re -i input.mp3 -c:a libmp3lame -b:a 128k -f mp3 \
  -ice_name "My Radio" \
  -ice_description "Best music 24/7" \
  -ice_genre "Pop" \
  -ice_url "http://myradio.com" \
  icecast://source:hackme@localhost:8000/live
```

### Stream from Microphone (Linux)

```bash
ffmpeg -f pulse -i default -c:a libmp3lame -b:a 128k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### Stream from Microphone (macOS)

```bash
ffmpeg -f avfoundation -i ":0" -c:a libmp3lame -b:a 128k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

### Continuous Playlist Loop

```bash
# Create a playlist file (playlist.txt):
# file 'song1.mp3'
# file 'song2.mp3'
# file 'song3.mp3'

ffmpeg -re -f concat -safe 0 -stream_loop -1 -i playlist.txt \
  -c:a libmp3lame -b:a 128k -f mp3 \
  icecast://source:hackme@localhost:8000/live
```

## BUTT (Broadcast Using This Tool)

BUTT is a popular GUI application for live streaming.

### Configuration

1. Open BUTT and go to **Settings**
2. Click **ADD** under Server Settings
3. Configure:
   - **Type**: Icecast
   - **Address**: `localhost` (or your server hostname)
   - **Port**: `8000`
   - **Password**: `hackme`
   - **Icecast mountpoint**: `/live`
   - **Icecast user**: `source`
4. Click **Save**
5. Select your audio input device
6. Click **Play** to start streaming

### Recommended Settings

- **Codec**: MP3 or Ogg Vorbis
- **Bitrate**: 128-320 kbps
- **Samplerate**: 44100 Hz
- **Channels**: Stereo

## Liquidsoap

Liquidsoap is a powerful audio stream generator.

### Basic Configuration

```liquidsoap
# Output to GoCast
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  name="My Radio",
  description="Powered by Liquidsoap",
  genre="Various",
  source
)
```

### Playlist with Jingles

```liquidsoap
# Music playlist
music = playlist("/path/to/music")

# Jingles every 30 minutes
jingles = playlist("/path/to/jingles")
radio = rotate(weights=[1, 29], [jingles, music])

# Output
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  radio
)
```

### Live Input with Fallback

```liquidsoap
# Live input
live = input.harbor("live", port=8005, password="livepassword")

# Fallback to playlist when no live input
music = playlist("/path/to/music")
radio = fallback(track_sensitive=false, [live, music])

# Output
output.icecast(
  %mp3(bitrate=128),
  host="localhost",
  port=8000,
  password="hackme",
  mount="/live",
  radio
)
```

## Mixxx

Mixxx is a free DJ software with built-in streaming support.

### Configuration

1. Open **Preferences** → **Live Broadcasting**
2. Configure:
   - **Type**: Icecast 2
   - **Host**: `localhost`
   - **Port**: `8000`
   - **Mount**: `/live`
   - **Login**: `source`
   - **Password**: `hackme`
   - **Stream name**: Your stream name
3. Select format (MP3/Ogg) and bitrate
4. Click **Enable Live Broadcasting**

## OBS Studio

OBS can stream audio to GoCast using custom FFmpeg output.

### Configuration

1. Go to **Settings** → **Stream**
2. Select **Custom...**
3. Set **Server**: `icecast://source:hackme@localhost:8000/live`
4. Go to **Settings** → **Output**
5. Set **Output Mode**: Advanced
6. Configure audio encoding (MP3 or AAC)

## GStreamer

GStreamer can be used for streaming audio.

### MP3 Stream

```bash
gst-launch-1.0 filesrc location=input.mp3 ! \
  decodebin ! audioconvert ! audioresample ! \
  lamemp3enc bitrate=128 ! \
  shout2send ip=localhost port=8000 password=hackme mount=/live
```

### Microphone Stream

```bash
gst-launch-1.0 pulsesrc ! audioconvert ! audioresample ! \
  lamemp3enc bitrate=128 ! \
  shout2send ip=localhost port=8000 password=hackme mount=/live
```

## VLC

VLC can also be used as a streaming source.

### Stream File

```bash
vlc input.mp3 --sout '#transcode{acodec=mp3,ab=128}:std{access=shout,mux=mp3,dst=source:hackme@localhost:8000/live}'
```

### Stream Playlist

```bash
vlc playlist.m3u --loop --sout '#transcode{acodec=mp3,ab=128}:std{access=shout,mux=mp3,dst=source:hackme@localhost:8000/live}'
```

## ices

ices is a lightweight source client for Icecast servers.

### Configuration (ices.xml)

```xml
<?xml version="1.0"?>
<ices>
  <stream>
    <name>My Radio</name>
    <genre>Various</genre>
    <description>Streaming with ices</description>
  </stream>
  <server>
    <hostname>localhost</hostname>
    <port>8000</port>
    <password>hackme</password>
    <mount>/live</mount>
  </server>
  <playlist>
    <file>/path/to/playlist.txt</file>
    <randomize>1</randomize>
  </playlist>
</ices>
```

## Troubleshooting

### Connection Refused

- Check that GoCast is running: `curl http://localhost:8000/`
- Verify the port is correct
- Check firewall settings

### Authentication Failed

- Verify password matches `source_password` in config
- Username should be `source` or empty
- Check for mount-specific passwords

### Stream Disconnects

- Check source timeout settings
- Ensure stable network connection
- Monitor server logs for errors

### No Audio

- Verify audio format is supported
- Check bitrate settings
- Confirm source is sending data: check server logs

### Buffer Underruns

- Increase `queue_size` in configuration
- Reduce bitrate
- Check network stability

## Best Practices

1. **Use appropriate bitrate**: 128-192 kbps for music, 64-96 kbps for speech
2. **Set metadata**: Include stream name, genre, and description
3. **Monitor the stream**: Watch server logs for issues
4. **Test before going live**: Verify stream works with a test listener
5. **Use stable network**: Wired connections are more reliable than WiFi
6. **Implement fallback**: Use Liquidsoap or similar for automatic fallback