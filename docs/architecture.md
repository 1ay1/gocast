# GoCast Architecture

This document describes the internal architecture and design of GoCast.

## Overview

GoCast is built as a modular Go application with clear separation of concerns. The architecture follows standard Go project layout conventions.

```
gocast/
├── cmd/gocast/          # Application entry point
├── internal/            # Private application code
│   ├── auth/           # Authentication handling
│   ├── config/         # Configuration management
│   ├── server/         # HTTP server and routing
│   ├── source/         # Source client handling
│   ├── stats/          # Statistics collection
│   └── stream/         # Stream buffer and mount management
├── pkg/vibe/           # VIBE configuration parser (public)
└── web/                # Web assets (templates, static files)
```

## Core Components

### 1. HTTP Server (`internal/server`)

The server component handles all HTTP traffic including:

- **Listener connections** - GET requests to mount points
- **Source connections** - PUT requests from streaming clients
- **Admin API** - Administrative endpoints
- **Status pages** - Public status in HTML/JSON/XML

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP Server                          │
├─────────────────────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐   │
│  │ Listener│  │ Source  │  │  Admin  │  │ Status  │   │
│  │ Handler │  │ Handler │  │ Handler │  │ Handler │   │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘   │
│       │            │            │            │         │
│       └────────────┴────────────┴────────────┘         │
│                          │                              │
│                    ┌─────┴─────┐                       │
│                    │  Router   │                       │
│                    └───────────┘                       │
└─────────────────────────────────────────────────────────┘
```

#### Request Flow

1. Request arrives at HTTP server
2. Router examines method and path
3. Request dispatched to appropriate handler
4. Handler processes request and responds

### 2. Mount Manager (`internal/stream`)

The mount manager is the central component for stream management.

```
┌─────────────────────────────────────────────────────────┐
│                   Mount Manager                          │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │   /live     │  │   /radio    │  │  /fallback  │    │
│  │   Mount     │  │   Mount     │  │   Mount     │    │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘    │
│         │                │                │            │
│  ┌──────┴──────┐  ┌──────┴──────┐  ┌──────┴──────┐    │
│  │   Buffer    │  │   Buffer    │  │   Buffer    │    │
│  │  (Ring)     │  │  (Ring)     │  │  (Ring)     │    │
│  └─────────────┘  └─────────────┘  └─────────────┘    │
└─────────────────────────────────────────────────────────┘
```

#### Mount Structure

Each mount contains:
- **Buffer** - Ring buffer for audio data
- **Metadata** - Stream information (title, genre, etc.)
- **Listeners** - Map of connected listeners
- **Config** - Mount-specific configuration
- **State** - Active/inactive status

### 3. Ring Buffer (`internal/stream/buffer.go`)

The ring buffer efficiently stores streaming audio data for distribution to listeners.

```
┌────────────────────────────────────────────────────────┐
│                    Ring Buffer                          │
│                                                         │
│  Write Position                                         │
│       ↓                                                 │
│  ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐   │
│  │ 0 │ 1 │ 2 │ 3 │ 4 │ 5 │ 6 │ 7 │ 8 │ 9 │...│ N │   │
│  └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘   │
│        ↑                   ↑                            │
│    Listener 1          Listener 2                       │
│    Read Position       Read Position                    │
│                                                         │
│  Size: 512KB (configurable)                            │
│  Burst: 64KB (initial data for new listeners)          │
└────────────────────────────────────────────────────────┘
```

#### Buffer Operations

- **Write** - Source writes data, position advances
- **Read** - Listeners read from their position
- **Burst** - New listeners get recent data immediately
- **Wrap** - Old data is overwritten when buffer is full

### 4. Source Handler (`internal/source`)

Handles incoming source connections using HTTP hijacking for proper streaming.

```
Source Client                    GoCast
     │                              │
     │──── PUT /live ──────────────→│
     │                              │ Authenticate
     │                              │ Create/Get Mount
     │←─── HTTP 200 OK ────────────│ Hijack Connection
     │                              │
     │──── Audio Data ─────────────→│
     │──── Audio Data ─────────────→│ Write to Buffer
     │──── Audio Data ─────────────→│
     │          ...                 │
     │                              │
     │←─── Connection Close ───────│ Cleanup
