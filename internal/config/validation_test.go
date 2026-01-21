package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInputs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "input.log")
	require.NoError(t, writeEmptyFile(filePath))

	cases := []struct {
		name    string
		inputs  []InputSource
		wantErr bool
	}{
		{
			name:    "NoInputs",
			inputs:  []InputSource{},
			wantErr: true,
		},
		{
			name: "MissingPath",
			inputs: []InputSource{{
				Path:     "",
				PathType: types.File,
			}},
			wantErr: true,
		},
		{
			name: "NonExistent",
			inputs: []InputSource{{
				Path:     filepath.Join(tmpDir, "nope.log"),
				PathType: types.File,
			}},
			wantErr: true,
		},
		{
			name: "PathTypeMismatch_FileGivenDirectory",
			inputs: []InputSource{{
				Path:     tmpDir,
				PathType: types.File,
			}},
			wantErr: true,
		},
		{
			name: "PathTypeMismatch_DirGivenFile",
			inputs: []InputSource{{
				Path:     filePath,
				PathType: types.Directory,
			}},
			wantErr: true,
		},
		{
			name: "Valid",
			inputs: []InputSource{{
				Path:     filePath,
				PathType: types.File,
			}},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateInputs(tc.inputs)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		output  Output
		wantErr bool
	}{
		{
			name:    "EmptyURL",
			output:  Output{Loki: LokiConfig{URL: "", BatchSize: 1, BatchTimeoutSeconds: "1s"}},
			wantErr: true,
		},
		{
			name:    "InvalidScheme",
			output:  Output{Loki: LokiConfig{URL: "localhost:3100", BatchSize: 1, BatchTimeoutSeconds: "1s"}},
			wantErr: true,
		},
		{
			name:    "ZeroBatchSize",
			output:  Output{Loki: LokiConfig{URL: "http://localhost", BatchSize: 0, BatchTimeoutSeconds: "1s"}},
			wantErr: true,
		},
		{
			name:    "BadDuration",
			output:  Output{Loki: LokiConfig{URL: "http://localhost", BatchSize: 1, BatchTimeoutSeconds: "invalid"}},
			wantErr: true,
		},
		{
			name:    "TooLargeDuration",
			output:  Output{Loki: LokiConfig{URL: "http://localhost", BatchSize: 1, BatchTimeoutSeconds: "2h"}},
			wantErr: true,
		},
		{
			name:    "Valid",
			output:  Output{Loki: LokiConfig{URL: "http://localhost", BatchSize: 10, BatchTimeoutSeconds: "30s"}},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateOutput(&tc.output)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePerformance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "TooSmallWorkerPool", cfg: Config{WorkerPoolSize: 0, ChannelBufferSize: 10}, wantErr: true},
		{name: "TooLargeWorkerPool", cfg: Config{WorkerPoolSize: 101, ChannelBufferSize: 10}, wantErr: true},
		{name: "TooSmallChannel", cfg: Config{WorkerPoolSize: 1, ChannelBufferSize: 0}, wantErr: true},
		{name: "TooLargeChannel", cfg: Config{WorkerPoolSize: 1, ChannelBufferSize: 200000}, wantErr: true},
		{name: "Valid", cfg: Config{WorkerPoolSize: 5, ChannelBufferSize: 100}, wantErr: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validatePerformance(&tc.cfg)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRetry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		retry   Retry
		wantErr bool
	}{
		{name: "ZeroAttempts", retry: Retry{MaxAttempts: 0, InitialBackoffSeconds: "1s", MaxBackoffSeconds: "1s"}, wantErr: true},
		{name: "BadInitialBackoff", retry: Retry{MaxAttempts: 1, InitialBackoffSeconds: "abc", MaxBackoffSeconds: "1s"}, wantErr: true},
		{name: "BadMaxBackoff", retry: Retry{MaxAttempts: 1, InitialBackoffSeconds: "1s", MaxBackoffSeconds: "abc"}, wantErr: true},
		{name: "InitialGreaterThanMax", retry: Retry{MaxAttempts: 1, InitialBackoffSeconds: "2s", MaxBackoffSeconds: "1s"}, wantErr: true},
		{name: "Valid", retry: Retry{MaxAttempts: 2, InitialBackoffSeconds: "1s", MaxBackoffSeconds: "5s"}, wantErr: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateRetry(&tc.retry)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// writeEmptyFile creates an empty file with default permissions
func writeEmptyFile(path string) error {
	return os.WriteFile(path, []byte(""), 0o644)
}
