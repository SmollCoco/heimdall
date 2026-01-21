package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStart_MultipleInputs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.log")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *types.LogEntry, 4)
	errCh := make(chan error, 1)

	inputs := []config.InputSource{
		{Path: filePath, PathType: types.File, Labels: map[string]string{"kind": "file"}},
		{Path: dir, PathType: types.Directory, Labels: map[string]string{"kind": "dir"}},
	}

	go func() {
		errCh <- Start(ctx, inputs, out)
	}()

	time.Sleep(100 * time.Millisecond)

	appendLine(t, filePath, "from-file\n")

	dirFile := filepath.Join(dir, "dir.log")
	require.NoError(t, os.WriteFile(dirFile, []byte(""), 0o644))
	appendLine(t, dirFile, "from-dir\n")

	got := []string{string(waitForEntry(t, out).Line), string(waitForEntry(t, out).Line)}
	assert.ElementsMatch(t, []string{"from-file", "from-dir"}, got)

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not exit after cancellation")
	}
}

func TestStart_ContextCancellationStopsWatchers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "stop.log")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan *types.LogEntry, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- Start(ctx, []config.InputSource{{Path: filePath, PathType: types.File}}, out)
	}()

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start did not exit after context cancellation")
	}
}

func TestStart_InvalidPathDoesNotPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	out := make(chan *types.LogEntry, 1)
	err := Start(ctx, []config.InputSource{{Path: "/nonexistent/path.log", PathType: types.File}}, out)
	assert.NoError(t, err)
}
