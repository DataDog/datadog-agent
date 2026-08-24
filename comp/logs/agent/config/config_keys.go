// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogsConfigKeys stores logs configuration keys stored in YAML configuration files
type LogsConfigKeys struct {
	prefix       string
	vectorPrefix string
	config       pkgconfigmodel.Reader

	// credentialProviders, when set, resolves delegated-auth credentials for additional endpoints
	// built from these keys. Nil means no delegated auth is wired for this subsystem.
	credentialProviders CredentialProviderLookup
}

// CompressionKind constants
const (
	GzipCompressionKind  = "gzip"
	GzipCompressionLevel = 6
	ZstdCompressionKind  = "zstd"
	ZstdCompressionLevel = 1
)

// defaultLogsConfigKeys defines the default YAML keys used to retrieve logs configuration
func defaultLogsConfigKeys(config pkgconfigmodel.Reader) *LogsConfigKeys {
	return NewLogsConfigKeys("logs_config.", config)
}

// defaultLogsConfigKeys defines the default YAML keys used to retrieve logs configuration
func defaultLogsConfigKeysWithVectorOverride(config pkgconfigmodel.Reader) *LogsConfigKeys {
	return NewLogsConfigKeysWithVector("logs_config.", "logs.", config)
}

// CredentialProviderLookup resolves the delegated-auth credential provider for one additional
// endpoint, identified by the setting it came from, its host, and its DELA(...) directive.
// It returns nil when that endpoint has no delegated-auth instance.
type CredentialProviderLookup func(configKey, host, directive string) CredentialProvider

// WithCredentialProviders attaches a lookup so endpoints built from these keys can take their
// credential from a provider instead of a configured API key. Returns l for chaining.
//
// Without it, a DELA(...) directive in additional_endpoints produces an endpoint that can never
// authorize, so nothing is sent there - deliberately, since the alternative is sending the
// directive text to the intake as if it were an API key.
func (l *LogsConfigKeys) WithCredentialProviders(lookup CredentialProviderLookup) *LogsConfigKeys {
	l.credentialProviders = lookup
	return l
}

// NewLogsConfigKeys returns a new logs configuration keys set
func NewLogsConfigKeys(configPrefix string, config pkgconfigmodel.Reader) *LogsConfigKeys {
	return &LogsConfigKeys{prefix: configPrefix, vectorPrefix: "", config: config}
}

// NewLogsConfigKeysWithVector returns a new logs configuration keys set with vector config keys enabled
func NewLogsConfigKeysWithVector(configPrefix, vectorPrefix string, config pkgconfigmodel.Reader) *LogsConfigKeys {
	return &LogsConfigKeys{prefix: configPrefix, vectorPrefix: vectorPrefix, config: config}
}

func (l *LogsConfigKeys) getConfig() pkgconfigmodel.Reader {
	return l.config
}

func (l *LogsConfigKeys) getConfigKey(key string) string {
	return l.prefix + key
}

func isSetAndNotEmpty(config pkgconfigmodel.Reader, key string) bool {
	return config.IsConfigured(key) && len(config.GetString(key)) > 0
}

func (l *LogsConfigKeys) isSetAndNotEmpty(key string) bool {
	return isSetAndNotEmpty(l.getConfig(), key)
}

func (l *LogsConfigKeys) ddURL443() string {
	return l.getConfig().GetString(l.getConfigKey("dd_url_443"))
}

func (l *LogsConfigKeys) logsDDURL() (string, bool) {
	configKey := l.getConfigKey("logs_dd_url")
	return l.getConfig().GetString(configKey), l.isSetAndNotEmpty(configKey)
}

func (l *LogsConfigKeys) ddPort() int {
	return l.getConfig().GetInt(l.getConfigKey("dd_port"))
}

func (l *LogsConfigKeys) isSocks5ProxySet() bool {
	return len(l.socks5ProxyAddress()) > 0
}

func (l *LogsConfigKeys) socks5ProxyAddress() string {
	return l.getConfig().GetString(l.getConfigKey("socks5_proxy_address"))
}

