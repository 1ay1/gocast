package server

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogEntry represents a single log entry
type LogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Count     int       `json:"count,omitempty"` // For aggregated entries
}

// LogBuffer is a circular buffer that stores log entries and broadcasts to subscribers
type LogBuffer struct {
	entries     []LogEntry
	maxSize     int
	nextID      int64
	mu          sync.RWMutex
	subscribers map[chan LogEntry]struct{}
	subMu       sync.RWMutex

	// Rate limiting for repeated messages
	lastMessage string
	lastSource  string
	lastTime    time.Time
	repeatCount int
	rateLimitMs int64 // Minimum ms between identical messages
}

// NewLogBuffer creates a new log buffer with the specified max size
func NewLogBuffer(maxSize int) *LogBuffer {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &LogBuffer{
		entries:     make([]LogEntry, 0, maxSize),
		maxSize:     maxSize,
		nextID:      1,
		subscribers: make(map[chan LogEntry]struct{}),
		rateLimitMs: 1000, // 1 second between identical messages
	}
}

// Add adds a new log entry to the buffer with rate limiting
func (lb *LogBuffer) Add(level LogLevel, source, message string) {
	lb.mu.Lock()

	message = strings.TrimSpace(message)
	now := time.Now()

	// Check for repeated messages (rate limiting)
	if message == lb.lastMessage && source == lb.lastSource {
		elapsed := now.Sub(lb.lastTime).Milliseconds()
		if elapsed < lb.rateLimitMs {
			// Same message within rate limit window - just count it
			lb.repeatCount++
			lb.mu.Unlock()
			return
		}
	}

	// If we had repeated messages, log the count first
	if lb.repeatCount > 0 {
		repeatEntry := LogEntry{
			ID:        lb.nextID,
			Timestamp: now,
			Level:     LogLevelInfo,
			Source:    lb.lastSource,
			Message:   fmt.Sprintf("(previous message repeated %d times)", lb.repeatCount),
			Count:     lb.repeatCount,
		}
		lb.nextID++
		if len(lb.entries) >= lb.maxSize {
			lb.entries = lb.entries[1:]
		}
		lb.entries = append(lb.entries, repeatEntry)
		lb.repeatCount = 0
	}

	entry := LogEntry{
		ID:        lb.nextID,
		Timestamp: now,
		Level:     level,
		Source:    source,
		Message:   message,
	}
	lb.nextID++

	// Update last message tracking
	lb.lastMessage = message
	lb.lastSource = source
	lb.lastTime = now

	// Add to buffer, removing oldest if at capacity
	if len(lb.entries) >= lb.maxSize {
		lb.entries = lb.entries[1:]
	}
	lb.entries = append(lb.entries, entry)

	lb.mu.Unlock()

	// Broadcast to subscribers (non-blocking)
	lb.broadcast(entry)
}

// AddInfo adds an info level log
func (lb *LogBuffer) AddInfo(source, message string) {
	lb.Add(LogLevelInfo, source, message)
}

// AddWarn adds a warning level log
func (lb *LogBuffer) AddWarn(source, message string) {
	lb.Add(LogLevelWarn, source, message)
}

// AddError adds an error level log
func (lb *LogBuffer) AddError(source, message string) {
	lb.Add(LogLevelError, source, message)
}

// AddDebug adds a debug level log
func (lb *LogBuffer) AddDebug(source, message string) {
	lb.Add(LogLevelDebug, source, message)
}

// GetRecent returns the most recent n entries
func (lb *LogBuffer) GetRecent(n int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if n <= 0 || n > len(lb.entries) {
		n = len(lb.entries)
	}

	start := len(lb.entries) - n
	result := make([]LogEntry, n)
	copy(result, lb.entries[start:])

	return result
}

// GetSince returns all entries since the given ID
func (lb *LogBuffer) GetSince(sinceID int64) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var result []LogEntry
	for _, entry := range lb.entries {
		if entry.ID > sinceID {
			result = append(result, entry)
		}
	}

	return result
}

// GetAll returns all entries in the buffer
func (lb *LogBuffer) GetAll() []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	result := make([]LogEntry, len(lb.entries))
	copy(result, lb.entries)
	return result
}

// Clear removes all entries from the buffer
func (lb *LogBuffer) Clear() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.entries = lb.entries[:0]
	lb.repeatCount = 0
	lb.lastMessage = ""
	lb.lastSource = ""
}

// Subscribe returns a channel that receives new log entries
func (lb *LogBuffer) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)

	lb.subMu.Lock()
	lb.subscribers[ch] = struct{}{}
	lb.subMu.Unlock()

	return ch
}

// Unsubscribe removes a subscriber channel
func (lb *LogBuffer) Unsubscribe(ch chan LogEntry) {
	lb.subMu.Lock()
	delete(lb.subscribers, ch)
	lb.subMu.Unlock()

	close(ch)
}

