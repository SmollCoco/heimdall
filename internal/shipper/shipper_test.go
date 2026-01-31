package shipper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupByLabels(t *testing.T) {
	t.Parallel()

	sharedLabels := map[string]string{"service": "nginx", "env": "prod"}
	differentLabels := map[string]string{"service": "api", "env": "prod"}

	tests := []struct {
		name           string
		entries        []*types.ProcessedEntry
		expectedGroups int
	}{
		{
			name:           "single entry",
			entries:        createTestProcessedEntries(1, sharedLabels, types.LogLevelInfo),
			expectedGroups: 1,
		},
		{
			name:           "multiple entries same labels",
			entries:        createTestProcessedEntries(3, sharedLabels, types.LogLevelInfo),
			expectedGroups: 1,
		},
		{
			name: "multiple entries different labels",
			entries: append(
				createTestProcessedEntries(2, sharedLabels, types.LogLevelInfo),
				createTestProcessedEntries(2, differentLabels, types.LogLevelInfo)...,
			),
			expectedGroups: 2,
		},
		{
			name: "same labels different levels",
			entries: append(
				createTestProcessedEntries(2, sharedLabels, types.LogLevelInfo),
				createTestProcessedEntries(2, sharedLabels, types.LogLevelError)...,
			),
			expectedGroups: 2,
		},
		{
			name:           "empty input",
			entries:        nil,
			expectedGroups: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			groups := groupByLabels(tt.entries)
			assert.Equal(t, tt.expectedGroups, len(groups))

			for _, groupEntries := range groups {
				if len(groupEntries) == 0 {
					continue
				}

				baseKeys := labelKeys(groupEntries[0].Labels)
				for _, entry := range groupEntries[1:] {
					assert.ElementsMatch(t, baseKeys, labelKeys(entry.Labels))
				}
			}
		})
	}
}

func TestCreateStream(t *testing.T) {
	t.Parallel()

	entries := createTestProcessedEntries(2, map[string]string{"service": "api"}, types.LogLevelInfo)
	entries[0].Timestamp = time.Unix(1700000000, 123456789)
	entries[1].Timestamp = time.Unix(1700000001, 987654321)

	stream := createStream(entries)
	require.Len(t, stream.Values, 2)
	assert.Equal(t, "info", stream.Stream["level"])
	assert.Equal(t, "api", stream.Stream["service"])

	for i, value := range stream.Values {
		assert.Len(t, value[0], 19)
		assert.Equal(t, string(entries[i].Line), value[1])
	}

	emptyStream := createStream(nil)
	assert.Empty(t, emptyStream.Stream)
	assert.Empty(t, emptyStream.Values)
}

func TestFormatLokiRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		entries         []*types.ProcessedEntry
		expectedStreams int
	}{
		{
			name:            "single entry",
			entries:         createTestProcessedEntries(1, map[string]string{"service": "test"}, types.LogLevelInfo),
			expectedStreams: 1,
		},
		{
			name:            "same label set",
			entries:         createTestProcessedEntries(5, map[string]string{"service": "test"}, types.LogLevelInfo),
			expectedStreams: 1,
		},
		{
			name: "different label sets",
			entries: append(
				createTestProcessedEntries(2, map[string]string{"service": "api"}, types.LogLevelInfo),
				createTestProcessedEntries(2, map[string]string{"service": "worker"}, types.LogLevelInfo)...,
			),
			expectedStreams: 2,
		},
		{
			name: "large batch",
			entries: append(
				append(
					createTestProcessedEntries(40, map[string]string{"service": "api"}, types.LogLevelInfo),
					createTestProcessedEntries(40, map[string]string{"service": "worker"}, types.LogLevelWarn)...,
				),
				createTestProcessedEntries(40, map[string]string{"service": "scheduler"}, types.LogLevelError)...,
			),
			expectedStreams: 3,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			req := formatLokiRequest(tt.entries)
			require.NotNil(t, req)
			assert.Equal(t, tt.expectedStreams, len(req.Streams))
			assertLokiRequestValid(t, req)

			payload, err := json.Marshal(req)
			require.NoError(t, err)

			var decoded LokiPushRequest
			require.NoError(t, json.Unmarshal(payload, &decoded))
		})
	}
}

