package config

import (
	"time"
)

// ServiceWriterConfig contains the configuration to customize the behaviour of a ServiceWriter.
type ServiceWriterConfig struct {
	FlushPeriod      time.Duration
	UpdateInfoPeriod time.Duration
	SenderConfig     QueuablePayloadSenderConf
}

// DefaultServiceWriterConfig creates a new instance of a ServiceWriterConfig using default values.
func DefaultServiceWriterConfig() ServiceWriterConfig {
	return ServiceWriterConfig{
		FlushPeriod:      5 * time.Second,
		UpdateInfoPeriod: 1 * time.Minute,
		SenderConfig:     DefaultQueuablePayloadSenderConf(),
	}
}