// broadcast sends a log entry to all subscribers
func (lb *LogBuffer) broadcast(entry LogEntry) {
	lb.subMu.RLock()
	defer lb.subMu.RUnlock()

	for ch := range lb.subscribers {
		select {
		case ch <- entry:
		default:
			// Channel full, skip this subscriber
		}
	}
}

// Count returns the number of entries in the buffer
func (lb *LogBuffer) Count() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return len(lb.entries)
}

// LogWriter is an io.Writer that writes to the log buffer
type LogWriter struct {
	buffer  *LogBuffer
	level   LogLevel
	source  string
	lineBuf strings.Builder
}

// NewLogWriter creates a new LogWriter that writes to the buffer
func NewLogWriter(buffer *LogBuffer, level LogLevel, source string) *LogWriter {
	return &LogWriter{
		buffer: buffer,
		level:  level,
		source: source,
	}
}

// Write implements io.Writer
func (lw *LogWriter) Write(p []byte) (n int, err error) {
	n = len(p)

	for _, b := range p {
		if b == '\n' {
			// Complete line, send to buffer
			line := lw.lineBuf.String()
			if line != "" {
				level, source, message := lw.parseLine(line)
				if message != "" {
					lw.buffer.Add(level, source, message)
				}
			}
			lw.lineBuf.Reset()
		} else {
			lw.lineBuf.WriteByte(b)
		}
	}

	return n, nil
}

// parseLine extracts level, source, and message from a log line
// It handles Go's standard log format: "2006/01/02 15:04:05 [Source] message"
func (lw *LogWriter) parseLine(line string) (LogLevel, string, string) {
	level := lw.level
	source := lw.source
	message := line

	// Try to strip Go log timestamp prefix (e.g., "2026/01/03 01:22:21 ")
	// Format: YYYY/MM/DD HH:MM:SS
	if len(line) >= 20 && line[4] == '/' && line[7] == '/' && line[10] == ' ' && line[13] == ':' && line[16] == ':' && line[19] == ' ' {
		message = line[20:]
	}

	// Try to extract source from [Source] prefix
	if len(message) > 0 && message[0] == '[' {
		if idx := strings.Index(message, "] "); idx > 0 {
			source = message[1:idx]
			message = message[idx+2:]
		}
	}

	// Determine log level from content
	upperMsg := strings.ToUpper(message)
	if strings.Contains(upperMsg, "ERROR") || strings.Contains(upperMsg, "FATAL") || strings.HasPrefix(upperMsg, "ERR:") {
		level = LogLevelError
	} else if strings.Contains(upperMsg, "WARN") || strings.HasPrefix(upperMsg, "WARNING:") {
		level = LogLevelWarn
	} else if strings.Contains(upperMsg, "DEBUG") || strings.HasPrefix(message, "DEBUG:") {
		level = LogLevelDebug
	}

	return level, source, strings.TrimSpace(message)
}

// ActivityType represents the type of admin activity
type ActivityType string

const (
	ActivityListenerConnect    ActivityType = "listener_connect"
	ActivityListenerDisconnect ActivityType = "listener_disconnect"
	ActivitySourceStart        ActivityType = "source_start"
	ActivitySourceStop         ActivityType = "source_stop"
	ActivityConfigChange       ActivityType = "config_change"
	ActivityMountCreate        ActivityType = "mount_create"
	ActivityMountDelete        ActivityType = "mount_delete"
	ActivityServerStart        ActivityType = "server_start"
	ActivityServerStop         ActivityType = "server_stop"
	ActivityAdminAction        ActivityType = "admin_action"
	ActivityListenerSummary    ActivityType = "listener_summary" // Aggregated listener events
)

// ActivityEntry represents an admin activity event
type ActivityEntry struct {
	ID        int64                  `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      ActivityType           `json:"type"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// listenerEvent tracks a pending listener event for aggregation
type listenerEvent struct {
	mount       string
	connects    int
	disconnects int
	ips         map[string]struct{}
	firstTime   time.Time
	lastTime    time.Time
}

// ActivityBuffer stores admin activity events with rate limiting
type ActivityBuffer struct {
	entries     []ActivityEntry
	maxSize     int
	nextID      int64
	mu          sync.RWMutex
	subscribers map[chan ActivityEntry]struct{}
	subMu       sync.RWMutex

	// Rate limiting for listener events
	pendingListeners map[string]*listenerEvent // key: mount path
	listenerMu       sync.Mutex
	flushInterval    time.Duration
	stopFlush        chan struct{}
	flushRunning     bool
}

// NewActivityBuffer creates a new activity buffer
func NewActivityBuffer(maxSize int) *ActivityBuffer {
	if maxSize <= 0 {
		maxSize = 500
	}
	ab := &ActivityBuffer{
		entries:          make([]ActivityEntry, 0, maxSize),
		maxSize:          maxSize,
		nextID:           1,
		subscribers:      make(map[chan ActivityEntry]struct{}),
		pendingListeners: make(map[string]*listenerEvent),
		flushInterval:    5 * time.Second, // Aggregate events over 5 seconds
		stopFlush:        make(chan struct{}),
	}
	ab.startFlushLoop()
	return ab
}

// startFlushLoop starts the background goroutine that flushes aggregated events
func (ab *ActivityBuffer) startFlushLoop() {
	ab.listenerMu.Lock()
	if ab.flushRunning {
		ab.listenerMu.Unlock()
		return
	}
	ab.flushRunning = true
	ab.listenerMu.Unlock()

	go func() {
		ticker := time.NewTicker(ab.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ab.flushPendingListeners()
			case <-ab.stopFlush:
				ab.flushPendingListeners() // Final flush
				return
			}
		}
	}()
}

