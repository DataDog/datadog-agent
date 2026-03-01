// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamconsumerimpl

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// configReader implements model.Reader backed by the config stream
type configReader struct {
	consumer *consumer
}

// GetLibType returns the library type powering the configuration
func (r *configReader) GetLibType() string {
	return "configstream"
}

// Get returns the value for the given key
func (r *configReader) Get(key string) interface{} {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()
	return r.consumer.effectiveConfig[strings.ToLower(key)]
}

// GetString returns the value for the given key as a string
func (r *configReader) GetString(key string) string {
	val := r.Get(key)
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// GetBool returns the value for the given key as a bool
func (r *configReader) GetBool(key string) bool {
	val := r.Get(key)
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}

// GetInt returns the value for the given key as an int
func (r *configReader) GetInt(key string) int {
	val := r.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// GetInt32 returns the value for the given key as an int32
func (r *configReader) GetInt32(key string) int32 {
	return int32(r.GetInt(key))
}

// GetInt64 returns the value for the given key as an int64
func (r *configReader) GetInt64(key string) int64 {
	val := r.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		i, _ := strconv.ParseInt(v, 10, 64)
		return i
	default:
		return 0
	}
}

// GetFloat64 returns the value for the given key as a float64
func (r *configReader) GetFloat64(key string) float64 {
	val := r.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

// GetDuration returns the value for the given key as a time.Duration
func (r *configReader) GetDuration(key string) time.Duration {
	val := r.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case time.Duration:
		return v
	case int:
		return time.Duration(v)
	case int64:
		return time.Duration(v)
	case float64:
		return time.Duration(v)
	case string:
		d, _ := time.ParseDuration(v)
		return d
	default:
		return 0
	}
}

// GetStringSlice returns the value for the given key as a []string
func (r *configReader) GetStringSlice(key string) []string {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	case string:
		return []string{v}
	default:
		return nil
	}
}

// GetFloat64Slice returns the value for the given key as a []float64
func (r *configReader) GetFloat64Slice(key string) []float64 {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []float64:
		return v
	case []interface{}:
		result := make([]float64, 0, len(v))
		for _, item := range v {
			if f, ok := item.(float64); ok {
				result = append(result, f)
			}
		}
		return result
	default:
		return nil
	}
}

// GetStringMap returns the value for the given key as a map[string]interface{}
func (r *configReader) GetStringMap(key string) map[string]interface{} {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	if m, ok := val.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// GetStringMapString returns the value for the given key as a map[string]string
func (r *configReader) GetStringMapString(key string) map[string]string {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case map[string]string:
		return v
	case map[string]interface{}:
		result := make(map[string]string, len(v))
		for k, val := range v {
			result[k] = fmt.Sprintf("%v", val)
		}
		return result
	default:
		return nil
	}
}

// GetStringMapStringSlice returns the value for the given key as a map[string][]string
func (r *configReader) GetStringMapStringSlice(key string) map[string][]string {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	if m, ok := val.(map[string][]string); ok {
		return m
	}
	return nil
}

// GetSizeInBytes returns the value for the given key as a size in bytes
func (r *configReader) GetSizeInBytes(key string) uint {
	return uint(r.GetInt64(key))
}

// GetProxies returns the proxy configuration
func (r *configReader) GetProxies() *model.Proxy {
	return nil
}

// GetSequenceID returns the last received sequence ID
func (r *configReader) GetSequenceID() uint64 {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()
	return uint64(r.consumer.lastSeqID)
}

// GetSource returns the source of the config value
func (r *configReader) GetSource(_ string) model.Source {
	// In the streamed config, everything comes from the core agent
	return model.SourceAgentRuntime
}

// GetAllSources returns all sources for the given key
func (r *configReader) GetAllSources(key string) []model.ValueWithSource {
	val := r.Get(key)
	if val == nil {
		return nil
	}
	return []model.ValueWithSource{
		{Value: val, Source: model.SourceAgentRuntime},
	}
}

// GetSubfields returns subfields for the given key
func (r *configReader) GetSubfields(key string) []string {
	val := r.Get(key)
	if m, ok := val.(map[string]interface{}); ok {
		subfields := make([]string, 0, len(m))
		for k := range m {
			subfields = append(subfields, k)
		}
		return subfields
	}
	return nil
}