func TestLabelsToKey(t *testing.T) {
	t.Parallel()

	labelsA := map[string]string{"service": "api", "env": "prod"}
	labelsB := map[string]string{"env": "prod", "service": "api"}

	keyA := labelsToKey(labelsA, types.LogLevelInfo)
	keyB := labelsToKey(labelsB, types.LogLevelInfo)

	assert.Equal(t, keyA, keyB)
	assert.Contains(t, keyA, "level=info")

	parts := strings.Split(keyA, ",")
	partsSorted := append([]string(nil), parts...)
	sort.Strings(partsSorted)
	assert.Equal(t, partsSorted, parts)

	keySpecial := labelsToKey(map[string]string{"path": "/var/log/nginx/access.log", "tag": "a=b,c d"}, types.LogLevelDebug)
	assert.Contains(t, keySpecial, "path=/var/log/nginx/access.log")
	assert.Contains(t, keySpecial, "tag=a=b,c d")
}

func TestLokiClient_SendRequest(t *testing.T) {
	t.Parallel()

	t.Run("successful request", func(t *testing.T) {
		var captured requestCapture
		ready := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			captured = captureRequest(r, body)
			close(ready)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 0, 0, 0)
		require.NoError(t, client.sendRequest(context.Background(), []byte(`{"streams":[]}`)))

		select {
		case <-ready:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("request not captured")
		}

		assert.Equal(t, "application/json", captured.contentType)
		assert.True(t, captured.jsonValid)
	})

	t.Run("loki errors", func(t *testing.T) {
		tests := []struct {
			name   string
			status int
		}{
			{name: "bad request", status: http.StatusBadRequest},
			{name: "server error", status: http.StatusInternalServerError},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				server := mockLokiServer(t, tt.status, 0)
				client := NewLokiClient(server.URL, 0, 0, 0)
				err := client.sendRequest(context.Background(), []byte(`{"streams":[]}`))
				require.Error(t, err)
				var le *lokiError
				require.True(t, errors.As(err, &le))
				assert.Equal(t, tt.status, le.StatusCode)
			})
		}
	})

	t.Run("network timeout", func(t *testing.T) {
		server := mockLokiServer(t, http.StatusNoContent, 200*time.Millisecond)
		client := NewLokiClient(server.URL, 0, 0, 0)
		client.httpClient.Timeout = 50 * time.Millisecond

		err := client.sendRequest(context.Background(), []byte(`{"streams":[]}`))
		require.Error(t, err)
	})

	t.Run("malformed json response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("{"))
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 0, 0, 0)
		err := client.sendRequest(context.Background(), []byte(`{"streams":[]}`))
		require.Error(t, err)
	})
}

func TestLokiClient_Push_NoRetry(t *testing.T) {
	t.Parallel()

	t.Run("success on first attempt", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 0, 0, 0)
		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "test"}, types.LogLevelInfo))
		require.NoError(t, client.Push(context.Background(), req))
		assert.Equal(t, int32(1), atomic.LoadInt32(&count))
	})

	t.Run("failed push returns error", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 0, 0, 0)
		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "test"}, types.LogLevelInfo))
		err := client.Push(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&count))
	})

	t.Run("context cancellation stops request", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 0, 0, 0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "test"}, types.LogLevelInfo))
		err := client.Push(ctx, req)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, int32(0), atomic.LoadInt32(&count))
	})
}

