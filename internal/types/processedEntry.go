// internal/types/processed.go
package types

import "time"

type ProcessedEntry struct {
	*LogEntry

	Level        LogLevel
	ParseSuccess bool
	ParseError   string
	ProcessedAt  time.Time
}

func NewProcessedEntry(entry *LogEntry) *ProcessedEntry {
	return &ProcessedEntry{
		LogEntry:     entry,
		Level:        LogLevelUnknown,
		ParseSuccess: true,
		ParseError:   "",
		ProcessedAt:  time.Now(),
	}
}

// GetLabels safely returns labels, handling nil LogEntry
func (pe *ProcessedEntry) GetLabels() map[string]string {
	if pe == nil || pe.LogEntry == nil {
		return make(map[string]string)
	}
	return pe.LogEntry.Labels
}