func (l *LogsConfigKeys) isForceTCPUse() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_tcp")) ||
		l.getConfig().GetBool(l.getConfigKey("force_use_tcp"))
}

func (l *LogsConfigKeys) usePort443() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_port_443"))
}

func (l *LogsConfigKeys) isForceHTTPUse() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_http")) ||
		l.getConfig().GetBool(l.getConfigKey("force_use_http"))
}

func (l *LogsConfigKeys) logsNoSSL() bool {
	return l.getConfig().GetBool(l.getConfigKey("logs_no_ssl"))
}

func (l *LogsConfigKeys) maxMessageSizeBytes() int {
	return l.getConfig().GetInt(l.getConfigKey("max_message_size_bytes"))
}

func (l *LogsConfigKeys) devModeNoSSL() bool {
	return l.getConfig().GetBool(l.getConfigKey("dev_mode_no_ssl"))
}

func (l *LogsConfigKeys) devModeUseProto() bool {
	return l.getConfig().GetBool(l.getConfigKey("dev_mode_use_proto"))
}

func (l *LogsConfigKeys) httpConnectivityRetryIntervalMax() time.Duration {
	return l.getConfig().GetDuration(l.getConfigKey("http_connectivity_retry_interval_max"))
}

func (l *LogsConfigKeys) compressionKind() string {
	configKey := l.getConfigKey("compression_kind")
	compressionKind := l.getConfig().GetString(configKey)

	endpoints, _ := l.getAdditionalEndpoints()
	if len(endpoints) > 0 {
		if !l.config.IsConfigured(configKey) {
			log.Debugf("Additional endpoints detected, pipeline: %s falling back to gzip compression for compatibility", l.prefix)
			return GzipCompressionKind
		}
	}

	if compressionKind == ZstdCompressionKind || compressionKind == GzipCompressionKind {
		return compressionKind
	}

	log.Warnf("Invalid compression kind: '%s', falling back to default compression: '%s' ", compressionKind, constants.DefaultLogCompressionKind)
	return constants.DefaultLogCompressionKind
}

func (l *LogsConfigKeys) compressionLevel() int {
	if l.compressionKind() == ZstdCompressionKind {
		level := l.getConfig().GetInt(l.getConfigKey("zstd_compression_level"))
		if strings.HasPrefix(l.prefix, "logs_config.") {
			log.Debugf("Logs pipeline is using compression zstd at level: %d", level)
		}
		return level
	}

	level := l.getConfig().GetInt(l.getConfigKey("compression_level"))
	if strings.HasPrefix(l.prefix, "logs_config.") {
		log.Debugf("Logs pipeline is using compression gzip atlevel: %d", level)
	}
	return level
}

func (l *LogsConfigKeys) useCompression() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_compression"))
}

func (l *LogsConfigKeys) hasAdditionalEndpoints() bool {
	endpoints, _ := l.getAdditionalEndpoints()
	return len(endpoints) > 0
}

// shouldUseTCP returns true if the configuration should use TCP.
// This happens when force_use_tcp, socks5_proxy_address, or additional_endpoints are set.
func (l *LogsConfigKeys) shouldUseTCP() bool {
	return l.isForceTCPUse() || l.isSocks5ProxySet() || l.hasAdditionalEndpoints()
}

// getMainAPIKey return the global API key for the current config with the path used to get it. Main api key means the
// top level one, not one from additional_endpoints.
func (l *LogsConfigKeys) getMainAPIKey() (string, string) {
	path := "api_key"
	if configKey := l.getConfigKey(path); l.isSetAndNotEmpty(configKey) {
		path = configKey
	}

	return l.getConfig().GetString(path), path
}

func (l *LogsConfigKeys) connectionResetInterval() time.Duration {
	return time.Duration(l.getConfig().GetInt(l.getConfigKey("connection_reset_interval"))) * time.Second

}

