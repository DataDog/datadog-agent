package config

import "time"

// maxEntriesPerPayload is the maximum number of entries in a stat payload. An
// entry has an average size of 125 bytes in a compressed payload. The current
// Datadog intake API limits a compressed payload to ~3MB (24,000 entries), but
// let's have the default ensure we don't have paylods > 1.5 MB (12,000
// entries).
const maxEntriesPerPayload = 12000

// StatsWriterConfig contains the configuration to customize the behaviour of a TraceWriter.
type StatsWriterConfig struct {
	MaxEntriesPerPayload int
	UpdateInfoPeriod     time.Duration
	SenderConfig         QueuablePayloadSenderConf
}

// DefaultStatsWriterConfig creates a new instance of a StatsWriterConfig using default values.
func DefaultStatsWriterConfig() StatsWriterConfig {
	return StatsWriterConfig{
		MaxEntriesPerPayload: maxEntriesPerPayload,
		UpdateInfoPeriod:     1 * time.Minute,
		SenderConfig:         DefaultQueuablePayloadSenderConf(),
	}
}
