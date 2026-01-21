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

func TestWatchDirectory_WatchesExistingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "existing.log")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 2)
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchDirectory(ctx, watcher, config.InputSource{Path: dir, PathType: types.Directory, Labels: map[string]string{"env": "dev"}}, out)
	}()

	time.Sleep(100 * time.Millisecond)
	appendLine(t, filePath, "first\n")

	entry := waitForEntry(t, out)
	assert.Equal(t, "first", string(entry.Line))
	assert.Equal(t, "dev", entry.Labels["env"])

	cancel()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("watchDirectory did not exit after cancellation")
	}
}

func TestWatchDirectory_DetectsNewFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 2)
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchDirectory(ctx, watcher, config.InputSource{Path: dir, PathType: types.Directory, Labels: map[string]string{"source": "new"}}, out)
	}()

	time.Sleep(50 * time.Millisecond)

	newFile := filepath.Join(dir, "new.log")
	require.NoError(t, os.WriteFile(newFile, []byte(""), 0o644))
	appendLine(t, newFile, "hello\n")

	entry := waitForEntry(t, out)
	assert.Equal(t, "hello", string(entry.Line))
	assert.Equal(t, "new", entry.Labels["source"])

	cancel()
	<-errCh
}

func TestWatchDirectory_IgnoresSpecialFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cases := []string{".hidden", "swap.swp", "temp.tmp", "backup~"}

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchDirectory(ctx, watcher, config.InputSource{Path: dir, PathType: types.Directory}, out)
	}()

	time.Sleep(50 * time.Millisecond)

	for _, name := range cases {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("ignored\n"), 0o644))
	}

	select {
	case <-out:
		t.Fatal("expected no log entries for ignored files")
	case <-time.After(300 * time.Millisecond):
		// No events received as expected
	}

	cancel()
	<-errCh
}

func TestWatchDirectory_IgnoresSubdirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested")
	require.NoError(t, os.Mkdir(subdir, 0o755))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- watchDirectory(ctx, watcher, config.InputSource{Path: dir, PathType: types.Directory}, out)
	}()

	time.Sleep(50 * time.Millisecond)

	nestedFile := filepath.Join(subdir, "file.log")
	require.NoError(t, os.WriteFile(nestedFile, []byte("line\n"), 0o644))

	select {
	case <-out:
		t.Fatal("expected no entries from subdirectories")
	case <-time.After(300 * time.Millisecond):
	}

	cancel()
	<-errCh
}

func TestSpawnFileWatcher_Deduplicates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "dedupe.log")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	active := make(map[string]context.CancelFunc)
	out := make(chan *types.LogEntry, 1)

	spawnFileWatcher(ctx, watcher, filePath, nil, out, active)
	spawnFileWatcher(ctx, watcher, filePath, nil, out, active)

	assert.Equal(t, 1, len(active))
	cancel()
	time.Sleep(50 * time.Millisecond)
}
