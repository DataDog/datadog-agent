package config

import "time"

// TraceWriterConfig contains the configuration to customize the behaviour of a TraceWriter.
type TraceWriterConfig struct {
	FlushPeriod      time.Duration
	UpdateInfoPeriod time.Duration
	SenderConfig     QueuablePayloadSenderConf
}

// DefaultTraceWriterConfig creates a new instance of a TraceWriterConfig using default values.
func DefaultTraceWriterConfig() TraceWriterConfig {
	return TraceWriterConfig{
		FlushPeriod:      5 * time.Second,
		UpdateInfoPeriod: 1 * time.Minute,
		SenderConfig:     DefaultQueuablePayloadSenderConf(),
	}
}
