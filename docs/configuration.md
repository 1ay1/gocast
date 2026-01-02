# GoCast Configuration Reference

GoCast uses the [VIBE](https://github.com/1ay1/vibe) configuration format - a human-friendly format with minimal syntax. Configuration files use the `.vibe` extension.

## Configuration File Location

By default, GoCast looks for `gocast.vibe` in the current directory. You can specify a different location:

```bash
./gocast -config /etc/gocast/gocast.vibe
```

## Complete Configuration Example

```vibe
# GoCast Configuration File

server {
    hostname localhost
    listen 0.0.0.0
    port 8000
    ssl_port 8443
    location "Earth"
    server_id GoCast
    admin_root /admin

    ssl {
        enabled false
        cert /etc/gocast/server.crt
        key /etc/gocast/server.key
    }
}

auth {
    source_password hackme
    relay_password ""
    admin_user admin
    admin_password hackme
}

limits {
    max_clients 100
    max_sources 10
    max_listeners_per_mount 100
    queue_size 524288
    burst_size 65535
    client_timeout 30
    header_timeout 15
    source_timeout 10
}

logging {
    access_log /var/log/gocast/access.log
    error_log /var/log/gocast/error.log
    level info
    log_size 10000
}

admin {
    enabled true
    user admin
    password hackme
}

directory {
    enabled false
    yp_urls [
        http://dir.xiph.org/cgi-bin/yp-cgi
    ]
    interval 600
}

mounts {
    live {
        password secret123
        max_listeners 100
        fallback /fallback
        stream_name "Live Stream"
        genre "Various"
        description "Live broadcast"
        url "http://example.com"
        bitrate 128
        type audio/mpeg
        public true
        hidden false
        burst_size 65535
        max_listener_duration 0
    }
}
```

## Configuration Sections

### server

Server-level settings for the GoCast instance.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `hostname` | string | `localhost` | Public hostname of the server |
| `listen` | string | `0.0.0.0` | IP address to bind to |
| `port` | integer | `8000` | HTTP port for streaming |
| `ssl_port` | integer | `8443` | HTTPS port (when SSL enabled) |
| `location` | string | `Earth` | Server location description |
| `server_id` | string | `GoCast` | Server identifier in responses |
| `admin_root` | string | `/admin` | URL path for admin interface |

#### server.ssl

SSL/TLS configuration for HTTPS streaming.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable HTTPS |
| `cert` | string | - | Path to SSL certificate file |
| `key` | string | - | Path to SSL private key file |

### auth

Authentication credentials for sources and administration.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `source_password` | string | `hackme` | Default password for source clients |
| `relay_password` | string | `""` | Password for relay connections |
| `admin_user` | string | `admin` | Admin interface username |
| `admin_password` | string | `hackme` | Admin interface password |

### limits

Resource limits and timeouts.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max_clients` | integer | `100` | Maximum total connected clients |
| `max_sources` | integer | `10` | Maximum source connections |
| `max_listeners_per_mount` | integer | `100` | Maximum listeners per mount point |
| `queue_size` | integer | `524288` | Stream buffer size in bytes (512KB) |
| `burst_size` | integer | `65535` | Initial burst size for new listeners (64KB) |
| `client_timeout` | integer | `30` | Listener timeout in seconds |
| `header_timeout` | integer | `15` | HTTP header read timeout in seconds |
| `source_timeout` | integer | `10` | Source connection timeout in seconds |

### logging

Logging configuration.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `access_log` | string | `/var/log/gocast/access.log` | Access log file path |
| `error_log` | string | `/var/log/gocast/error.log` | Error log file path |
| `level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `log_size` | integer | `10000` | Maximum log entries to keep |

### admin

Admin interface settings.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable admin interface |
| `user` | string | `admin` | Admin username |
| `password` | string | `hackme` | Admin password |

### directory

Directory/YP (Yellow Pages) listing settings.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable directory listings |
| `yp_urls` | array | `[]` | List of YP server URLs |
| `interval` | integer | `600` | Update interval in seconds |

### mounts

Mount point configurations. Each mount is defined as a named object.

```vibe
mounts {
    mountname {
        # mount settings
    }
}
```

The mount name becomes the URL path (e.g., `mountname` → `/mountname`).

#### Mount Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `password` | string | (global) | Mount-specific source password |
| `max_listeners` | integer | `100` | Maximum listeners for this mount |
| `fallback` | string | - | Fallback mount when source disconnects |
| `stream_name` | string | (mount name) | Stream display name |
| `genre` | string | - | Stream genre |
| `description` | string | - | Stream description |
| `url` | string | - | Associated website URL |
| `bitrate` | integer | `128` | Stream bitrate in kbps |
| `type` | string | `audio/mpeg` | Content-Type (MIME type) |
| `public` | boolean | `true` | List in public directories |
| `hidden` | boolean | `false` | Hide from status page |
| `burst_size` | integer | (global) | Burst size for this mount |
| `max_listener_duration` | integer | `0` | Max listen time in seconds (0=unlimited) |
| `allowed_ips` | array | `[]` | IP whitelist (supports wildcards) |
| `denied_ips` | array | `[]` | IP blacklist (supports wildcards) |
| `dump_file` | string | - | Record stream to file |

## Content Types

Common content types for streaming:

| Format | Content-Type |
|--------|--------------|
| MP3 | `audio/mpeg` |
| Ogg Vorbis | `application/ogg` or `audio/ogg` |
| Ogg Opus | `audio/ogg; codecs=opus` |
| AAC | `audio/aac` or `audio/aacp` |
| FLAC | `audio/flac` |
| WebM | `audio/webm` |

## IP Filtering

Use wildcards for IP ranges:

```vibe
mounts {
    private {
        allowed_ips [
            192.168.1.*
            10.0.0.*
            127.0.0.1
        ]
        denied_ips [
            192.168.1.100
        ]
    }
}
```

## Environment-Specific Configs

Create separate config files for different environments:

```bash
# Development
./gocast -config gocast.dev.vibe

# Production
./gocast -config gocast.prod.vibe
```

## Validating Configuration

Check your configuration without starting the server:

```bash
./gocast -check -config gocast.vibe
```

## Hot Reload

Send SIGHUP to reload configuration:

```bash
kill -HUP $(pgrep gocast)
```

Note: Some settings require a full restart to take effect.