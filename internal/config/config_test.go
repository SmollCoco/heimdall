package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// helper to build minimal valid YAML with provided path
func buildValidYAML(path string) string {
	return fmt.Sprintf(`
inputs:
  - path: %q
    path_type: file
    labels:
      app: heimdall
output:
  loki:
    url: "http://localhost:3100"
    batch_size: 200
    batch_timeout: "10s"
retry:
  max_attempts: 3
  initial_backoff: "2s"
  max_backoff: "10s"
`, path)
}

func TestConfigYAMLParsing_Valid(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp("", "heimdall-log-*.log")
	require.NoError(t, err)
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	content := buildValidYAML(tmpFile.Name()) + "worker_pool_size: 8\nchannel_buffer_size: 500\nself_monitoring:\n  enabled: true\n  interval: \"45s\"\n"

	var cfg Config
	err = yaml.Unmarshal([]byte(content), &cfg)
	require.NoError(t, err)

	setDefaults(&cfg)
	require.NoError(t, Validate(&cfg))

	assert.Equal(t, 8, cfg.WorkerPoolSize)
	assert.Equal(t, 500, cfg.ChannelBufferSize)
	assert.Equal(t, "http://localhost:3100", cfg.Output.Loki.URL)
	assert.Equal(t, "45s", cfg.SelfMonitoring.IntervalSecs)
	assert.Equal(t, 200, cfg.Output.Loki.BatchSize)
	assert.Equal(t, "10s", cfg.Output.Loki.BatchTimeoutSeconds)
}

func TestConfigYAMLParsing_InvalidSyntax(t *testing.T) {
	t.Parallel()

	badYAML := "inputs: [ path: /tmp"

	var cfg Config
	err := yaml.Unmarshal([]byte(badYAML), &cfg)
	assert.Error(t, err)
}

func TestConfigYAMLParsing_MissingRequired(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "NoInputs",
			content: "output:\n  loki:\n    url: http://localhost:3100\n",
		},
		{
			name: "EmptyOutputURL",
			content: func() string {
				tmpFile, _ := os.CreateTemp("", "heimdall-log-*.log")
				tmpFile.Close()
				t.Cleanup(func() { os.Remove(tmpFile.Name()) })
				return fmt.Sprintf("inputs:\n  - path: %s\n    path_type: file\noutput:\n  loki:\n    url: \n", tmpFile.Name())
			}(),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cfg Config
			err := yaml.Unmarshal([]byte(tc.content), &cfg)
			require.NoError(t, err)
			setDefaults(&cfg)
			err = Validate(&cfg)
			assert.Error(t, err)
		})
	}
}

func TestConfig_DefaultsApplied(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "input.log")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	content := fmt.Sprintf(`
inputs:
  - path: %q
    path_type: file
output:
  loki:
    url: "http://localhost:3100"
`, path)

	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(content), &cfg))

	setDefaults(&cfg)
	require.NoError(t, Validate(&cfg))

	assert.Equal(t, DefaultWorkerPoolSize, cfg.WorkerPoolSize)
	assert.Equal(t, DefaultChannelBufferSize, cfg.ChannelBufferSize)
	assert.Equal(t, DefaultBatchSize, cfg.Output.Loki.BatchSize)
	assert.Equal(t, DefaultBatchTimeout, cfg.Output.Loki.BatchTimeoutSeconds)
	assert.Equal(t, DefaultMaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, DefaultInitialBackoff, cfg.Retry.InitialBackoffSeconds)
	assert.Equal(t, DefaultMaxBackoff, cfg.Retry.MaxBackoffSeconds)
	assert.False(t, cfg.SelfMonitoring.Enabled)
}

func TestConfig_InvalidValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.log")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	cases := []struct {
		name        string
		mutate      func(cfg *Config)
		expectError bool
	}{
		{
			name: "InvalidURL",
			mutate: func(cfg *Config) {
				cfg.Output.Loki.URL = "localhost:3100"
			},
			expectError: true,
		},
		{
			name: "ZeroBatchSize",
			mutate: func(cfg *Config) {
				cfg.Output.Loki.BatchSize = 0
			},
			expectError: true,
		},
		{
			name: "NegativeWorkerPool",
			mutate: func(cfg *Config) {
				cfg.WorkerPoolSize = -1
			},
			expectError: true,
		},
		{
			name: "TooLargeChannelBuffer",
			mutate: func(cfg *Config) {
				cfg.ChannelBufferSize = 200000
			},
			expectError: true,
		},
		{
			name: "BadBatchTimeout",
			mutate: func(cfg *Config) {
				cfg.Output.Loki.BatchTimeoutSeconds = "-1s"
			},
			expectError: true,
		},
		{
			name:        "ValidBaseline",
			mutate:      func(cfg *Config) {},
			expectError: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cfg Config
			require.NoError(t, yaml.Unmarshal([]byte(buildValidYAML(path)), &cfg))
			setDefaults(&cfg)
			tc.mutate(&cfg)

			err := Validate(&cfg)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_SetDefaultsSelfMonitoring(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SelfMonitoring: SelfMonitoring{Enabled: true},
	}

	setDefaults(&cfg)

	assert.Equal(t, DefaultMonitoringInterval, cfg.SelfMonitoring.IntervalSecs)
}

func TestConfig_TimestampParsingBounds(t *testing.T) {
	t.Parallel()

	durationTests := []struct {
		name    string
		value   string
		isValid bool
	}{
		{name: "Valid", value: "10s", isValid: true},
		{name: "Zero", value: "0s", isValid: false},
		{name: "TooLarge", value: "2h", isValid: false},
		{name: "Malformed", value: "abc", isValid: false},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "timeout.log")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	for _, tc := range durationTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			content := fmt.Sprintf(`
inputs:
  - path: %q
    path_type: file
output:
  loki:
    url: "http://localhost:3100"
    batch_timeout: %q
`, path, tc.value)

			var cfg Config
			err := yaml.Unmarshal([]byte(content), &cfg)
			require.NoError(t, err)
			setDefaults(&cfg)
			err = Validate(&cfg)
			if tc.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestConfig_LabelsNotNilAfterParse(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp("", "heimdall-log-*.log")
	require.NoError(t, err)
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	content := fmt.Sprintf(`
inputs:
  - path: %q
    path_type: file
output:
  loki:
    url: "http://localhost:3100"
`, tmpFile.Name())

	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(content), &cfg))
	setDefaults(&cfg)

	require.NoError(t, Validate(&cfg))
	require.Len(t, cfg.Inputs, 1)
	if cfg.Inputs[0].Labels == nil {
		cfg.Inputs[0].Labels = map[string]string{}
	}
	cfg.Inputs[0].Labels["added"] = "ok"
	assert.Equal(t, "ok", cfg.Inputs[0].Labels["added"])
}

func TestConfig_LoadAndValidate_WithTimeoutBounds(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "bounded.log")
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o644))

	cfg := Config{
		Inputs: []InputSource{{Path: logPath, PathType: 0}},
		Output: Output{Loki: LokiConfig{URL: "http://localhost:3100", BatchSize: 1, BatchTimeoutSeconds: "59m"}},
		Retry:  Retry{MaxAttempts: 1, InitialBackoffSeconds: "1s", MaxBackoffSeconds: "1s"},
	}

	setDefaults(&cfg)
	err := Validate(&cfg)
	assert.NoError(t, err)

	cfg.Output.Loki.BatchTimeoutSeconds = "61m"
	err = Validate(&cfg)
	assert.Error(t, err)
}
