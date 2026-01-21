package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchFile_DetectsWrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o644))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 4)
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchFile(ctx, watcher, config.InputSource{Path: logPath, PathType: types.File, Labels: map[string]string{"service": "api"}}, out)
	}()

	time.Sleep(50 * time.Millisecond)
	appendLine(t, logPath, "hello world\n")

	entry := waitForEntry(t, out)
	assert.Equal(t, "hello world", string(entry.Line))
	assert.Equal(t, "api", entry.Labels["service"])

	cancel()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("watchFile did not exit after cancellation")
	}
}

func TestWatchFile_HandlesDeletion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "delete.log")
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o644))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- watchFile(ctx, watcher, config.InputSource{Path: logPath, PathType: types.File}, make(chan *types.LogEntry, 1))
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, os.Remove(logPath))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watchFile did not stop after deletion")
	}
}

func TestWatchFile_HandlesRename(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotate.log")
	require.NoError(t, os.WriteFile(logPath, []byte(""), 0o644))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- watchFile(ctx, watcher, config.InputSource{Path: logPath, PathType: types.File}, make(chan *types.LogEntry, 1))
	}()

	time.Sleep(50 * time.Millisecond)
	rotated := logPath + ".1"
	require.NoError(t, os.Rename(logPath, rotated))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watchFile did not stop after rename")
	}
}

func TestReadNewLines_SkipsEmptyLines(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "lines.log")
	require.NoError(t, os.WriteFile(logPath, []byte("\nfirst\nsecond\n"), 0o644))

	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Seek(0, 0)
	require.NoError(t, err)

	out := make(chan *types.LogEntry, 3)
	err = readNewLines(file, logPath, nil, out)
	assert.NoError(t, err)
	close(out)

	var lines []string
	for entry := range out {
		lines = append(lines, string(entry.Line))
	}
	assert.Equal(t, []string{"first", "second"}, lines)
}

func TestReadNewLines_NonBlockingWhenChannelFull(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "drop.log")
	require.NoError(t, os.WriteFile(logPath, []byte("dropped\n"), 0o644))

	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	out := make(chan *types.LogEntry, 1)
	out <- types.NewLogEntry([]byte("existing"), logPath, nil)

	start := time.Now()
	err = readNewLines(file, logPath, nil, out)
	duration := time.Since(start)
	assert.NoError(t, err)
	assert.True(t, duration < time.Second, "readNewLines should not block when channel is full")
	assert.Equal(t, 1, len(out), "new log entry should be dropped when channel is full")
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString(line)
	require.NoError(t, err)
}

func waitForEntry(t *testing.T, ch <-chan *types.LogEntry) *types.LogEntry {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for log entry")
		return nil
	}
}
