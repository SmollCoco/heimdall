package shipper

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SmollCoco/heimdall/internal/types"
)

// groupByLabels groups ProcessedEntries by their labels
// Returns a map where key = label set, value = entries with those labels
func groupByLabels(entries []*types.ProcessedEntry) map[string][]*types.ProcessedEntry {
	// Create a string key from labels (sorted to ensure consistency)
	// Example key: "level=error,service=nginx"
	res := make(map[string][]*types.ProcessedEntry)

	for _, entry := range entries {
		if entry == nil || entry.LogEntry == nil {
			log.Printf("WARNING [groupByLabels]: Skipping nil entry or nil LogEntry")
			continue
		}

		labels := entry.GetLabels()
		log.Printf("DEBUG [groupByLabels]: Processing entry with labels: %+v, level: %s", labels, entry.Level)
		key := labelsToKey(labels, entry.Level)
		res[key] = append(res[key], entry)
	}

	return res
}

// formatLokiRequest converts ProcessedEntries to Loki JSON format
func formatLokiRequest(entries []*types.ProcessedEntry) *LokiPushRequest {
	// Group entries by labels
	grouped := groupByLabels(entries)

	// Create streams
	streams := make([]LokiStream, 0, len(grouped))

	for _, groupEntries := range grouped {
		stream := createStream(groupEntries)
		streams = append(streams, stream)
	}

	return &LokiPushRequest{
		Streams: streams,
	}
}

// createStream creates a LokiStream from a group of entries with same labels
func createStream(entries []*types.ProcessedEntry) LokiStream {
	// 1. Extract labels from first entry (all have same labels in this group)
	// 2. Convert each entry to LokiValue [timestamp, log_line]
	// 3. Return LokiStream

	if len(entries) == 0 {
		return LokiStream{
			Stream: map[string]string{},
			Values: []LokiValue{},
		}
	}

	firstEntry := entries[0]

	if firstEntry == nil || firstEntry.LogEntry == nil {
		log.Printf("WARNING [createStream]: First entry or LogEntry is nil")
		return LokiStream{
			Stream: map[string]string{},
			Values: []LokiValue{},
		}
	}

	entryLabels := firstEntry.GetLabels()
	log.Printf("DEBUG [createStream]: First entry labels: %+v", entryLabels)

	labels := make(map[string]string)
	for k, v := range entryLabels {
		labels[k] = v
	}
	labels["level"] = firstEntry.Level.String()

	log.Printf("DEBUG [createStream]: Final stream labels: %+v", labels)

	values := make([]LokiValue, 0, len(entries))
	for _, entry := range entries {
		timestamp := timestampToNanos(entry.Timestamp)
		values = append(values, LokiValue{timestamp, string(entry.Line)})
	}

	return LokiStream{
		Stream: labels,
		Values: values,
	}
}

// timestampToNanos converts a time.Time to nanosecond string
func timestampToNanos(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}

func labelsToKey(labels map[string]string, level types.LogLevel) string {
	// Combine all labels into a sorted string
	// Include log level in the key

	keys := make([]string, 0, len(labels)+1)

	// Add level
	keys = append(keys, fmt.Sprintf("level=%s", level.String()))

	// Add other labels
	if labels != nil {
		for k, v := range labels {
			keys = append(keys, fmt.Sprintf("%s=%s", k, v))
		}
	} else {
		log.Printf("WARNING [labelsToKey]: labels is nil!")
	}

	sort.Strings(keys) // Sort for consistency
	return strings.Join(keys, ",")
}
