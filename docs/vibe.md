# VIBE Configuration Format

GoCast uses the [VIBE](https://github.com/1ay1/vibe) configuration format. VIBE (Values In Bracket Expression) is a human-friendly configuration format designed for readability and simplicity.

## Why VIBE?

- **Simple syntax** - No YAML indentation nightmares, no JSON comma issues
- **Human readable** - Clean, minimal punctuation
- **Fast parsing** - Single-pass O(n) parser
- **Comments** - Full support for `#` comments
- **Type inference** - Automatic detection of integers, floats, booleans, strings

## Basic Syntax

### Key-Value Assignment

```vibe
key value
name "My Application"
port 8080
enabled true
```

### Objects (Nested Structures)

```vibe
server {
    host localhost
    port 8000
}

database {
    host db.example.com
    port 5432
    name myapp
}
```

### Arrays

```vibe
# Inline array
ports [8080 8081 8082]

# Multi-line array
hosts [
    server1.example.com
    server2.example.com
    server3.example.com
]
```

### Comments

```vibe
# This is a full-line comment
server {
    host localhost  # Inline comment
    port 8000
}
```

## Data Types

VIBE automatically infers types:

| Type | Example | Description |
|------|---------|-------------|
| Integer | `42`, `-17` | Whole numbers |
| Float | `3.14`, `-0.5` | Decimal numbers |
| Boolean | `true`, `false` | Lowercase only |
| String | `"hello"`, `localhost` | Quoted or unquoted |
| Array | `[a b c]` | Space-separated values |
| Object | `{ key value }` | Nested key-value pairs |

### Strings

Unquoted strings are allowed for simple values:

```vibe
hostname localhost
path /usr/local/bin
email admin@example.com
url https://example.com/api
```

Use quotes when the value contains spaces or special characters:

```vibe
description "My awesome streaming server"
message "Hello, World!"
path "C:\\Program Files\\App"
```

### Escape Sequences

Quoted strings support escape sequences:

| Escape | Character |
|--------|-----------|
| `\"` | Double quote |
| `\\` | Backslash |
| `\n` | Newline |
| `\t` | Tab |
| `\r` | Carriage return |
| `\uXXXX` | Unicode codepoint |

```vibe
greeting "Hello\nWorld"
path "C:\\Users\\Name"
quote "She said \"Hello\""
```

## Nested Objects

Objects can be nested to any depth:

```vibe
application {
    name "GoCast"
    version 1.0
    
    server {
        host 0.0.0.0
        port 8000
        
        ssl {
            enabled true
            cert /etc/ssl/cert.pem
            key /etc/ssl/key.pem
        }
    }
    
    limits {
        max_clients 100
        timeout 30
    }
}
```

## Arrays

Arrays contain scalar values only (no nested objects or arrays):

```vibe
# Numbers
ports [8080 8081 8082]

# Strings
hosts [server1 server2 server3]

# Mixed types
config [42 "hello" true 3.14]

# Multi-line for readability
allowed_ips [
    192.168.1.*
    10.0.0.*
    127.0.0.1
]
```

**Note:** VIBE intentionally forbids objects inside arrays. Use named objects instead:

```vibe
# ❌ This is NOT valid VIBE
servers [
    { host server1 port 8080 }
    { host server2 port 8081 }
]

# ✅ Use named objects instead
servers {
    primary {
        host server1
        port 8080
    }
    secondary {
        host server2
        port 8081
    }
}
```

## GoCast Configuration Structure

Here's the complete structure for GoCast:

```vibe
# Server configuration
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
        cert /path/to/cert.crt
        key /path/to/key.key
    }
}

# Authentication
auth {
    source_password hackme
    relay_password ""
    admin_user admin
    admin_password hackme
}

# Resource limits
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

# Logging
logging {
    access_log /var/log/gocast/access.log
    error_log /var/log/gocast/error.log
    level info
    log_size 10000
}

# Admin interface
admin {
    enabled true
    user admin
    password hackme
}

# Directory listings
directory {
    enabled false
    yp_urls [
        http://dir.xiph.org/cgi-bin/yp-cgi
    ]
    interval 600
}

# Mount points
mounts {
    live {
        password secret
        max_listeners 100
        fallback /fallback
        stream_name "Live Stream"
        genre "Various"
        description "24/7 Radio"
        url "http://example.com"
        bitrate 128
        type audio/mpeg
        public true
        hidden false
        burst_size 65535
        max_listener_duration 0
        allowed_ips []
        denied_ips []
        dump_file ""
    }
}
```

## Accessing Values (Go API)

GoCast includes a Go implementation of the VIBE parser:

```go
import "github.com/gocast/gocast/pkg/vibe"

// Parse a file
config, err := vibe.ParseFile("config.vibe")
if err != nil {
    log.Fatal(err)
}

// Access values using dot notation
hostname := config.GetString("server.hostname")
port := config.GetInt("server.port")
sslEnabled := config.GetBool("server.ssl.enabled")

// Access with defaults
timeout := config.GetIntDefault("limits.timeout", 30)

// Access arrays
hosts := config.GetStringArray("directory.yp_urls")

// Access nested objects
mounts := config.GetObject("mounts")
for _, key := range mounts.Keys {
    mount := config.GetObject("mounts." + key)
    // ...
}
```

## Path Notation

Access nested values using dot notation:

| Path | Value |
|------|-------|
| `server.hostname` | `localhost` |
| `server.ssl.enabled` | `true` |
| `mounts.live.max_listeners` | `100` |
| `directory.yp_urls[0]` | First URL in array |

## Best Practices

1. **Use comments liberally** - Explain non-obvious settings
2. **Group related settings** - Use objects for organization
3. **Use meaningful names** - Mount names become URL paths
4. **Quote strings with spaces** - Required for multi-word values
5. **Validate before running** - Use `gocast -check -config file.vibe`

## Comparison to Other Formats

| Feature | VIBE | JSON | YAML | TOML |
|---------|------|------|------|------|
| Comments | ✅ | ❌ | ✅ | ✅ |
| Trailing commas | N/A | ❌ | N/A | ❌ |
| Indentation sensitive | ❌ | ❌ | ✅ | ❌ |
| Unquoted strings | ✅ | ❌ | ✅ | ❌ |
| Type inference | ✅ | ❌ | ✅ | ✅ |
| Multi-line strings | ❌ | ❌ | ✅ | ✅ |

## Resources

- [VIBE Specification](https://github.com/1ay1/vibe/blob/main/SPECIFICATION.md)
- [VIBE GitHub Repository](https://github.com/1ay1/vibe)
- [GoCast VIBE Parser Source](../pkg/vibe/)