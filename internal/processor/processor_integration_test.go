package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/SmollCoco/heimdall/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullPipeline_FileWatching(t *testing.T) {
	t.Parallel()

	logPath, cleanup := createTempLogFile(t)
	t.Cleanup(cleanup)

	labels := map[string]string{"service": "heimdall", "env": "test"}

	watcherChan := make(chan *types.LogEntry, 128)
	processorChan := make(chan *types.ProcessedEntry, 128)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Start(ctx, []config.InputSource{{
			Path:     logPath,
			PathType: types.File,
			Labels:   labels,
		}}, watcherChan)
	}()

	processorDone := make(chan error, 1)
	go func() {
		processorDone <- Start(ctx, watcherChan, processorChan, 2)
	}()

	t.Cleanup(func() {
		cancel()
		watcherStopped := false
		select {
		case err := <-watcherDone:
			assert.NoError(t, err, "watcher returned an error")
			watcherStopped = true
		case <-time.After(time.Second):
			t.Error("watcher did not stop after cancellation")
		}
		if watcherStopped {
			close(watcherChan)
		}

		processorStopped := false
		select {
		case err := <-processorDone:
			assert.NoError(t, err, "processor returned an error")
			processorStopped = true
		case <-time.After(time.Second):
			t.Error("processor did not stop after cancellation")
		}
		if processorStopped {
			close(processorChan)
		}
	})

	time.Sleep(100 * time.Millisecond)

	logLines := []string{
		"INFO: Application started",
		"DEBUG: Configuration loaded",
		"WARN: Memory usage high",
		"ERROR: Database connection failed",
		"FATAL: System crash",
		"This is a log with no level",
	}

	for _, line := range logLines {
		writeLogLine(t, logPath, line)
	}

	processed := collectProcessedEntries(ctx, processorChan, len(logLines), 2*time.Second)
	require.Len(t, processed, len(logLines), "expected all log lines to be processed")

	expectedLevels := map[string]types.LogLevel{
		"INFO: Application started":         types.LogLevelInfo,
		"DEBUG: Configuration loaded":       types.LogLevelDebug,
		"WARN: Memory usage high":           types.LogLevelWarn,
		"ERROR: Database connection failed": types.LogLevelError,
		"FATAL: System crash":               types.LogLevelError,
		"This is a log with no level":       types.LogLevelUnknown,
	}

	for _, entry := range processed {
		line := string(entry.Line)
		expectedLevel, ok := expectedLevels[line]
		require.True(t, ok, "unexpected log line received: %q", line)
		assert.Equal(t, expectedLevel, entry.Level, "log level mismatch for line: %q", line)
		assert.Equal(t, logPath, entry.Source, "source path should match temp file")
		assert.Equal(t, labels, entry.Labels, "labels should be propagated")
		assert.True(t, entry.ParseSuccess, "parse success should be true")
		assert.False(t, entry.ProcessedAt.IsZero(), "processed timestamp should be set")
	}
}

