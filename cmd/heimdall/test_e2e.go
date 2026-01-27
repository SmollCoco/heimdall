package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/processor"
	"github.com/SmollCoco/heimdall/internal/shipper"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/SmollCoco/heimdall/internal/watcher"
)

func main() {
	log.Println("=== Heimdall End-to-End Pipeline Test ===")

	// Create test directory
	testDir := "./tmp/heimdall_e2e_test"
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)
	defer os.RemoveAll(testDir)

	testFile := testDir + "/app.log"
	os.WriteFile(testFile, []byte(""), 0644)

	log.Printf("Created test file: %s", testFile)

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

	// Create channels
	watcherChan := make(chan *types.LogEntry, 100)
	processorChan := make(chan *types.ProcessedEntry, 100)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watcher
	go func() {
		if err := watcher.Start(ctx, inputs, watcherChan); err != nil {
			log.Printf("Watcher error: %v", err)
		}
		close(watcherChan)
	}()

	// Start processor
	go func() {
		if err := processor.Start(ctx, watcherChan, processorChan, 2); err != nil {
			log.Printf("Processor error: %v", err)
		}
		close(processorChan)
	}()

	// Start shipper
	lokiShipper := shipper.NewShipper(
		"http://localhost:3100",
		10,             // batch_size
		2*time.Second,  // batch_timeout
		5,              // max_retries
		1*time.Second,  // initial_backoff
		30*time.Second, // max_backoff
	)

	go func() {
		if err := lokiShipper.Start(ctx, processorChan); err != nil {
			log.Printf("Shipper error: %v", err)
		}
	}()

	// Give pipeline time to initialize
	time.Sleep(1 * time.Second)

	// Write test logs
	log.Println("\n--- Writing test logs to file ---")
	f, _ := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)

	testLogs := []string{
		"INFO: Application started successfully",
		"DEBUG: Loading configuration",
		"WARN: Connection pool running low",
		"ERROR: Failed to connect to database",
		"FATAL: Out of memory",
		"Regular log without level",
	}

	for i, logLine := range testLogs {
		fmt.Fprintf(f, "[%d] %s\n", i+1, logLine)
		log.Printf("Wrote: %s", logLine)
		time.Sleep(500 * time.Millisecond)
	}
	f.Close()

	// Wait for processing
	log.Println("\n--- Waiting for pipeline to process logs ---")
	time.Sleep(5 * time.Second)

	log.Println("\n=== Test Complete! ===")
	log.Println("Check Grafana: http://localhost:3000")
	log.Println("Query: {service=\"e2e-test\"}")
	log.Println("\nPress Ctrl+C to exit")

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nShutting down...")
	cancel()
	time.Sleep(1 * time.Second)
}
