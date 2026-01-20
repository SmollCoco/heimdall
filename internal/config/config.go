// config.go
package config

import (
	"github.com/SmollCoco/heimdall/internal/types"
)

type InputSource struct {
	Path     string            `yaml:"path"`
	PathType types.PathType    `yaml:"path_type"`
	Labels   map[string]string `yaml:"labels"`
}

type LokiConfig struct {
	URL                 string `yaml:"url"`
	BatchSize           int    `yaml:"batch_size"`
	BatchTimeoutSeconds string `yaml:"batch_timeout"`
}

type Output struct {
	Loki LokiConfig `yaml:"loki"`
}

type Retry struct {
	MaxAttempts           int    `yaml:"max_attempts"`
	InitialBackoffSeconds string `yaml:"initial_backoff"`
	MaxBackoffSeconds     string `yaml:"max_backoff"`
}

type SelfMonitoring struct {
	Enabled      bool   `yaml:"enabled"`
	IntervalSecs string `yaml:"interval"`
}

type Config struct {
	Inputs            []InputSource  `yaml:"inputs"`
	Output            Output         `yaml:"output"`
	WorkerPoolSize    int            `yaml:"worker_pool_size"`
	ChannelBufferSize int            `yaml:"channel_buffer_size"`
	Retry             Retry          `yaml:"retry"`
	SelfMonitoring    SelfMonitoring `yaml:"self_monitoring"`
}
