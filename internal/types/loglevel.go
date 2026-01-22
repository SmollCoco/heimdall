package types

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	LogLevelUnknown LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns the string representation of the log level
func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	case LogLevelUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseLogLevel converts a string to a LogLevel
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error", "fatal":
		return LogLevelError
	default:
		return LogLevelUnknown
	}
}

// UnmarshalYAML implements custom YAML unmarshaling (for future config use)
func (ll *LogLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	*ll = ParseLogLevel(s)
	return nil
}

// MarshalYAML implements custom YAML marshaling
func (ll LogLevel) MarshalYAML() (interface{}, error) {
	return ll.String(), nil
}