// Stop stops the flush loop
func (ab *ActivityBuffer) Stop() {
	ab.listenerMu.Lock()
	if ab.flushRunning {
		close(ab.stopFlush)
		ab.flushRunning = false
	}
	ab.listenerMu.Unlock()
}

// flushPendingListeners flushes any aggregated listener events
func (ab *ActivityBuffer) flushPendingListeners() {
	ab.listenerMu.Lock()
	pending := ab.pendingListeners
	ab.pendingListeners = make(map[string]*listenerEvent)
	ab.listenerMu.Unlock()

	for mount, evt := range pending {
		if evt.connects == 0 && evt.disconnects == 0 {
			continue
		}

		var msg string
		uniqueIPs := len(evt.ips)

		if evt.connects > 0 && evt.disconnects > 0 {
			msg = fmt.Sprintf("%s: %d connects, %d disconnects (%d unique IPs)",
				mount, evt.connects, evt.disconnects, uniqueIPs)
		} else if evt.connects > 0 {
			if evt.connects == 1 {
				msg = fmt.Sprintf("Listener connected to %s", mount)
			} else {
				msg = fmt.Sprintf("%d listeners connected to %s (%d unique IPs)",
					evt.connects, mount, uniqueIPs)
			}
		} else {
			if evt.disconnects == 1 {
				msg = fmt.Sprintf("Listener disconnected from %s", mount)
			} else {
				msg = fmt.Sprintf("%d listeners disconnected from %s",
					evt.disconnects, mount)
			}
		}

		data := map[string]interface{}{
			"mount":        mount,
			"connects":     evt.connects,
			"disconnects":  evt.disconnects,
			"unique_ips":   uniqueIPs,
			"period_start": evt.firstTime,
			"period_end":   evt.lastTime,
		}

		// Choose type based on which is more significant
		actType := ActivityListenerSummary
		if evt.connects > 0 && evt.disconnects == 0 {
			actType = ActivityListenerConnect
		} else if evt.disconnects > 0 && evt.connects == 0 {
			actType = ActivityListenerDisconnect
		}

		ab.addDirect(actType, msg, data)
	}
}

// Add adds a new activity entry (for non-listener events)
func (ab *ActivityBuffer) Add(actType ActivityType, message string, data map[string]interface{}) {
	// Route listener events through aggregation
	if actType == ActivityListenerConnect || actType == ActivityListenerDisconnect {
		ab.addListenerEvent(actType, data)
		return
	}

	ab.addDirect(actType, message, data)
}

// addDirect adds an entry directly without aggregation
func (ab *ActivityBuffer) addDirect(actType ActivityType, message string, data map[string]interface{}) {
	ab.mu.Lock()

	entry := ActivityEntry{
		ID:        ab.nextID,
		Timestamp: time.Now(),
		Type:      actType,
		Message:   message,
		Data:      data,
	}
	ab.nextID++

	if len(ab.entries) >= ab.maxSize {
		ab.entries = ab.entries[1:]
	}
	ab.entries = append(ab.entries, entry)

	ab.mu.Unlock()

	ab.broadcast(entry)
}

// addListenerEvent aggregates listener connect/disconnect events
func (ab *ActivityBuffer) addListenerEvent(actType ActivityType, data map[string]interface{}) {
	mount, _ := data["mount"].(string)
	ip, _ := data["ip"].(string)

	if mount == "" {
		return
	}

	ab.listenerMu.Lock()
	defer ab.listenerMu.Unlock()

	evt, exists := ab.pendingListeners[mount]
	if !exists {
		evt = &listenerEvent{
			mount:     mount,
			ips:       make(map[string]struct{}),
			firstTime: time.Now(),
		}
		ab.pendingListeners[mount] = evt
	}

	evt.lastTime = time.Now()
	if ip != "" {
		evt.ips[ip] = struct{}{}
	}

	if actType == ActivityListenerConnect {
		evt.connects++
	} else {
		evt.disconnects++
	}
}

