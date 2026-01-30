package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/SmollCoco/heimdall/internal/config"
	"github.com/SmollCoco/heimdall/internal/processor"
	"github.com/SmollCoco/heimdall/internal/shipper"
	"github.com/SmollCoco/heimdall/internal/types"
	"github.com/SmollCoco/heimdall/internal/watcher"
)

// Build info (set via -ldflags in step C)
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	var (
		configPath = flag.String("config", "/etc/heimdall/config.yaml", "Path to config file")
		showVer    = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("heimdall version=%s commit=%s date=%s\n", Version, Commit, Date)
		os.Exit(0)
	}

	// Context canceled on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	log.Printf("Heimdall starting (config=%s)", *configPath)

	// load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("config error: %v", err)
		os.Exit(2)
	}

	batchTimeout, err := time.ParseDuration(cfg.Output.Loki.BatchTimeoutSeconds)
	if err != nil {
		log.Printf("runtime error: invalid loki batch timeout: %v", err)
		os.Exit(1)
	}
	initialBackoff, err := time.ParseDuration(cfg.Retry.InitialBackoffSeconds)
	if err != nil {
		log.Printf("runtime error: invalid retry initial backoff: %v", err)
		os.Exit(1)
	}
	maxBackoff, err := time.ParseDuration(cfg.Retry.MaxBackoffSeconds)
	if err != nil {
		log.Printf("runtime error: invalid retry max backoff: %v", err)
		os.Exit(1)
	}

	// create channels using cfg.ChannelBufferSize
	watcherCh := make(chan *types.LogEntry, cfg.ChannelBufferSize)
	processorCh := make(chan *types.ProcessedEntry, cfg.ChannelBufferSize)

	// Start watcher, processor, shipper with proper goroutine ownership
	// watcher should stop on ctx.Done()
	errCh := make(chan error, 3)
	var wg sync.WaitGroup
	setErr := func(err error) {
		if err == nil {
			return
		}
		select {
		case errCh <- err:
		default:
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := watcher.Start(ctx, cfg.Inputs, watcherCh); err != nil {
			setErr(err)
			stop()
		}
		close(watcherCh)
	}()

	// processor workers should stop/drain on ctx.Done()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := processor.Start(ctx, watcherCh, processorCh, cfg.WorkerPoolSize); err != nil {
			setErr(err)
			stop()
		}
		close(processorCh)
	}()

	// shipper should flush on shutdown and return nil
	ship := shipper.NewShipper(
		cfg.Output.Loki.URL,
		cfg.Output.Loki.BatchSize,
		batchTimeout,
		cfg.Retry.MaxAttempts,
		initialBackoff,
		maxBackoff,
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ship.Start(ctx, processorCh); err != nil {
			setErr(err)
			stop()
		}
	}()

	<-ctx.Done()
	// At this point, signal received; the real work is graceful shutdown.
	log.Printf("shutdown requested: %v", ctx.Err())

	// wait for pipeline to finish
	wg.Wait()
	close(errCh)

	// Evaluate errors (non-cancel) for exit code 1
	hadFatal := false
	for e := range errCh {
		if e == nil {
			continue
		}
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			continue
		}
		log.Printf("pipeline error: %v", e)
		hadFatal = true
	}

	log.Printf("Heimdall exited cleanly (uptime=%s)", time.Since(start).Truncate(time.Millisecond))
	if hadFatal {
		os.Exit(1)
	}
	os.Exit(0)
}
