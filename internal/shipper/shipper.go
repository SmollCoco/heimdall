package shipper

import (
	"context"
	"log"
	"time"

	"github.com/SmollCoco/heimdall/internal/types"
)

// Pusher defines the interface for pushing logs to Loki
type Pusher interface {
	Push(ctx context.Context, request *LokiPushRequest) error
}

// Shipper batches and sends ProcessedEntries to Loki
type Shipper struct {
	client       Pusher
	batchSize    int
	batchTimeout time.Duration
}

func NewShipper(lokiURL string, batchSize int, batchTimeout time.Duration, maxRetries int, initialBackoff, maxBackoff time.Duration) *Shipper {
	return &Shipper{
		client:       NewLokiClient(lokiURL, maxRetries, initialBackoff, maxBackoff), // *LokiClient implements Push
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
	}
}

func NewShipperWithClient(client Pusher, batchSize int, batchTimeout time.Duration) *Shipper {
	return &Shipper{
		client:       client,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
	}
}

// Start begins consuming ProcessedEntries and sending them to Loki
func (s *Shipper) Start(ctx context.Context, input <-chan *types.ProcessedEntry) error {
	log.Printf("Starting shipper (batch_size=%d, batch_timeout=%s)", s.batchSize, s.batchTimeout)

	batch := make([]*types.ProcessedEntry, 0, s.batchSize)
	ticker := time.NewTicker(s.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-input:
			if !ok {
				// Channel closed, send remaining batch
				if len(batch) > 0 {
					s.sendBatch(ctx, batch)
				}
				log.Println("Shipper input channel closed, exiting")
				return nil
			}

			// Add to batch
			batch = append(batch, entry)

			// Send if batch is full
			if len(batch) >= s.batchSize {
				s.sendBatch(ctx, batch)
				batch = make([]*types.ProcessedEntry, 0, s.batchSize)
				ticker.Reset(s.batchTimeout) // Reset timer
			}

		case <-ticker.C:
			// Timeout reached, send current batch
			if len(batch) > 0 {
				s.sendBatch(ctx, batch)
				batch = make([]*types.ProcessedEntry, 0, s.batchSize)
			}

		case <-ctx.Done():
			// Context cancelled, send remaining batch
			if len(batch) > 0 {
				s.sendBatch(ctx, batch)
			}
			log.Println("Shipper context cancelled, exiting")
			return nil
		}
	}
}

// sendBatch formats and sends a batch to Loki
func (s *Shipper) sendBatch(ctx context.Context, batch []*types.ProcessedEntry) {
	if len(batch) == 0 {
		return
	}

	log.Printf("Sending batch of %d entries to Loki", len(batch))

	// Format request
	request := formatLokiRequest(batch)

	// Send to Loki
	if err := s.client.Push(ctx, request); err != nil {
		log.Printf("Failed to send batch to Loki: %v", err)
		// TODO Phase 3: Write to local buffer file on failure
	} else {
		log.Printf("Successfully sent %d entries to Loki", len(batch))
	}
}
