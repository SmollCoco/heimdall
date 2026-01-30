package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/fsnotify/fsnotify"
)

// watchDirectory monitors a directory for new log files
func watchDirectory(ctx context.Context, watcher *fsnotify.Watcher, input config.InputSource, output chan<- *types.LogEntry) error {
	// Add directory to fsnotify watcher
	var err error
	input.Path, err = filepath.Abs(input.Path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %s: %w", input.Path, err)
	}
	absoluteDir := input.Path

	err = watcher.Add(absoluteDir)
	if err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", absoluteDir, err)
	}
	defer watcher.Remove(absoluteDir)

	log.Printf("Started watching directory: %s", absoluteDir)

	// Track active file watchers
	activeWatchers := make(map[string]context.CancelFunc)

	// Read existing files in the directory
	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	log.Printf("Found %d existing files in %s", len(entries), absoluteDir)

	// Spawn watchers for existing files
	for _, entry := range entries {
		fullPath := filepath.Join(absoluteDir, entry.Name())

		if shouldIgnoreFile(fullPath) {
			log.Printf("Ignoring file: %s", fullPath)
			continue
		}

		spawnFileWatcher(ctx, watcher, fullPath, input.Labels, output, activeWatchers)
	}

	// Event loop - watch for new files
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Only handle Create events
			if event.Op&fsnotify.Create == fsnotify.Create {
				fullPath := event.Name
				if !filepath.IsAbs(fullPath) {
					fullPath = filepath.Join(absoluteDir, fullPath)
				}
				if shouldIgnoreFile(fullPath) {
					continue
				}

				log.Printf("New file detected: %s", fullPath)
				spawnFileWatcher(ctx, watcher, fullPath, input.Labels, output, activeWatchers)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error for directory %s: %v", absoluteDir, err)

		case <-ctx.Done():
			log.Printf("Context cancelled, stopping directory watcher for %s", absoluteDir)
			// Cancel all active file watchers
			for _, cancel := range activeWatchers {
				cancel()
			}
			return nil
		}
	}
}

// spawnFileWatcher creates and starts a watcher for a single file
func spawnFileWatcher(ctx context.Context, watcher *fsnotify.Watcher, path string, labels map[string]string, output chan<- *types.LogEntry, activeWatchers map[string]context.CancelFunc) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Printf("failed to abs path %s: %v", path, err)
		return
	}

	// Check if already watching
	if _, exists := activeWatchers[absPath]; exists {
		log.Printf("Already watching %s, skipping", absPath)
		return
	}

	// Create input source
	fileInput := config.InputSource{
		Path:     absPath,
		PathType: types.File,
		Labels:   labels,
	}

	// Create cancellable context for this file
	fileCtx, fileCancel := context.WithCancel(ctx)
	activeWatchers[absPath] = fileCancel

	// Spawn goroutine
	go func() {
		defer func() {
			delete(activeWatchers, absPath)
			fileCancel()
		}()

		if err := watchFile(fileCtx, watcher, fileInput, output); err != nil {
			log.Printf("Error watching file %s: %v", absPath, err)
		}
	}()
}

// shouldIgnoreFile returns true if the file should be ignored
func shouldIgnoreFile(path string) bool {
	basename := filepath.Base(path)

	// Ignore hidden files (start with ".")
	if strings.HasPrefix(basename, ".") {
		return true
	}

	// Ignore Vim swap files
	if strings.HasSuffix(basename, ".swp") {
		return true
	}

	// Ignore temporary files
	if strings.HasSuffix(basename, ".tmp") {
		return true
	}

	// Ignore backup files
	if strings.HasSuffix(basename, "~") {
		return true
	}

	// Ignore directories (we only watch files)
	info, err := os.Stat(path)
	if err != nil {
		return true // If we can't stat it, ignore it
	}
	if info.IsDir() {
		return true
	}

	return false
}
