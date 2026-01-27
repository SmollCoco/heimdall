// worker.go
package processor

import (
	"context"
	"log"

	"github.com/SmollCoco/heimdall/internal/types"
)

// processWorker is the worker goroutine that processes log entries
func processWorker(ctx context.Context, workerID int, input <-chan *types.LogEntry, output chan<- *types.ProcessedEntry) {
	log.Printf("Processor worker %d started", workerID)

	for {
		select {
		case entry, ok := <-input:
			if !ok {
				// Channel closed, exit
				log.Printf("Processor worker %d: input channel closed", workerID)
				return
			}

			// Process the entry
			processed := processEntry(entry)

			// Send to output
			select {
			case output <- processed:
				// Sent successfully
			case <-ctx.Done():
				// Context cancelled
				return
			}

		case <-ctx.Done():
			// Context cancelled
			log.Printf("Processor worker %d: context cancelled", workerID)
			return
		}
	}
}

// processEntry transforms a LogEntry into a ProcessedEntry
func processEntry(entry *types.LogEntry) *types.ProcessedEntry {
	log.Printf("DEBUG [processEntry]: Input LogEntry labels: %+v", entry.Labels)

	processed := types.NewProcessedEntry(entry)

	log.Printf("DEBUG [processEntry]: ProcessedEntry labels after creation: %+v", processed.Labels)

	// Detect log level
	level := detectLogLevel(entry.Line)
	processed.Level = level

	// For MVP, we don't do any complex parsing
	// Just pass through with detected level

	return processed
}