```

#### Connection Hijacking

For streaming PUT requests, GoCast hijacks the HTTP connection:

1. Complete HTTP handshake
2. Send 200 OK response
3. Take control of raw TCP connection
4. Read streaming data until disconnect

### 5. Listener Handler (`internal/server/listener.go`)

Serves audio streams to connected listeners.

```
Listener                        GoCast
    │                              │
    │──── GET /live ──────────────→│
    │     Icy-MetaData: 1          │ Check mount active
    │                              │ Check limits
    │←─── HTTP 200 ────────────────│
    │     icy-metaint: 16000       │
    │     Content-Type: audio/mpeg │
    │                              │
    │←─── Burst Data ──────────────│ Initial burst
    │←─── Audio Data ──────────────│
    │←─── [Metadata Block] ────────│ Every 16000 bytes
    │←─── Audio Data ──────────────│
    │          ...                 │
```

#### ICY Metadata Injection

When listeners request metadata (`Icy-MetaData: 1`):

1. Server sets `icy-metaint` header (interval in bytes)
2. After every `metaint` bytes, insert metadata block
3. Metadata block format: `[size_byte][padded_string]`

### 6. Authentication (`internal/auth`)

Handles source and admin authentication with brute-force protection.

```
┌─────────────────────────────────────────────────────────┐
│                   Authenticator                          │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐    ┌──────────────────────────────┐  │
│  │   Request    │───→│  Check Lockout (by IP)       │  │
│  └──────────────┘    └──────────────┬───────────────┘  │
│                                      │                   │
│                      ┌───────────────┴───────────────┐  │
│                      │  Extract Credentials          │  │
│                      │  (Basic Auth / ICY Headers)   │  │
│                      └───────────────┬───────────────┘  │
│                                      │                   │
│                      ┌───────────────┴───────────────┐  │
│                      │  Validate Against Config      │  │
│                      │  (Source/Admin/Mount)         │  │
│                      └───────────────┬───────────────┘  │
│                                      │                   │
│                      ┌───────────────┴───────────────┐  │
│                      │  Record Result               │  │
│                      │  (Clear/Increment Failures)   │  │
│                      └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 7. Configuration (`internal/config`)

Parses VIBE configuration files into Go structs.

```
┌─────────────────────────────────────────────────────────┐
│                 Configuration Flow                       │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  gocast.vibe ──→ VIBE Parser ──→ *vibe.Value            │
│                                       │                  │
│                                       ↓                  │
│                              Config Loader               │
│                                       │                  │
│                                       ↓                  │
│                              *config.Config              │
│                                       │                  │
│              ┌────────────────────────┼────────────┐    │
│              ↓                        ↓            ↓    │
│        ServerConfig            LimitsConfig   MountConfig│
│        AuthConfig              LoggingConfig  AdminConfig│
└─────────────────────────────────────────────────────────┘
```

### 8. VIBE Parser (`pkg/vibe`)

A complete Go implementation of the VIBE configuration format.

```
┌─────────────────────────────────────────────────────────┐
│                    VIBE Parser                           │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Input String ──→ Lexer ──→ Tokens ──→ Parser ──→ AST  │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Lexer (lexer.go)                                │   │
│  │ - Tokenizes input into: IDENTIFIER, STRING,    │   │
│  │   INT, FLOAT, BOOL, BRACE, BRACKET, etc.       │   │
│  └─────────────────────────────────────────────────┘   │
│                          │                              │
│                          ↓                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Parser (parser.go)                              │   │
│  │ - Builds tree of Value objects                  │   │
│  │ - Handles objects, arrays, scalars              │   │
│  └─────────────────────────────────────────────────┘   │
│                          │                              │
│                          ↓                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Value (value.go)                                │   │
│  │ - Type-safe value access                        │   │
│  │ - Path notation (server.ssl.enabled)            │   │
│  │ - Array indexing (ports[0])                     │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Data Flow

### Source to Listener Flow

```
┌──────────┐     ┌─────────────┐     ┌─────────────┐     ┌──────────┐
│  Source  │────→│   Source    │────→│    Mount    │────→│ Listener │
│  Client  │     │   Handler   │     │   Buffer    │     │  Handler │
└──────────┘     └─────────────┘     └─────────────┘     └──────────┘
                                            │                   │
    FFmpeg,                          Ring Buffer           Listeners
    BUTT, etc.                       (512KB)               (N clients)
