// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogsConfigKeys stores logs configuration keys stored in YAML configuration files
type LogsConfigKeys struct {
	prefix       string
	vectorPrefix string
	config       coreConfig.ConfigReader
}

// defaultLogsConfigKeys defines the default YAML keys used to retrieve logs configuration
func defaultLogsConfigKeys(config coreConfig.ConfigReader) *LogsConfigKeys {
	return NewLogsConfigKeys("logs_config.", config)
}

// defaultLogsConfigKeys defines the default YAML keys used to retrieve logs configuration
func defaultLogsConfigKeysWithVectorOverride(config coreConfig.ConfigReader) *LogsConfigKeys {
	return NewLogsConfigKeysWithVector("logs_config.", "logs.", config)
}

// NewLogsConfigKeys returns a new logs configuration keys set
func NewLogsConfigKeys(configPrefix string, config coreConfig.ConfigReader) *LogsConfigKeys {
	return &LogsConfigKeys{prefix: configPrefix, vectorPrefix: "", config: config}
}

// NewLogsConfigKeysWithVector returns a new logs configuration keys set with vector config keys enabled
func NewLogsConfigKeysWithVector(configPrefix, vectorPrefix string, config coreConfig.ConfigReader) *LogsConfigKeys {
	return &LogsConfigKeys{prefix: configPrefix, vectorPrefix: vectorPrefix, config: config}
}

func (l *LogsConfigKeys) getConfig() coreConfig.ConfigReader {
	return l.config
}

func (l *LogsConfigKeys) getConfigKey(key string) string {
	return l.prefix + key
}

func isSetAndNotEmpty(config coreConfig.ConfigReader, key string) bool {
	return config.IsSet(key) && len(config.GetString(key)) > 0
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

func (l *LogsConfigKeys) compressionLevel() int {
	return l.getConfig().GetInt(l.getConfigKey("compression_level"))
}

func (l *LogsConfigKeys) useCompression() bool {
	return l.getConfig().GetBool(l.getConfigKey("use_compression"))
}

func (l *LogsConfigKeys) hasAdditionalEndpoints() bool {
	return len(l.getAdditionalEndpoints()) > 0
}

// getLogsAPIKey provides the dd api key used by the main logs agent sender.
func (l *LogsConfigKeys) getLogsAPIKey() string {
	if configKey := l.getConfigKey("api_key"); l.isSetAndNotEmpty(configKey) {
		return configUtils.SanitizeAPIKey(l.getConfig().GetString(configKey))
	}
	return configUtils.SanitizeAPIKey(l.getConfig().GetString("api_key"))
}

func (l *LogsConfigKeys) connectionResetInterval() time.Duration {
	return time.Duration(l.getConfig().GetInt(l.getConfigKey("connection_reset_interval"))) * time.Second

}

func (l *LogsConfigKeys) getAdditionalEndpoints() []Endpoint {
	var endpoints []Endpoint
	var err error
	configKey := l.getConfigKey("additional_endpoints")
	raw := l.getConfig().Get(configKey)
	if raw == nil {
		return endpoints
	}
	if s, ok := raw.(string); ok && s != "" {
		err = json.Unmarshal([]byte(s), &endpoints)
	} else {
		err = l.getConfig().UnmarshalKey(configKey, &endpoints)
	}
	if err != nil {
		log.Warnf("Could not parse additional_endpoints for logs: %v", err)
	}
	return endpoints
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
	batchWait := l.getConfig().GetInt(key)
	if batchWait < 1 || 10 < batchWait {
		log.Warnf("Invalid %s: %v should be in [1, 10], fallback on %v", key, batchWait, coreConfig.DefaultBatchWait)
		return coreConfig.DefaultBatchWait * time.Second
	}
	return (time.Duration(batchWait) * time.Second)
}

func (l *LogsConfigKeys) batchMaxConcurrentSend() int {
	key := l.getConfigKey("batch_max_concurrent_send")
	batchMaxConcurrentSend := l.getConfig().GetInt(key)
	if batchMaxConcurrentSend < 0 {
		log.Warnf("Invalid %s: %v should be >= 0, fallback on %v", key, batchMaxConcurrentSend, coreConfig.DefaultBatchMaxConcurrentSend)
		return coreConfig.DefaultBatchMaxConcurrentSend
	}
	return batchMaxConcurrentSend
}

func (l *LogsConfigKeys) batchMaxSize() int {
	key := l.getConfigKey("batch_max_size")
	batchMaxSize := l.getConfig().GetInt(key)
	if batchMaxSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, batchMaxSize, coreConfig.DefaultBatchMaxSize)
		return coreConfig.DefaultBatchMaxSize
	}
	return batchMaxSize
}

func (l *LogsConfigKeys) batchMaxContentSize() int {
	key := l.getConfigKey("batch_max_content_size")
	batchMaxContentSize := l.getConfig().GetInt(key)
	if batchMaxContentSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, batchMaxContentSize, coreConfig.DefaultBatchMaxContentSize)
		return coreConfig.DefaultBatchMaxContentSize
	}
	return batchMaxContentSize
}

func (l *LogsConfigKeys) inputChanSize() int {
	key := l.getConfigKey("input_chan_size")
	inputChanSize := l.getConfig().GetInt(key)
	if inputChanSize <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, inputChanSize, coreConfig.DefaultInputChanSize)
		return coreConfig.DefaultInputChanSize
	}
	return inputChanSize
}

func (l *LogsConfigKeys) senderBackoffFactor() float64 {
	key := l.getConfigKey("sender_backoff_factor")
	senderBackoffFactor := l.getConfig().GetFloat64(key)
	if senderBackoffFactor < 2 {
		log.Warnf("Invalid %s: %v should be >= 2, fallback on %v", key, senderBackoffFactor, coreConfig.DefaultLogsSenderBackoffFactor)
		return coreConfig.DefaultLogsSenderBackoffFactor
	}
	return senderBackoffFactor
}

func (l *LogsConfigKeys) senderBackoffBase() float64 {
	key := l.getConfigKey("sender_backoff_base")
	senderBackoffBase := l.getConfig().GetFloat64(key)
	if senderBackoffBase <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, senderBackoffBase, coreConfig.DefaultLogsSenderBackoffBase)
		return coreConfig.DefaultLogsSenderBackoffBase
	}
	return senderBackoffBase
}

func (l *LogsConfigKeys) senderBackoffMax() float64 {
	key := l.getConfigKey("sender_backoff_max")
	senderBackoffMax := l.getConfig().GetFloat64(key)
	if senderBackoffMax <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, senderBackoffMax, coreConfig.DefaultLogsSenderBackoffMax)
		return coreConfig.DefaultLogsSenderBackoffMax
	}
	return senderBackoffMax
}

func (l *LogsConfigKeys) senderRecoveryInterval() int {
	key := l.getConfigKey("sender_recovery_interval")
	recoveryInterval := l.getConfig().GetInt(key)
	if recoveryInterval <= 0 {
		log.Warnf("Invalid %s: %v should be > 0, fallback on %v", key, recoveryInterval, coreConfig.DefaultLogsSenderBackoffRecoveryInterval)
		return coreConfig.DefaultLogsSenderBackoffRecoveryInterval
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
