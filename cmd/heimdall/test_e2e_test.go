// cmd/heimdall/test_e2e_test.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/processor"
	"github.com/SmollCoco/heimdall/internal/shipper"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/SmollCoco/heimdall/internal/watcher"
	"github.com/stretchr/testify/require"
)

func TestHeimdall_E2E_Pipeline_ToLoki(t *testing.T) {
	// This is an integration test: it expects Loki running on localhost:3100.
	// If Loki isn't up, we skip (so CI/dev doesn't fail unexpectedly).

	lokiBase := "http://localhost:3100"
	requireLokiReadyOrSkip(t, lokiBase)

	// Create test file in a temp dir
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "app.log")
	require.NoError(t, os.WriteFile(testFile, []byte(""), 0o644))

	// Configure pipeline
	inputs := []config.InputSource{
		{
			Path:     testFile,
			PathType: types.File,
			Labels: map[string]string{
				"service":     "e2e-test",
				"environment": "test",
			},
		},
	}

	// Channels
	watcherCh := make(chan *types.LogEntry, 100)
	processorCh := make(chan *types.ProcessedEntry, 100)

	// Context: bounded so the test can never hang forever
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collect async errors
	errCh := make(chan error, 3)

	// Start watcher
	go func() {
		defer close(watcherCh)
		errCh <- watcher.Start(ctx, inputs, watcherCh)
	}()

	// Start processor
	go func() {
		defer close(processorCh)
		errCh <- processor.Start(ctx, watcherCh, processorCh, 2)
	}()

	// Start shipper
	lokiShipper := shipper.NewShipper(
		lokiBase,
		10,             // batch_size (entries)
		2*time.Second,  // batch_timeout
		5,              // max_retries
		1*time.Second,  // initial_backoff
		30*time.Second, // max_backoff
	)

	go func() {
		errCh <- lokiShipper.Start(ctx, processorCh)
	}()

	// Give pipeline a moment to initialize
	time.Sleep(300 * time.Millisecond)

	// Write test logs
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)

	testLogs := []string{
		"INFO: Application started successfully",
		"DEBUG: Loading configuration",
		"WARN: Connection pool running low",
		"ERROR: Failed to connect to database",
		"FATAL: Out of memory",
		"Regular log without level",
	}
	for i, line := range testLogs {
		_, werr := fmt.Fprintf(f, "[%d] %s\n", i+1, line)
		require.NoError(t, werr)
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, f.Close())

	// Verify logs arrive in Loki (eventually)
	query := `{service="e2e-test"}`
	require.Eventually(t, func() bool {
		n, qerr := lokiCountResults(ctx, lokiBase, query, 2*time.Minute)
		if qerr != nil {
			return false
		}
		return n > 0
	}, 15*time.Second, 300*time.Millisecond, "expected Loki query to return results for %s", query)

	// Shutdown
	cancel()

	// Drain async errors (we allow context cancellation)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()

	for i := 0; i < 3; i++ {
		select {
		case e := <-errCh:
			if e == nil {
				continue
			}
			// Treat context cancellation as clean shutdown
			if ctx.Err() != nil {
				continue
			}
			require.NoError(t, e)
		case <-deadline.C:
			// If goroutines haven't reported, that's ok; context timeout still protects test.
			return
		}
	}
}

func requireLokiReadyOrSkip(t *testing.T, baseURL string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/ready", nil)
	if err != nil {
		t.Skipf("skip: cannot build request to Loki /ready: %v", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("skip: Loki not reachable on %s: %v", baseURL, err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Skipf("skip: Loki /ready returned %d", resp.StatusCode)
		return
	}
}

type lokiQueryResp struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Values [][]string `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func lokiCountResults(ctx context.Context, baseURL, query string, lookback time.Duration) (int, error) {
	u, err := url.Parse(baseURL + "/loki/api/v1/query_range")
	if err != nil {
		return 0, err
	}

	end := time.Now()
	start := end.Add(-lookback)

	q := u.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	q.Set("end", fmt.Sprintf("%d", end.UnixNano()))
	q.Set("limit", "1000")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("loki query_range status=%d body=%s", resp.StatusCode, string(b))
	}

	var out lokiQueryResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}

	total := 0
	for _, r := range out.Data.Result {
		total += len(r.Values)
	}
	return total, nil
}