// Helper methods for common activities
func (ab *ActivityBuffer) ListenerConnected(mount, ip, userAgent string) {
	ab.Add(ActivityListenerConnect, "", map[string]interface{}{
		"mount":      mount,
		"ip":         ip,
		"user_agent": userAgent,
	})
}

func (ab *ActivityBuffer) ListenerDisconnected(mount, ip string, duration time.Duration) {
	ab.Add(ActivityListenerDisconnect, "", map[string]interface{}{
		"mount":    mount,
		"ip":       ip,
		"duration": duration.Seconds(),
	})
}

func (ab *ActivityBuffer) SourceStarted(mount, name string, bitrate int) {
	ab.Add(ActivitySourceStart, fmt.Sprintf("Source started on %s: %s", mount, name), map[string]interface{}{
		"mount":   mount,
		"name":    name,
		"bitrate": bitrate,
	})
}

func (ab *ActivityBuffer) SourceStopped(mount string, duration time.Duration) {
	ab.Add(ActivitySourceStop, fmt.Sprintf("Source stopped on %s after %s", mount, duration.Round(time.Second)), map[string]interface{}{
		"mount":    mount,
		"duration": duration.Seconds(),
	})
}

func (ab *ActivityBuffer) ConfigChanged(section, description string) {
	ab.Add(ActivityConfigChange, fmt.Sprintf("Config changed: %s - %s", section, description), map[string]interface{}{
		"section":     section,
		"description": description,
	})
}

func (ab *ActivityBuffer) MountCreated(mount string) {
	ab.Add(ActivityMountCreate, fmt.Sprintf("Mount created: %s", mount), map[string]interface{}{
		"mount": mount,
	})
}

func (ab *ActivityBuffer) MountDeleted(mount string) {
	ab.Add(ActivityMountDelete, fmt.Sprintf("Mount deleted: %s", mount), map[string]interface{}{
		"mount": mount,
	})
}

func (ab *ActivityBuffer) AdminAction(action, details string) {
	ab.Add(ActivityAdminAction, fmt.Sprintf("Admin: %s - %s", action, details), map[string]interface{}{
		"action":  action,
		"details": details,
	})
}

// GetRecent returns the most recent n entries
func (ab *ActivityBuffer) GetRecent(n int) []ActivityEntry {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	if n <= 0 || n > len(ab.entries) {
		n = len(ab.entries)
	}

	start := len(ab.entries) - n
	result := make([]ActivityEntry, n)
	copy(result, ab.entries[start:])

	return result
}

// GetSince returns all entries since the given ID
func (ab *ActivityBuffer) GetSince(sinceID int64) []ActivityEntry {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	var result []ActivityEntry
	for _, entry := range ab.entries {
		if entry.ID > sinceID {
			result = append(result, entry)
		}
	}

	return result
}

// GetAll returns all entries
func (ab *ActivityBuffer) GetAll() []ActivityEntry {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	result := make([]ActivityEntry, len(ab.entries))
	copy(result, ab.entries)
	return result
}

// Clear removes all entries
func (ab *ActivityBuffer) Clear() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	ab.entries = ab.entries[:0]
}

// Subscribe returns a channel for new activity entries
func (ab *ActivityBuffer) Subscribe() chan ActivityEntry {
	ch := make(chan ActivityEntry, 50)

	ab.subMu.Lock()
	ab.subscribers[ch] = struct{}{}
	ab.subMu.Unlock()

	return ch
}

// Unsubscribe removes a subscriber
func (ab *ActivityBuffer) Unsubscribe(ch chan ActivityEntry) {
	ab.subMu.Lock()
	delete(ab.subscribers, ch)
	ab.subMu.Unlock()

	close(ch)
}

// broadcast sends an activity entry to all subscribers
func (ab *ActivityBuffer) broadcast(entry ActivityEntry) {
	ab.subMu.RLock()
	defer ab.subMu.RUnlock()

	for ch := range ab.subscribers {
		select {
		case ch <- entry:
		default:
			// Channel full, skip
		}
	}
}

// Count returns the number of entries
func (ab *ActivityBuffer) Count() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.entries)
}

// MultiWriter combines multiple io.Writers
func MultiWriter(writers ...io.Writer) io.Writer {
	allWriters := make([]io.Writer, 0, len(writers))
	for _, w := range writers {
		if mw, ok := w.(*multiWriter); ok {
			allWriters = append(allWriters, mw.writers...)
		} else {
			allWriters = append(allWriters, w)
		}
	}
	return &multiWriter{writers: allWriters}
}

type multiWriter struct {
	writers []io.Writer
}

func (t *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}
