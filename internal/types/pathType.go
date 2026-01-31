// pathType.go
package types

import "fmt"

// PathType represents the type of input source
type PathType int

const (
	File PathType = iota
	Directory
)

// UnmarshalYAML converts YAML string to PathType enum
func (pt *PathType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	switch s {
	case "file":
		*pt = File
	case "directory":
		*pt = Directory
	default:
		return fmt.Errorf("invalid path_type: %s (must be 'file' or 'directory')", s)
	}
	return nil
}

// String returns the string representation (useful for logging)
func (pt PathType) String() string {
	switch pt {
	case File:
		return "file"
	case Directory:
		return "directory"
	default:
		return "unknown"
	}
}
