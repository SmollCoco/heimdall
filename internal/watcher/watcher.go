package watcher

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/fsnotify/fsnotify"
)

// Start initializes watchers for all configured inputs
func Start(ctx context.Context, inputs []config.InputSource, output chan<- *types.LogEntry) error {
	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	// WaitGroup to track all watcher goroutines
	var wg sync.WaitGroup

	// Start a watcher for each input
	for _, input := range inputs {
		wg.Add(1)

		switch input.PathType {
		case types.File:
			go func(inp config.InputSource) {
				defer wg.Done()
				if err := watchFile(ctx, watcher, inp, output); err != nil {
					log.Printf("Error watching file %s: %v", inp.Path, err)
				}
			}(input)

		case types.Directory:
			go func(inp config.InputSource) {
				defer wg.Done()
				if err := watchDirectory(ctx, watcher, inp, output); err != nil {
					log.Printf("Error watching directory %s: %v", inp.Path, err)
				}
			}(input)
		}
	}

	// Wait for context cancellation or all watchers to finish
	wg.Wait()
	return nil
}