func TestLokiClient_Push_WithRetry(t *testing.T) {
	t.Parallel()

	t.Run("retry on 500", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := atomic.AddInt32(&count, 1)
			if attempt == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 2, 20*time.Millisecond, 50*time.Millisecond)
		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "retry"}, types.LogLevelInfo))

		start := time.Now()
		err := client.Push(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&count))
		assert.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond)
	})

	t.Run("no retry on 400", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 3, 10*time.Millisecond, 20*time.Millisecond)
		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "noretry"}, types.LogLevelInfo))

		err := client.Push(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&count))
	})

	t.Run("max retries respected", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 2, 5*time.Millisecond, 10*time.Millisecond)
		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "max"}, types.LogLevelInfo))

		err := client.Push(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, int32(3), atomic.LoadInt32(&count))
	})

	t.Run("context cancellation during retry", func(t *testing.T) {
		var count int32
		var cancel context.CancelFunc

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := atomic.AddInt32(&count, 1)
			if attempt == 1 && cancel != nil {
				cancel()
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		client := NewLokiClient(server.URL, 5, 50*time.Millisecond, 100*time.Millisecond)
		ctx, cancelFn := context.WithCancel(context.Background())
		cancel = cancelFn

		req := formatLokiRequest(createTestProcessedEntries(1, map[string]string{"service": "cancel"}, types.LogLevelInfo))
		err := client.Push(ctx, req)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, int32(1), atomic.LoadInt32(&count))
	})
}

func TestShipper_E2E_RealLoki(t *testing.T) {
	lokiURL := "http://localhost:3100"
	if !isLokiAvailable(lokiURL) {
		t.Skip("Loki not running")
	}

	client := NewLokiClient(lokiURL, 2, 50*time.Millisecond, 200*time.Millisecond)

	tests := []struct {
		name     string
		entries  []*types.ProcessedEntry
		labels   map[string]string
		minCount int
	}{
		{
			name:     "single entry",
			entries:  createTestProcessedEntries(1, map[string]string{"service": "heimdall"}, types.LogLevelInfo),
			labels:   map[string]string{"service": "heimdall"},
			minCount: 1,
		},
		{
			name:     "batch of 100 entries",
			entries:  createTestProcessedEntries(100, map[string]string{"service": "heimdall"}, types.LogLevelInfo),
			labels:   map[string]string{"service": "heimdall"},
			minCount: 100,
		},
		{
			name: "multiple label sets",
			entries: append(
				createTestProcessedEntries(5, map[string]string{"service": "api"}, types.LogLevelInfo),
				createTestProcessedEntries(5, map[string]string{"service": "worker"}, types.LogLevelInfo)...,
			),
			labels:   map[string]string{"service": "api"},
			minCount: 5,
		},
		{
			name: "all log levels",
			entries: append(
				append(
					createTestProcessedEntries(2, map[string]string{"service": "levels"}, types.LogLevelError),
					createTestProcessedEntries(2, map[string]string{"service": "levels"}, types.LogLevelWarn)...,
				),
				append(
					createTestProcessedEntries(2, map[string]string{"service": "levels"}, types.LogLevelInfo),
					createTestProcessedEntries(2, map[string]string{"service": "levels"}, types.LogLevelDebug)...,
				)...,
			),
			labels:   map[string]string{"service": "levels"},
			minCount: 8,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			testID := strconv.FormatInt(time.Now().UnixNano(), 10)
			labels := make(map[string]string, len(tt.labels)+1)
			for k, v := range tt.labels {
				labels[k] = v
			}
			labels["test_id"] = testID

			entries := cloneEntriesWithLabels(tt.entries, labels)
			stampEntriesNow(entries)

			req := formatLokiRequest(entries)
			require.NoError(t, client.Push(context.Background(), req))

			query := fmt.Sprintf("{test_id=\"%s\"}", testID)
			require.NoError(t, waitForLokiCount(t, lokiURL, query, tt.minCount, 5*time.Second))
		})
	}
}

// fake pusher for shipper tests
type fakePusher struct {
	calls chan int // sends "how many values were pushed" each time
}

