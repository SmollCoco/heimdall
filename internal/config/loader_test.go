package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "input.log")
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o644))

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(buildValidYAML(logPath)), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, logPath, cfg.Inputs[0].Path)
	assert.Equal(t, 200, cfg.Output.Loki.BatchSize)
	assert.Equal(t, "10s", cfg.Output.Loki.BatchTimeoutSeconds)
	assert.Equal(t, DefaultWorkerPoolSize, cfg.WorkerPoolSize)
	assert.Equal(t, DefaultChannelBufferSize, cfg.ChannelBufferSize)
}

func TestLoadConfig_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadConfig("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestLoadConfig_UnreadableFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(buildValidYAML(cfgPath)), 0o000))

	_, err := LoadConfig(cfgPath)
	assert.Error(t, err)
}

func TestLoadConfig_InvalidConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	content := "inputs: []\noutput:\n  loki:\n    url: "
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	_, err := LoadConfig(cfgPath)
	assert.Error(t, err)
}
