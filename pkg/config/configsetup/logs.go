package configsetup

import "github.com/DataDog/datadog-agent/pkg/conf"

const (

	// DefaultBatchWait is the default HTTP batch wait in second for logs
	DefaultBatchWait = 5

	// DefaultBatchMaxConcurrentSend is the default HTTP batch max concurrent send for logs
	DefaultBatchMaxConcurrentSend = 0

	// DefaultBatchMaxSize is the default HTTP batch max size (maximum number of events in a single batch) for logs
	DefaultBatchMaxSize = 1000

	// DefaultInputChanSize is the default input chan size for events
	DefaultInputChanSize = 100

	// DefaultBatchMaxContentSize is the default HTTP batch max content size (before compression) for logs
	// It is also the maximum possible size of a single event. Events exceeding this limit are dropped.
	DefaultBatchMaxContentSize = 5000000

	// DefaultAuditorTTL is the default logs auditor TTL in hours
	DefaultAuditorTTL = 23
	// DefaultLogsSenderBackoffFactor is the default logs sender backoff randomness factor
	DefaultLogsSenderBackoffFactor = 2.0

	// DefaultLogsSenderBackoffBase is the default logs sender base backoff time, seconds
	DefaultLogsSenderBackoffBase = 1.0

	// DefaultLogsSenderBackoffMax is the default logs sender maximum backoff time, seconds
	DefaultLogsSenderBackoffMax = 120.0

	// DefaultLogsSenderBackoffRecoveryInterval is the default logs sender backoff recovery interval
	DefaultLogsSenderBackoffRecoveryInterval = 2
	// DefaultForwarderRecoveryInterval is the default recovery interval,
	// also used if the user-provided value is invalid.
	DefaultForwarderRecoveryInterval = 2
)

func bindEnvAndSetLogsConfigKeys(config conf.Config, prefix string) {
	config.BindEnv(prefix + "logs_dd_url") // Send the logs to a proxy. Must respect format '<HOST>:<PORT>' and '<PORT>' to be an integer
	config.BindEnv(prefix + "dd_url")
	config.BindEnv(prefix + "additional_endpoints")
	config.BindEnvAndSetDefault(prefix+"use_compression", true)
	config.BindEnvAndSetDefault(prefix+"compression_level", 6) // Default level for the gzip/deflate algorithm
	config.BindEnvAndSetDefault(prefix+"batch_wait", DefaultBatchWait)
	config.BindEnvAndSetDefault(prefix+"connection_reset_interval", 0) // in seconds, 0 means disabled
	config.BindEnvAndSetDefault(prefix+"logs_no_ssl", false)
	config.BindEnvAndSetDefault(prefix+"batch_max_concurrent_send", DefaultBatchMaxConcurrentSend)
	config.BindEnvAndSetDefault(prefix+"batch_max_content_size", DefaultBatchMaxContentSize)
	config.BindEnvAndSetDefault(prefix+"batch_max_size", DefaultBatchMaxSize)
	config.BindEnvAndSetDefault(prefix+"input_chan_size", DefaultInputChanSize) // Only used by EP Forwarder for now, not used by logs
	config.BindEnvAndSetDefault(prefix+"sender_backoff_factor", DefaultLogsSenderBackoffFactor)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_base", DefaultLogsSenderBackoffBase)
	config.BindEnvAndSetDefault(prefix+"sender_backoff_max", DefaultLogsSenderBackoffMax)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_interval", DefaultForwarderRecoveryInterval)
	config.BindEnvAndSetDefault(prefix+"sender_recovery_reset", false)
	config.BindEnvAndSetDefault(prefix+"use_v2_api", true)
}