func newFakePusher(buf int) *fakePusher {
	return &fakePusher{calls: make(chan int, buf)}
}

func (f *fakePusher) Push(ctx context.Context, req *LokiPushRequest) error {
	total := 0
	for _, s := range req.Streams {
		total += len(s.Values)
	}
	f.calls <- total
	return nil
}

func TestShipper_BatchingBySize(t *testing.T) {
	t.Parallel()

	fp := newFakePusher(10)
	s := NewShipperWithClient(fp, 3, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan *types.ProcessedEntry, 10)

	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, in) }()

	// send 3 entries (same labels)
	labels := map[string]string{"service": "batch"}
	entries := createTestProcessedEntries(3, labels, types.LogLevelInfo)
	for _, e := range entries {
		in <- e
	}

	// wait for exactly one push of 3
	select {
	case got := <-fp.calls:
		require.Equal(t, 3, got)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected push did not happen")
	}

	// stop shipper cleanly
	cancel()
	close(in)
	require.NoError(t, <-done)
}

func TestShipper_BatchingByTimeout(t *testing.T) {
	t.Parallel()

	fp := newFakePusher(10)
	s := NewShipperWithClient(fp, 100, 50*time.Millisecond)

	ctx := context.Background()
	in := make(chan *types.ProcessedEntry)

	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, in) }()

	labels := map[string]string{"service": "batch"}
	entries := createTestProcessedEntries(5, labels, types.LogLevelInfo)
	for _, e := range entries {
		in <- e
	}

	// expect one push of 5 (timeout flush)
	select {
	case got := <-fp.calls:
		require.Equal(t, 5, got)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected push did not happen")
	}

	// ensure no second push (optional strictness)
	select {
	case extra := <-fp.calls:
		t.Fatalf("unexpected extra push: %d", extra)
	case <-time.After(100 * time.Millisecond):
		// ok
	}

	close(in)

	// wait for shipper to exit
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("shipper did not exit")
	}
}


func TestShipper_BatchingAfterShutdown(t *testing.T) {
	t.Parallel()

	fp := newFakePusher(10)

	// Make size/timeout NOT trigger; we want flush ONLY on shutdown.
	s := NewShipperWithClient(fp, 10, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered channel so sends only complete when shipper receives them.
	in := make(chan *types.ProcessedEntry)

	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, in) }()

	// Send 4 entries (same labels)
	labels := map[string]string{"service": "shutdown"}
	entries := createTestProcessedEntries(4, labels, types.LogLevelInfo)

	for _, e := range entries {
		in <- e
	}

	// Now trigger shutdown flush
	cancel()

	// Expect one push of 4
	select {
	case got := <-fp.calls:
		require.Equal(t, 4, got)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected push did not happen")
	}

	// Stop shipper goroutine cleanly
	close(in)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("shipper did not exit")
	}

	// Ensure no extra pushes
	select {
	case extra := <-fp.calls:
		t.Fatalf("unexpected extra push: %d", extra)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func BenchmarkFormatLokiRequest(b *testing.B) {
	entries10 := createTestProcessedEntries(10, map[string]string{"service": "bench"}, types.LogLevelInfo)
	entries100 := createTestProcessedEntries(100, map[string]string{"service": "bench"}, types.LogLevelInfo)
	entries1000 := createTestProcessedEntries(1000, map[string]string{"service": "bench"}, types.LogLevelInfo)

	b.Run("10", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = formatLokiRequest(entries10)
		}
	})

	b.Run("100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = formatLokiRequest(entries100)
		}
	})

	b.Run("1000", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = formatLokiRequest(entries1000)
		}
	})
}

func BenchmarkLokiClient_Push(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewLokiClient(server.URL, 0, 0, 0)
	request := formatLokiRequest(createTestProcessedEntries(100, map[string]string{"service": "bench"}, types.LogLevelInfo))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.Push(context.Background(), request)
	}
}

type requestCapture struct {
	contentType string
	body        []byte
	jsonValid   bool
}

