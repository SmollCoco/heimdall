// internal/processor/detector.go
package processor

import (
	"strings"

	"github.com/SmollCoco/heimdall/internal/types"
)

// detectLogLevel analyzes a log line and returns the log level
func detectLogLevel(line []byte) types.LogLevel {
	content := strings.ToLower(string(line))

	if strings.Contains(content, "error") || strings.Contains(content, "fatal") {
		return types.LogLevelError
	}
	if strings.Contains(content, "warn") || strings.Contains(content, "warning") {
		return types.LogLevelWarn
	}
	if strings.Contains(content, "info") {
		return types.LogLevelInfo
	}
	if strings.Contains(content, "debug") {
		return types.LogLevelDebug
	}

	return types.LogLevelUnknown
}