// ConfigFileUsed returns the config file used
func (r *configReader) ConfigFileUsed() string {
	return ""
}

// ExtraConfigFilesUsed returns extra config files used
func (r *configReader) ExtraConfigFilesUsed() []string {
	return nil
}

// AllSettings returns all settings
func (r *configReader) AllSettings() map[string]interface{} {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]interface{}, len(r.consumer.effectiveConfig))
	for k, v := range r.consumer.effectiveConfig {
		result[k] = v
	}
	return result
}

// AllSettingsWithoutDefault returns all settings without defaults
func (r *configReader) AllSettingsWithoutDefault() map[string]interface{} {
	return r.AllSettings()
}

// AllSettingsBySource returns all settings by source
func (r *configReader) AllSettingsBySource() map[model.Source]interface{} {
	return map[model.Source]interface{}{
		model.SourceAgentRuntime: r.AllSettings(),
	}
}

// AllKeysLowercased returns all config keys lowercased
func (r *configReader) AllKeysLowercased() []string {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()

	keys := make([]string, 0, len(r.consumer.effectiveConfig))
	for k := range r.consumer.effectiveConfig {
		keys = append(keys, strings.ToLower(k))
	}
	return keys
}

// AllSettingsWithSequenceID returns all settings with sequence ID
func (r *configReader) AllSettingsWithSequenceID() (map[string]interface{}, uint64) {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()

	result := make(map[string]interface{}, len(r.consumer.effectiveConfig))
	for k, v := range r.consumer.effectiveConfig {
		result[k] = v
	}
	return result, uint64(r.consumer.lastSeqID)
}

// AllFlattenedSettingsWithSequenceID returns all settings as a flattened map with the current sequence ID
// The effective config is already flattened with dot notation keys (e.g., "logs_config.enabled")
func (r *configReader) AllFlattenedSettingsWithSequenceID() (map[string]interface{}, uint64) {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()

	result := make(map[string]interface{}, len(r.consumer.effectiveConfig))
	for k, v := range r.consumer.effectiveConfig {
		result[k] = v
	}
	return result, uint64(r.consumer.lastSeqID)
}

// SetTestOnlyDynamicSchema is used by tests
func (r *configReader) SetTestOnlyDynamicSchema(_ bool) {
	// No-op for streamed config
}

// IsSet returns true if a value is found
func (r *configReader) IsSet(key string) bool {
	_, exists := r.consumer.effectiveConfig[strings.ToLower(key)]
	return exists
}

// IsConfigured returns true if configured by the user
func (r *configReader) IsConfigured(key string) bool {
	return r.IsSet(key)
}

// HasSection returns true if the key is for a non-leaf setting
func (r *configReader) HasSection(key string) bool {
	val := r.Get(key)
	_, isMap := val.(map[string]interface{})
	return isMap
}

// IsKnown returns whether this key is known
func (r *configReader) IsKnown(key string) bool {
	return r.IsSet(key)
}

// GetKnownKeysLowercased returns all known keys lowercased
func (r *configReader) GetKnownKeysLowercased() map[string]interface{} {
	r.consumer.configLock.RLock()
	defer r.consumer.configLock.RUnlock()

	result := make(map[string]interface{}, len(r.consumer.effectiveConfig))
	for k, v := range r.consumer.effectiveConfig {
		result[strings.ToLower(k)] = v
	}
	return result
}

// GetEnvVars returns a list of env vars
func (r *configReader) GetEnvVars() []string {
	return nil
}

// Warnings returns pointer to warnings
func (r *configReader) Warnings() *model.Warnings {
	return &model.Warnings{}
}

// Object returns Reader
func (r *configReader) Object() model.Reader {
	return r
}

// OnUpdate adds a callback for config changes
func (r *configReader) OnUpdate(callback model.NotificationReceiver) {
	// Subscribe to change events and invoke callback
	go func() {
		ch, unsubscribe := r.consumer.Subscribe()
		defer unsubscribe()

		for event := range ch {
			callback(r.consumer.params.ClientName, model.SourceAgentRuntime, event.OldValue, event.NewValue, uint64(r.consumer.lastSeqID))
		}
	}()
}

// Stringify stringifies the config (test-only)
func (r *configReader) Stringify(_ model.Source, _ ...model.StringifyOption) string {
	return fmt.Sprintf("%v", r.AllSettings())
}
