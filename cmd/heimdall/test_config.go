package main

import (
	"fmt"
	"log"
	"github.com/SmollCoco/heimdall/internal/config"
)

func main() {
	// Load config
	cfg, err := config.LoadConfig("../../configs/heimdall.example.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Print loaded config
	fmt.Println("✅ Config loaded successfully!")
	fmt.Printf("\n=== Configuration ===\n")
	fmt.Printf("Inputs: %d sources\n", len(cfg.Inputs))
	for i, input := range cfg.Inputs {
		fmt.Printf("  [%d] %s (%s)\n", i, input.Path, input.PathType)
	}
	fmt.Printf("\nOutput: %s\n", cfg.Output.Loki.URL)
	fmt.Printf("Worker Pool Size: %d\n", cfg.WorkerPoolSize)
	fmt.Printf("Channel Buffer Size: %d\n", cfg.ChannelBufferSize)
	fmt.Printf("Retry Max Attempts: %d\n", cfg.Retry.MaxAttempts)
	fmt.Printf("Self-Monitoring: %v\n", cfg.SelfMonitoring.Enabled)
}