func captureRequest(r *http.Request, body []byte) requestCapture {
	var payload map[string]interface{}
	jsonValid := json.Unmarshal(body, &payload) == nil

	return requestCapture{
		contentType: r.Header.Get("Content-Type"),
		body:        body,
		jsonValid:   jsonValid,
	}
}

// createTestProcessedEntries creates test entries with specified labels
func createTestProcessedEntries(count int, labels map[string]string, level types.LogLevel) []*types.ProcessedEntry {
	entries := make([]*types.ProcessedEntry, 0, count)
	for i := 0; i < count; i++ {
		entry := types.NewLogEntry([]byte(fmt.Sprintf("line-%d", i)), "/var/log/test.log", labels)
		entry.Timestamp = time.Unix(1700000000, int64(i))
		processed := types.NewProcessedEntry(entry)
		processed.Level = level
		entries = append(entries, processed)
	}
	return entries
}

// assertLokiRequestValid validates LokiPushRequest structure
func assertLokiRequestValid(t *testing.T, req *LokiPushRequest) {
	t.Helper()
	require.NotNil(t, req)

	for _, stream := range req.Streams {
		require.NotNil(t, stream.Stream)
		require.NotNil(t, stream.Values)
		for _, value := range stream.Values {
			require.Len(t, value, 2)
			require.NotEmpty(t, value[0])
		}
	}
}

// mockLokiServer creates httptest server simulating Loki
func mockLokiServer(t *testing.T, responseCode int, responseDelay time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if responseDelay > 0 {
			time.Sleep(responseDelay)
		}
		w.WriteHeader(responseCode)
	}))
	t.Cleanup(server.Close)
	return server
}

// queryLoki queries real Loki to verify logs were received
func queryLoki(t *testing.T, lokiURL string, query string) (int, error) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}

	u, err := url.Parse(lokiURL + "/loki/api/v1/query_range")
	if err != nil {
		return 0, err
	}

	params := u.Query()
	params.Set("query", query)
	params.Set("limit", "1000")
	params.Set("direction", "forward")
	params.Set("start", strconv.FormatInt(time.Now().Add(-1*time.Minute).UnixNano(), 10))
	params.Set("end", strconv.FormatInt(time.Now().Add(1*time.Minute).UnixNano(), 10))
	u.RawQuery = params.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("query failed: %s", string(body))
	}

	var payload lokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}

	count := 0
	for _, result := range payload.Data.Result {
		count += len(result.Values)
	}

	return count, nil
}

func waitForLokiCount(t *testing.T, lokiURL string, query string, minCount int, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count, err := queryLoki(t, lokiURL, query)
		if err == nil && count >= minCount {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("logs not found for query %s", query)
}

func isLokiAvailable(lokiURL string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(lokiURL + "/ready")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func labelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	return keys
}

func copyLabels(labels map[string]string) map[string]string {
	clone := make(map[string]string, len(labels))
	for k, v := range labels {
		clone[k] = v
	}
	return clone
}

func cloneEntriesWithLabels(entries []*types.ProcessedEntry, labels map[string]string) []*types.ProcessedEntry {
	cloned := make([]*types.ProcessedEntry, 0, len(entries))
	for _, entry := range entries {
		logEntry := types.NewLogEntry(entry.Line, entry.Source, labels)
		logEntry.Timestamp = entry.Timestamp
		processed := types.NewProcessedEntry(logEntry)
		processed.Level = entry.Level
		cloned = append(cloned, processed)
	}
	return cloned
}

func stampEntriesNow(entries []*types.ProcessedEntry) {
	base := time.Now().Add(-200 * time.Millisecond)
	for i, entry := range entries {
		entry.Timestamp = base.Add(time.Duration(i) * time.Millisecond)
	}
}

type lokiQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []lokiQueryResult `json:"result"`
	} `json:"data"`
}

type lokiQueryResult struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}