```

### Detailed Flow

1. **Source connects** via HTTP PUT to `/live`
2. **Authentication** validated against config
3. **Mount created** or existing mount used
4. **Connection hijacked** for raw streaming
5. **Audio data** written to ring buffer
6. **Listeners notified** via channel
7. **Listeners read** from buffer at their position
8. **Metadata injected** if requested

## Concurrency Model

GoCast uses Go's goroutines and channels for concurrent operations:

```
┌─────────────────────────────────────────────────────────┐
│                  Concurrency Model                       │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  HTTP Server (goroutine per connection)                 │
│       │                                                  │
│       ├──→ Source Handler (1 per active source)         │
│       │         │                                        │
│       │         └──→ Write to Buffer (mutex protected)  │
│       │                      │                           │
│       │                      ↓                           │
│       │              Notify Channel                      │
│       │                      │                           │
│       ├──→ Listener Handler ←┘ (1 per listener)         │
│       │         │                                        │
│       │         └──→ Read from Buffer (RWMutex)         │
│       │                                                  │
│       └──→ Admin Handler (on-demand)                    │
│                                                          │
│  Synchronization:                                       │
│  - sync.RWMutex for buffer access                       │
│  - sync.atomic for counters                             │
│  - Channels for notifications                           │
└─────────────────────────────────────────────────────────┘
```

## Memory Management

### Buffer Sizing

```
Default Configuration:
- Queue Size: 512KB per mount
- Burst Size: 64KB initial data

Memory per mount ≈ Queue Size + Metadata + Overhead
Memory per listener ≈ 1KB (connection state)

Example (10 mounts, 1000 listeners):
- Buffers: 10 × 512KB = 5MB
- Listeners: 1000 × 1KB = 1MB
- Overhead: ~10MB
- Total: ~16MB
```

### Garbage Collection

- Ring buffer reuses allocated memory
- Listeners cleaned up on disconnect
- No persistent allocations per audio packet

## Error Handling

```
┌─────────────────────────────────────────────────────────┐
│                   Error Handling                         │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Network Errors:                                        │
│  - Connection reset → Clean up listener/source          │
│  - Timeout → Disconnect with logging                    │
│                                                          │
│  Authentication Errors:                                 │
│  - Invalid credentials → 401 Unauthorized               │
│  - Lockout → 401 + delay                               │
│                                                          │
│  Capacity Errors:                                       │
│  - Max listeners → 503 Service Unavailable              │
│  - Max sources → 503 Service Unavailable                │
│                                                          │
│  Stream Errors:                                         │
│  - Source disconnect → Listeners get fallback or 503    │
│  - Buffer overrun → Listener position adjusted          │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Icecast Compatibility

GoCast maintains compatibility with Icecast:

| Feature | Icecast | GoCast |
|---------|---------|--------|
| Source Protocol | PUT, SOURCE | PUT, SOURCE |
| Authentication | Basic Auth | Basic Auth |
| ICY Metadata | Yes | Yes |
| Status XML | Yes | Yes |
| Status JSON | Yes | Yes |
| Admin API | Yes | Yes |
| YP Directory | Yes | Planned |

## Performance Characteristics

- **Latency**: ~100ms (configurable via burst size)
- **Throughput**: 1000+ concurrent listeners per mount
- **Memory**: ~16KB per listener
- **CPU**: Minimal (mostly I/O bound)
- **Startup**: <100ms

## Future Enhancements

- [ ] WebSocket support for modern web players
- [ ] Relay support for distributed streaming
- [ ] Prometheus metrics endpoint
- [ ] HLS/DASH output
- [ ] Recording to file
- [ ] Scheduled playlist support