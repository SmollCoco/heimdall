package types

import "time"

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
	labelCopy := make(map[string]string, len(labels))
	for k, v := range labels {
		labelCopy[k] = v
	}

	return &LogEntry{
		Line:      line,
		Timestamp: time.Now(),
		Source:    source,
		Labels:    labelCopy,
		StreamID:  generateStreamID(source, labels),
	}
}

// generateStreamID creates a unique identifier for the log stream
// Format: "{source}:{label1=value1,label2=value2}"
func generateStreamID(source string, labels map[string]string) string {
	// Just return source for now
	return source
}
