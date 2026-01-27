package shipper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// LokiClient sends logs to Loki with retry logic
type LokiClient struct {
	url            string
	httpClient     *http.Client
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// NewLokiClient creates a new Loki client
func NewLokiClient(url string, maxRetries int, initialBackoff, maxBackoff time.Duration) *LokiClient {
	return &LokiClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
}

// Push sends a batch of entries to Loki with retry logic
func (c *LokiClient) Push(ctx context.Context, request *LokiPushRequest) error {
	// Short-circuit if no streams
	if len(request.Streams) == 0 {
		return nil
	}

	for i, stream := range request.Streams {
		log.Printf("DEBUG [LokiClient.Push]: stream[%d] labels=%+v entries=%d", i, stream.Stream, len(stream.Values))
	}

	// Marshal to JSON
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry loop with exponential backoff
	backoff := c.initialBackoff
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Try to send
		err := c.sendRequest(ctx, body)

		if err == nil {
			// Success!
			return nil
		}
		lastErr = err

		// Don't retry after the last attempt
		if attempt == c.maxRetries {
			break
		}

		// Check if error is retryable
		if !isRetryable(err) {
			// Permanent error (4xx), don't retry
			return fmt.Errorf("permanent error, not retrying: %w", err)
		}

		// Log retry attempt
		log.Printf("Loki push failed (attempt %d/%d): %v. Retrying in %s...",
			attempt+1, c.maxRetries+1, err, backoff)

		// Wait with exponential backoff
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}

		// Double the backoff for next attempt
		backoff *= 2
		if backoff > c.maxBackoff {
			backoff = c.maxBackoff
		}
	}

	// All retries exhausted
	return fmt.Errorf("failed to push to Loki after %d attempts: %w", c.maxRetries+1, lastErr)
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	// Check if it's a lokiError
	var lokiErr *lokiError
	if errors.As(err, &lokiErr) {
		// Retry on 5xx (server errors), not on 4xx (client errors)
		return lokiErr.StatusCode >= 500
	}

	// Retry on network errors (timeout, connection refused, etc.)
	return true
}

// LokiError represents an error response from Loki
type lokiError struct {
	StatusCode int
	Body       string
}

func (e *lokiError) Error() string {
	return fmt.Sprintf("loki returned %d: %s", e.StatusCode, e.Body)
}

func shouldRetry(err error) bool {
	var le *lokiError
	if errors.As(err, &le) {
		return le.StatusCode >= http.StatusInternalServerError
	}

	return true
}

func (c *LokiClient) sendRequest(ctx context.Context, body []byte) error {
	// Create request
	fullURL := c.url + "/loki/api/v1/push"
	log.Printf("DEBUG [LokiClient.sendRequest]: url=%s", fullURL)
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode == 204 {
		// Success!
		return nil
	}

	// Read error body
	respBody, _ := io.ReadAll(resp.Body)

	// Return structured error
	return &lokiError{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
	}
}
