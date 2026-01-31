package config

import (
	"fmt"
	"os"
	"time"
	"github.com/SmollCoco/heimdall/internal/types"
	"gopkg.in/yaml.v3"
	"strings"
)

// Default values
const (
    DefaultWorkerPoolSize    = 4
    DefaultChannelBufferSize = 1000
    DefaultBatchSize         = 100
    DefaultBatchTimeout      = "5s"
    DefaultMaxAttempts       = 5
    DefaultInitialBackoff    = "1s"
    DefaultMaxBackoff        = "30s"
    DefaultMonitoringInterval = "30s"
)

// LoadConfig loads and validates configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Read file
    data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
    
	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set default
	setDefaults(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func Validate(cfg *Config) error {
    // 1. Validate Inputs
	if err := validateInputs(cfg.Inputs); err != nil {
		return err
	}
	// 2. Validate Output
	if err := validateOutput(&cfg.Output); err != nil {
		return err
	}
	// 3. Validate Performance
	if err := validatePerformance(cfg); err != nil {
		return err
	}
	// 4. Validate Retry
	if err := validateRetry(&cfg.Retry); err != nil {
		return err
	}
	// 5. Return nil if all validations pass
    return nil
}

func validateInputs(inputs []InputSource) error {
    // 1. Check if inputs array is empty
	if len(inputs) == 0 {
		return fmt.Errorf("no input sources defined")
	}
    // 2. For each input:
	for i, input := range inputs {
		//    a. Check if Path is empty
		if len(input.Path) == 0 {
			return fmt.Errorf("input source %d has empty path", i)
		}
		//    b. Check if path exists on disk (use os.Stat)
		info, err := os.Stat(input.Path)
		if err != nil {
			return fmt.Errorf("input source %d path does not exist: %w", i, err)
		}
		//    c. If PathType is File, check it's not a directory
		if info.IsDir() && input.PathType == types.File {
			return fmt.Errorf("input source %d path is a directory but path_type is 'file'", i)
		}
		//    d. If PathType is Directory, check it's not a file
		if !info.IsDir() && input.PathType == types.Directory {
			return fmt.Errorf("input source %d path is a file but path_type is 'directory'", i)
		}
	}
    // 3. Return nil if all checks pass
	return nil
}

func validateOutput(output *Output) error {
    // 1. Check if Loki.URL is empty
	if len(output.Loki.URL) == 0 {
		return fmt.Errorf("loki output URL is empty")
	}
    // 2. Check if URL starts with "http://" or "https://"
	if !strings.HasPrefix(output.Loki.URL, "http://") && !strings.HasPrefix(output.Loki.URL, "https://") {
		return fmt.Errorf("loki output URL must start with 'http://' or 'https://'")
	}
    // 3. Check if BatchSize > 0
	if output.Loki.BatchSize <= 0 {
		return fmt.Errorf("loki output batch_size must be greater than 0")
	}
    // 4. Parse and validate BatchTimeout duration
	duration, err := time.ParseDuration(output.Loki.BatchTimeoutSeconds)
	if err != nil {
		return fmt.Errorf("invalid loki output batch_timeout: %w", err)
	}
	if duration <= 0 || duration >= time.Hour {
		return fmt.Errorf("loki output batch_timeout must be between 0 and 1 hour")
	}
    // 5. Return nil if all checks pass
	return nil
}

func validatePerformance(cfg *Config) error {
    // 1. Check WorkerPoolSize is between 1 and 100
	if cfg.WorkerPoolSize < 1 || cfg.WorkerPoolSize > 100 {
		return fmt.Errorf("worker_pool_size must be between 1 and 100")
	}
    // 2. Check ChannelBufferSize is between 1 and 100,000
	if cfg.ChannelBufferSize < 1 || cfg.ChannelBufferSize > 100000 {
		return fmt.Errorf("channel_buffer_size must be between 1 and 100000")
	}
    // 3. Return nil if valid
	return nil
}

func validateRetry(retry *Retry) error {
    // 1. Check MaxAttempts > 0
	if retry.MaxAttempts <= 0 {
		return fmt.Errorf("retry max_attempts must be greater than 0")
	}
    // 2. Parse InitialBackoff and MaxBackoff durations
	initialBackoff, err := time.ParseDuration(retry.InitialBackoffSeconds)
	if err != nil {
		return fmt.Errorf("invalid retry initial_backoff: %w", err)
	}
	maxBackoff, err := time.ParseDuration(retry.MaxBackoffSeconds)
	if err != nil {
		return fmt.Errorf("invalid retry max_backoff: %w", err)
	}
    // 3. Check InitialBackoff <= MaxBackoff
	if initialBackoff > maxBackoff {
		return fmt.Errorf("retry initial_backoff must be less than or equal to max_backoff")
	}
    // 4. Return nil if valid
	return nil
}

// setDefaults fills in default values for optional fields
func setDefaults(cfg *Config) {
    if cfg.WorkerPoolSize == 0 {
		cfg.WorkerPoolSize = DefaultWorkerPoolSize
	}

	if cfg.ChannelBufferSize == 0 {
		cfg.ChannelBufferSize = DefaultChannelBufferSize
	}

	if cfg.Output.Loki.BatchSize == 0 {
		cfg.Output.Loki.BatchSize = DefaultBatchSize
	}

	if cfg.Output.Loki.BatchTimeoutSeconds == "" {
		cfg.Output.Loki.BatchTimeoutSeconds = DefaultBatchTimeout
	}

	if cfg.Retry.MaxAttempts == 0 {
		cfg.Retry.MaxAttempts = DefaultMaxAttempts
	}

	if cfg.Retry.InitialBackoffSeconds == "" {
		cfg.Retry.InitialBackoffSeconds = DefaultInitialBackoff
	}

	if cfg.Retry.MaxBackoffSeconds == "" {
		cfg.Retry.MaxBackoffSeconds = DefaultMaxBackoff
	}

	if cfg.SelfMonitoring.Enabled && cfg.SelfMonitoring.IntervalSecs == "" {
		cfg.SelfMonitoring.IntervalSecs = DefaultMonitoringInterval
	}
}
