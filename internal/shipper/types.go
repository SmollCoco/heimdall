package shipper

// LokiPushRequest represents the JSON structure Loki expects
type LokiPushRequest struct {
	Streams []LokiStream `json:"streams"`
}

// LokiStream represents a single log stream (grouped by labels)
type LokiStream struct {
	Stream map[string]string `json:"stream"` // Labels
	Values []LokiValue       `json:"values"` // Log entries
}

// LokiValue is a [timestamp, log_line] pair
type LokiValue [2]string // [0] = timestamp (nanoseconds), [1] = log line
