package types

import (
	"log"
	"time"
)

// LogEntry represents a single log line with metadata
type LogEntry struct {
	// Raw log content (what was read from the file)
	Line []byte
	// When the watcher read this line (not when it was written to the file)
	Timestamp time.Time
	// Source file path (e.g., "/var/log/nginx/access.log")
	Source string
	// Labels from configuration (e.g., {"service": "nginx", "environment": "production"})
	Labels map[string]string
	// Optional: Stream identifier (useful for Loki)
	// This helps group logs from the same source
	StreamID string
}

// NewLogEntry creates a LogEntry with timestamp set to now
func NewLogEntry(line []byte, source string, labels map[string]string) *LogEntry {
	log.Printf("DEBUG [NewLogEntry]: Received labels: %+v", labels)

	labelCopy := make(map[string]string, len(labels))
	if labels != nil {
		for k, v := range labels {
			labelCopy[k] = v
		}
	}

	entry := &LogEntry{
		Line:      line,
		Timestamp: time.Now(),
		Source:    source,
		Labels:    labelCopy,
		StreamID:  generateStreamID(source, labelCopy),
	}

	log.Printf("DEBUG [NewLogEntry]: Created entry with labels: %+v", entry.Labels)

	return entry
}

// generateStreamID creates a unique identifier for the log stream
// Format: "{source}:{label1=value1,label2=value2}"
func generateStreamID(source string, labels map[string]string) string {
	// Just return source for now
	return source
}
