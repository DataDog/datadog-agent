package config

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/writer/backoff"
)

// QueuablePayloadSenderConf contains the configuration needed by a QueuablePayloadSender to operate.
type QueuablePayloadSenderConf struct {
	MaxAge             time.Duration
	MaxQueuedBytes     int64
	MaxQueuedPayloads  int
	ExponentialBackoff backoff.ExponentialConfig
}

// DefaultQueuablePayloadSenderConf constructs a QueuablePayloadSenderConf with default sane options.
func DefaultQueuablePayloadSenderConf() QueuablePayloadSenderConf {
	return QueuablePayloadSenderConf{
		MaxAge:             20 * time.Minute,
		MaxQueuedBytes:     64 * 1024 * 1024, // 64 MB
		MaxQueuedPayloads:  -1,               // Unlimited
		ExponentialBackoff: backoff.DefaultExponentialConfig(),
	}
}
