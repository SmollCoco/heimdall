// processor.go
package processor

import (
	"context"
	"log"
	"sync"

	"github.com/SmollCoco/heimdall/internal/types"
)

// Start initializes the processor worker pool
func Start(ctx context.Context, input <-chan *types.LogEntry, output chan<- *types.ProcessedEntry, numWorkers int) error {
	log.Printf("Starting processor with %d workers", numWorkers)

	var wg sync.WaitGroup

	// Spawn worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processWorker(ctx, workerID, input, output)
		}(i)
	}

	// Wait for all workers to finish
	wg.Wait()
	log.Println("All processor workers stopped")
	return nil
}
