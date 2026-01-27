package watcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/fsnotify/fsnotify"
)

// watchFile monitors a single file for changes
func watchFile(ctx context.Context, watcher *fsnotify.Watcher, input config.InputSource, output chan<- *types.LogEntry) error {
	absPath, err := filepath.Abs(input.Path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %s: %w", input.Path, err)
	}

	// Open the file
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", absPath, err)
	}
	defer file.Close()

	// Seek to end (we don't want to read existing logs on startup)
	_, err = file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek to end of file %s: %w", absPath, err)
	}

	// Add file to fsnotify watcher
	err = watcher.Add(absPath)
	if err != nil {
		return fmt.Errorf("failed to add file %s to watcher: %w", absPath, err)
	}
	defer watcher.Remove(absPath)

	log.Printf("Started watching file: %s", absPath)

	// Event loop
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Only process events for our file (fsnotify might send events for other files)
			if event.Name != absPath {
				continue
			}

			switch {
			case event.Op&fsnotify.Write == fsnotify.Write:
				// File was modified, read new lines
				log.Printf("DEBUG [watchFile]: Write event for %s", absPath)
				if err := readNewLines(file, absPath, input.Labels, output); err != nil {
					log.Printf("Error reading from %s: %v", absPath, err)
				}

			case event.Op&fsnotify.Remove == fsnotify.Remove:
				// File was deleted, stop watching
				log.Printf("File %s was deleted, stopping watcher", input.Path)
				return nil

			case event.Op&fsnotify.Rename == fsnotify.Rename:
				// File was renamed (likely rotated)
				log.Printf("File %s was renamed/rotated", input.Path)
				// For MVP, we'll stop watching and let systemd restart us
				// In Phase 3, we'll implement proper rotation handling
				return nil
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error for file %s: %v", absPath, err)

		case <-ctx.Done():
			log.Printf("Context cancelled, stopping watcher for %s", absPath)
			return nil
		}
	}
}

// readNewLines reads new lines from the file and sends them to the channel
func readNewLines(file *os.File, source string, labels map[string]string, output chan<- *types.LogEntry) error {
	log.Printf("DEBUG [readNewLines]: source=%s labels=%+v", source, labels)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Create LogEntry (scanner.Bytes() is a shared buffer, so we need to copy)
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		log.Printf("DEBUG [readNewLines]: line=%s", string(lineCopy))

		entry := types.NewLogEntry(lineCopy, source, labels)

		// Send to output channel
		select {
		case output <- entry:
			// Successfully sent
		default:
			// Channel full, log warning
			log.Printf("Warning: output channel full, dropping log from %s", source)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}