func (l *LogsConfigKeys) getAdditionalEndpoints() ([]unmarshalEndpoint, string) {
	var endpoints []unmarshalEndpoint
	configKey := l.getConfigKey("additional_endpoints")
	err := structure.UnmarshalKey(l.getConfig(), configKey, &endpoints, structure.EnableStringUnmarshal, structure.EnableSquash)
	if err != nil {
		log.Warnf("Could not parse additional_endpoints for logs: %v", err)
	}
	return endpoints, configKey
}

func (l *LogsConfigKeys) expectedTagsDuration() time.Duration {
	return l.getConfig().GetDuration(l.getConfigKey("expected_tags_duration"))
}

func (l *LogsConfigKeys) taggerWarmupDuration() time.Duration {
	// note that this multiplies a duration (in ns) by 1 second (in ns), so the user must specify
	// an integer number of seconds ("5") and not a duration expression ("5s").
	return l.getConfig().GetDuration(l.getConfigKey("tagger_warmup_duration")) * time.Second
}

func (l *LogsConfigKeys) batchWait() time.Duration {
	key := l.getConfigKey("batch_wait")
	batchWaitFloat := l.getConfig().GetFloat64(key)
	// Valid range: 0.1 seconds (100ms) to 10 seconds
	if batchWaitFloat < 0.1 || 10 < batchWaitFloat {
		log.Warnf("Invalid %s: %v should be in [0.1, 10], fallback on %v", key, batchWaitFloat, constants.DefaultBatchWait)
		return time.Duration(constants.DefaultBatchWait * float64(time.Second))
	}
	return time.Duration(batchWaitFloat * float64(time.Second))
}

func (l *LogsConfigKeys) batchMaxConcurrentSend() int {
	key := l.getConfigKey("batch_max_concurrent_send")
	batchMaxConcurrentSend := l.getConfig().GetInt(key)
	if batchMaxConcurrentSend < 0 {
		log.Warnf("Invalid %s: %v should be >= 0, fallback on %v", key, batchMaxConcurrentSend, constants.DefaultBatchMaxConcurrentSend)
		return constants.DefaultBatchMaxConcurrentSend
	}
	return batchMaxConcurrentSend
}

func (l *LogsConfigKeys) batchMaxSize() int {
	key := l.getConfigKey("batch_max_size")
	batchMaxSize := l.getConfig().GetInt(key)
	if batchMaxSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, batchMaxSize, constants.DefaultBatchMaxSize)
		return constants.DefaultBatchMaxSize
	}
	return batchMaxSize
}

func (l *LogsConfigKeys) batchMaxContentSize() int {
	key := l.getConfigKey("batch_max_content_size")
	batchMaxContentSize := l.getConfig().GetInt(key)
	if batchMaxContentSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, batchMaxContentSize, constants.DefaultBatchMaxContentSize)
		return constants.DefaultBatchMaxContentSize
	}
	return batchMaxContentSize
}

func (l *LogsConfigKeys) InputChanSize() int {
	key := l.getConfigKey("input_chan_size")
	inputChanSize := l.getConfig().GetInt(key)
	if inputChanSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, inputChanSize, constants.DefaultInputChanSize)
		return constants.DefaultInputChanSize
	}
	return inputChanSize
}

func (l *LogsConfigKeys) senderBackoffFactor() float64 {
	key := l.getConfigKey("sender_backoff_factor")
	senderBackoffFactor := l.getConfig().GetFloat64(key)
	if senderBackoffFactor < 2 {
		log.Warnf("Invalid %s: %v should be >= 2, fallback on %v", key, senderBackoffFactor, constants.DefaultLogsSenderBackoffFactor)
		return constants.DefaultLogsSenderBackoffFactor
	}
	return senderBackoffFactor
}

func (l *LogsConfigKeys) senderBackoffBase() float64 {
	key := l.getConfigKey("sender_backoff_base")
	senderBackoffBase := l.getConfig().GetFloat64(key)
	if senderBackoffBase <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, senderBackoffBase, constants.DefaultLogsSenderBackoffBase)
		return constants.DefaultLogsSenderBackoffBase
	}
	return senderBackoffBase
}

