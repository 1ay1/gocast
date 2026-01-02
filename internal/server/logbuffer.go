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
}

// LogBuffer is a circular buffer that stores log entries and broadcasts to subscribers
type LogBuffer struct {
	entries     []LogEntry
	maxSize     int
	nextID      int64
	mu          sync.RWMutex
	subscribers map[chan LogEntry]struct{}
	subMu       sync.RWMutex
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
	}
}

// Add adds a new log entry to the buffer
func (lb *LogBuffer) Add(level LogLevel, source, message string) {
	lb.mu.Lock()

	entry := LogEntry{
		ID:        lb.nextID,
		Timestamp: time.Now(),
		Level:     level,
		Source:    source,
		Message:   strings.TrimSpace(message),
	}
	lb.nextID++

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
				// Try to parse log level from the line
				level := lw.level
				if strings.Contains(line, "ERROR") || strings.Contains(line, "error:") {
					level = LogLevelError
				} else if strings.Contains(line, "WARN") || strings.Contains(line, "warning:") {
					level = LogLevelWarn
				} else if strings.Contains(line, "DEBUG") {
					level = LogLevelDebug
				}

				lw.buffer.Add(level, lw.source, line)
			}
			lw.lineBuf.Reset()
		} else {
			lw.lineBuf.WriteByte(b)
		}
	}

	return n, nil
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
)

// ActivityEntry represents an admin activity event
type ActivityEntry struct {
	ID        int64                  `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      ActivityType           `json:"type"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// ActivityBuffer stores admin activity events
type ActivityBuffer struct {
	entries     []ActivityEntry
	maxSize     int
	nextID      int64
	mu          sync.RWMutex
	subscribers map[chan ActivityEntry]struct{}
	subMu       sync.RWMutex
}

// NewActivityBuffer creates a new activity buffer
func NewActivityBuffer(maxSize int) *ActivityBuffer {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &ActivityBuffer{
		entries:     make([]ActivityEntry, 0, maxSize),
		maxSize:     maxSize,
		nextID:      1,
		subscribers: make(map[chan ActivityEntry]struct{}),
	}
}

// Add adds a new activity entry
func (ab *ActivityBuffer) Add(actType ActivityType, message string, data map[string]interface{}) {
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

// Helper methods for common activities
func (ab *ActivityBuffer) ListenerConnected(mount, ip, userAgent string) {
	ab.Add(ActivityListenerConnect, fmt.Sprintf("Listener connected to %s", mount), map[string]interface{}{
		"mount":      mount,
		"ip":         ip,
		"user_agent": userAgent,
	})
}

func (ab *ActivityBuffer) ListenerDisconnected(mount, ip string, duration time.Duration) {
	ab.Add(ActivityListenerDisconnect, fmt.Sprintf("Listener disconnected from %s after %s", mount, duration.Round(time.Second)), map[string]interface{}{
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