func TestFullPipeline_DirectoryWatching(t *testing.T) {
	t.Parallel()

	dirPath, cleanup := createTempDirectory(t)
	t.Cleanup(cleanup)

	fileOne := filepath.Join(dirPath, "app1.log")
	fileTwo := filepath.Join(dirPath, "app2.log")
	require.NoError(t, os.WriteFile(fileOne, []byte(""), 0o644))
	require.NoError(t, os.WriteFile(fileTwo, []byte(""), 0o644))

	labels := map[string]string{"service": "dir", "env": "test"}

	watcherChan := make(chan *types.LogEntry, 256)
	processorChan := make(chan *types.ProcessedEntry, 256)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Start(ctx, []config.InputSource{{
			Path:     dirPath,
			PathType: types.Directory,
			Labels:   labels,
		}}, watcherChan)
	}()

	processorDone := make(chan error, 1)
	go func() {
		processorDone <- Start(ctx, watcherChan, processorChan, 2)
	}()

	t.Cleanup(func() {
		cancel()
		watcherStopped := false
		select {
		case err := <-watcherDone:
			assert.NoError(t, err, "watcher returned an error")
			watcherStopped = true
		case <-time.After(time.Second):
			t.Error("watcher did not stop after cancellation")
		}
		if watcherStopped {
			close(watcherChan)
		}

		processorStopped := false
		select {
		case err := <-processorDone:
			assert.NoError(t, err, "processor returned an error")
			processorStopped = true
		case <-time.After(time.Second):
			t.Error("processor did not stop after cancellation")
		}
		if processorStopped {
			close(processorChan)
		}
	})

	time.Sleep(150 * time.Millisecond)

	newFile := filepath.Join(dirPath, "new.log")
	require.NoError(t, os.WriteFile(newFile, []byte(""), 0o644))

	ignoredFile := filepath.Join(dirPath, "ignored.swp")
	require.NoError(t, os.WriteFile(ignoredFile, []byte(""), 0o644))
	writeLogLine(t, ignoredFile, "ignored line")

	expectedLines := map[string]string{
		fileOne: "file-one line",
		fileTwo: "file-two line",
		newFile: "new file line",
	}

	for path, line := range expectedLines {
		writeLogLine(t, path, line)
	}

	expectedSources := map[string]bool{
		normalizeTmpPath(fileOne): true,
		normalizeTmpPath(fileTwo): true,
		normalizeTmpPath(newFile): true,
	}

	seen := map[string]bool{}
	processed := []*types.ProcessedEntry{}

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()

	retryTicker := time.NewTicker(150 * time.Millisecond)
	defer retryTicker.Stop()

loop:
	for len(seen) < len(expectedSources) {
		select {
		case <-ctx.Done():
			break loop
		case <-deadline.C:
			break loop
		case <-retryTicker.C:
			for path, line := range expectedLines {
				if !seen[normalizeTmpPath(path)] {
					writeLogLine(t, path, line)
				}
			}
		case entry := <-processorChan:
			if entry == nil {
				continue
			}
			processed = append(processed, entry)

			src := normalizeTmpPath(entry.Source)
			if src == normalizeTmpPath(ignoredFile) {
				t.Fatalf("ignored .swp file should not be processed: %s", entry.Source)
			}
			if expectedSources[src] {
				seen[src] = true
			}
		}
	}

	require.Len(t, seen, len(expectedSources), "expected logs from all valid files")

	for _, entry := range processed {
		src := normalizeTmpPath(entry.Source)
		if expectedSources[src] {
			assert.Equal(t, labels, entry.Labels, "labels should be propagated")
		}
	}
}

func TestLogLevelDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		logLine       string
		expectedLevel types.LogLevel
	}{
		{"ERROR: Something failed", types.LogLevelError},
		{"Fatal error occurred", types.LogLevelError},
		{"WARN: This is a warning", types.LogLevelWarn},
		{"Warning: Low disk space", types.LogLevelWarn},
		{"INFO: Request completed", types.LogLevelInfo},
		{"Information message", types.LogLevelInfo},
		{"DEBUG: Variable x = 5", types.LogLevelDebug},
		{"No level indicator here", types.LogLevelUnknown},
		{"", types.LogLevelUnknown},
	}

	input := make(chan *types.LogEntry, len(testCases))
	output := make(chan *types.ProcessedEntry, len(testCases))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)

	processorDone := make(chan error, 1)
	go func() {
		processorDone <- Start(ctx, input, output, 1)
	}()

	for _, tc := range testCases {
		input <- types.NewLogEntry([]byte(tc.logLine), "unit-test", map[string]string{"case": "loglevel"})
	}
	close(input)

	processed := collectProcessedEntries(ctx, output, len(testCases), 2*time.Second)
	require.Len(t, processed, len(testCases), "expected all log level cases to be processed")

	for i, entry := range processed {
		expected := testCases[i].expectedLevel
		assert.Equal(t, expected, entry.Level, "log level mismatch for line %q", testCases[i].logLine)
		assert.True(t, entry.ParseSuccess, "parse success should be true")
		assert.False(t, entry.ProcessedAt.IsZero(), "processed timestamp should be set")
	}

	select {
	case err := <-processorDone:
		assert.NoError(t, err, "processor returned an error")
	case <-time.After(time.Second):
		t.Error("processor did not stop after input closed")
	}
	close(output)
}

