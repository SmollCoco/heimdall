package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SmollCoco/heimdall/internal/config"
)

// Build info (set via -ldflags in step C)
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "help", "-h", "--help":
		usage(os.Stdout)
		os.Exit(0)

	case "version":
		fmt.Printf("heimdall-cli version=%s commit=%s date=%s\n", Version, Commit, Date)
		os.Exit(0)

	case "sample-config":
		printSampleConfig()
		os.Exit(0)

	case "validate":
		validateCmd(os.Args[2:]) // exits inside

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprintln(w, "Heimdall CLI (helper tool)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  heimdall-cli validate -config <path>")
	fmt.Fprintln(w, "  heimdall-cli sample-config")
	fmt.Fprintln(w, "  heimdall-cli version")
	fmt.Fprintln(w, "  heimdall-cli help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Exit codes:")
	fmt.Fprintln(w, "  0 success")
	fmt.Fprintln(w, "  1 validation/runtime error")
	fmt.Fprintln(w, "  2 usage/flags error")
}

func validateCmd(args []string) {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "/etc/heimdall/config.yaml", "Path to config file")
	if err := fs.Parse(args); err != nil {
		// flag package already printed an error
		os.Exit(2)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config invalid: %v\n", err)
		os.Exit(1)
	}

	// Small useful output (keep it simple)
	fmt.Printf("OK: inputs=%d worker_pool_size=%d channel_buffer_size=%d loki_url=%s\n",
		len(cfg.Inputs), cfg.WorkerPoolSize, cfg.ChannelBufferSize, cfg.Output.Loki.URL)

	os.Exit(0)
}

func printSampleConfig() {
	fmt.Print(`# Example Heimdall Configuration
inputs:
  - path: /var/log/
    path_type: directory
    labels:
      service: system
      environment: production

output:
  loki:
    url: http://localhost:3100
    batch_size: 100
    batch_timeout: 5s

worker_pool_size: 4
channel_buffer_size: 1000

retry:
  max_attempts: 5
  initial_backoff: 1s
  max_backoff: 30s

self_monitoring:
  enabled: false
  interval: 30s
`)
}