func (l *LogsConfigKeys) senderBackoffMax() float64 {
	key := l.getConfigKey("sender_backoff_max")
	senderBackoffMax := l.getConfig().GetFloat64(key)
	if senderBackoffMax <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, senderBackoffMax, constants.DefaultLogsSenderBackoffMax)
		return constants.DefaultLogsSenderBackoffMax
	}
	return senderBackoffMax
}

func (l *LogsConfigKeys) senderRecoveryInterval() int {
	key := l.getConfigKey("sender_recovery_interval")
	recoveryInterval := l.getConfig().GetInt(key)
	if recoveryInterval <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, recoveryInterval, constants.DefaultForwarderRecoveryInterval)
		return constants.DefaultForwarderRecoveryInterval
	}
	return recoveryInterval
}

func (l *LogsConfigKeys) senderRecoveryReset() bool {
	return l.getConfig().GetBool(l.getConfigKey("sender_recovery_reset"))
}

// AggregationTimeout is used when performing aggregation operations
func (l *LogsConfigKeys) aggregationTimeout() time.Duration {
	return l.getConfig().GetDuration(l.getConfigKey("aggregation_timeout")) * time.Millisecond
}

func (l *LogsConfigKeys) useV2API() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_v2_api"))
}

func (l *LogsConfigKeys) getObsPipelineConfigKey(configPrefix string, key string) string {
	return configPrefix + "." + l.vectorPrefix + key
}

func (l *LogsConfigKeys) obsPipelineWorkerEnabled() bool {
	if l.vectorPrefix == "" {
		return false
	}
	if l.getConfig().GetBool(l.getObsPipelineConfigKey("observability_pipelines_worker", "enabled")) {
		return true
	}
	return l.getConfig().GetBool(l.getObsPipelineConfigKey("vector", "enabled"))
}

// obsPipelineWorkerDualShip returns true when dual-ship mode is enabled, i.e. logs should be
// sent to both the primary Datadog intake and the Observability Pipelines Worker simultaneously.
// This method always returns false when the vectorPrefix is empty (i.e. the config keys instance
// was not constructed with OPW support).
// It mirrors the fallback pattern of obsPipelineWorkerEnabled / getObsPipelineURL: if the modern
// observability_pipelines_worker key is not set it falls back to the legacy vector.* key so that
// users still on the legacy config are not silently broken.
func (l *LogsConfigKeys) obsPipelineWorkerDualShip() bool {
	if l.vectorPrefix == "" {
		return false
	}
	if l.getConfig().GetBool(l.getObsPipelineConfigKey("observability_pipelines_worker", "dual_ship")) {
		return true
	}
	return l.getConfig().GetBool(l.getObsPipelineConfigKey("vector", "dual_ship"))
}

// obsPipelineWorkerDualShipReliable reports whether the OPW dual-ship endpoint should be treated
// as reliable. Reliable additional endpoints apply backpressure to the main pipeline if they fail,
// which can stall delivery to Datadog when OPW is degraded. The default is false (best-effort) so
// that an unhealthy OPW cannot block the primary Datadog destination; operators who want OPW to
// participate in flow control can opt in by setting this key to true.
// Like obsPipelineWorkerDualShip it falls back to the legacy vector.* prefix.
func (l *LogsConfigKeys) obsPipelineWorkerDualShipReliable() bool {
	if l.vectorPrefix == "" {
		return false
	}
	if l.getConfig().GetBool(l.getObsPipelineConfigKey("observability_pipelines_worker", "dual_ship_reliable")) {
		return true
	}
	return l.getConfig().GetBool(l.getObsPipelineConfigKey("vector", "dual_ship_reliable"))
}

func (l *LogsConfigKeys) getObsPipelineURL() (string, bool) {
	if l.vectorPrefix != "" {
		configKey := l.getObsPipelineConfigKey("observability_pipelines_worker", "url")
		if l.isSetAndNotEmpty(configKey) {
			return l.getConfig().GetString(configKey), true
		}

		configKey = l.getObsPipelineConfigKey("vector", "url")
		if l.isSetAndNotEmpty(configKey) {
			return l.getConfig().GetString(configKey), true
		}
	}
	return "", false
}