func TestFullPipeline_HighVolume(t *testing.T) {
	logPath, cleanup := createTempLogFile(t)
	t.Cleanup(cleanup)

	watcherChan := make(chan *types.LogEntry, 1024)
	processorChan := make(chan *types.ProcessedEntry, 1024)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Start(ctx, []config.InputSource{{
			Path:     logPath,
			PathType: types.File,
			Labels:   map[string]string{"volume": "high"},
		}}, watcherChan)
	}()

	processorDone := make(chan error, 1)
	go func() {
		processorDone <- Start(ctx, watcherChan, processorChan, 4)
	}()

	t.Cleanup(func() {
		cancel()
		watcherStopped := false
		select {
		case err := <-watcherDone:
			assert.NoError(t, err, "watcher returned an error")
			watcherStopped = true
		case <-time.After(time.Second):
			t.Error("watcher did not stop after cancellation")
		}
		if watcherStopped {
			close(watcherChan)
		}

		processorStopped := false
		select {
		case err := <-processorDone:
			assert.NoError(t, err, "processor returned an error")
			processorStopped = true
		case <-time.After(time.Second):
			t.Error("processor did not stop after cancellation")
		}
		if processorStopped {
			close(processorChan)
		}
	})

	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	for i := 0; i < 1000; i++ {
		var line string
		switch i % 5 {
		case 0:
			line = fmt.Sprintf("INFO: line %d", i)
		case 1:
			line = fmt.Sprintf("DEBUG: line %d", i)
		case 2:
			line = fmt.Sprintf("WARN: line %d", i)
		case 3:
			line = fmt.Sprintf("ERROR: line %d", i)
		default:
			line = fmt.Sprintf("Plain line %d", i)
		}
		writeLogLine(t, logPath, line)
	}

	processed := collectProcessedEntries(ctx, processorChan, 1000, 5*time.Second)
	require.Len(t, processed, 1000, "expected all high-volume logs to be processed")

	duration := time.Since(start)
	assert.Less(t, duration, 5*time.Second, "high-volume processing exceeded time limit")
}

func createTempLogFile(t *testing.T) (string, func()) {
	t.Helper()

	file, err := os.CreateTemp("/tmp", "heimdall_test_pipeline_*.log")
	require.NoError(t, err, "failed to create temp log file")
	require.NoError(t, file.Close(), "failed to close temp log file")

	cleanup := func() {
		_ = os.Remove(file.Name())
	}

	return file.Name(), cleanup
}

func createTempDirectory(t *testing.T) (string, func()) {
	t.Helper()

	baseDir := "./tmp"
	require.NoError(t, os.MkdirAll(baseDir, 0o755), "failed to create temp base directory")

	dir, err := os.MkdirTemp(baseDir, "heimdall_test_dir_*")
	require.NoError(t, err, "failed to create temp directory")

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	return dir, cleanup
}

func writeLogLine(t *testing.T, path string, line string) {
	t.Helper()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err, "failed to open log file for append")
	defer file.Close()

	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	_, err = file.WriteString(line)
	require.NoError(t, err, "failed to write log line")
}

func collectProcessedEntries(ctx context.Context, ch <-chan *types.ProcessedEntry, count int, timeout time.Duration) []*types.ProcessedEntry {
	entries := make([]*types.ProcessedEntry, 0, count)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(entries) < count {
		select {
		case <-ctx.Done():
			return entries
		case <-timer.C:
			return entries
		case entry, ok := <-ch:
			if !ok {
				return entries
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

func normalizeTmpPath(path string) string {
	baseAbs, err := filepath.Abs("./tmp")
	if err != nil {
		return filepath.Clean(path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	if strings.HasPrefix(abs, baseAbs) {
		rel, relErr := filepath.Rel(baseAbs, abs)
		if relErr == nil {
			return filepath.Clean(filepath.Join("./tmp", rel))
		}
	}

	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, "tmp/") {
		return filepath.Clean("./" + cleaned)
	}

	return cleaned
}
