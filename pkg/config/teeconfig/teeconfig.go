// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package teeconfig is a tee of two configs that writes to both but reads from only one
package teeconfig

import (
	"io"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/viper"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// teeConfig is a combination of two configs, both get written to but only baseline is read
type teeConfig struct {
	baseline model.Config
	compare  model.Config
}

// NewTeeConfig constructs a new teeConfig
func NewTeeConfig(baseline, compare model.Config) model.Config {
	return &teeConfig{baseline: baseline, compare: compare}
}

// OnUpdate adds a callback to the list receivers to be called each time a value is changed in the configuration
// by a call to the 'Set' method.
// Callbacks are only called if the value is effectively changed.
func (t *teeConfig) OnUpdate(callback model.NotificationReceiver) {
	t.baseline.OnUpdate(callback)
	t.compare.OnUpdate(callback)
}

// Set wraps Viper for concurrent access
func (t *teeConfig) Set(key string, newValue interface{}, source model.Source) {
	t.baseline.Set(key, newValue, source)
	t.compare.Set(key, newValue, source)
}

// SetWithoutSource sets the given value using source Unknown
func (t *teeConfig) SetWithoutSource(key string, value interface{}) {
	t.baseline.SetWithoutSource(key, value)
	t.compare.SetWithoutSource(key, value)
}

// SetDefault wraps Viper for concurrent access
func (t *teeConfig) SetDefault(key string, value interface{}) {
	t.baseline.SetDefault(key, value)
	t.compare.SetDefault(key, value)
}

// UnsetForSource unsets a config entry for a given source
func (t *teeConfig) UnsetForSource(key string, source model.Source) {
	t.baseline.UnsetForSource(key, source)
	t.compare.UnsetForSource(key, source)
}

// SetKnown adds a key to the set of known valid config keys
func (t *teeConfig) SetKnown(key string) {
	t.baseline.SetKnown(key)
	t.compare.SetKnown(key)
}

// IsKnown returns whether a key is known
func (t *teeConfig) IsKnown(key string) bool {
	base := t.baseline.IsKnown(key)
	compare := t.compare.IsKnown(key)
	if base != compare {
		log.Warnf("difference in config: IsKnown(%s) -> base: %v | compare %v", key, base, compare)
	}
	return base
}

// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
// Note that it returns the keys lowercased.
func (t *teeConfig) GetKnownKeysLowercased() map[string]interface{} {
	base := t.baseline.GetKnownKeysLowercased()
	compare := t.compare.GetKnownKeysLowercased()
	compareResult("", "GetKnownKeysLowercased", base, compare)
	return base
}

// BuildSchema constructs the default schema and marks the config as ready for use
func (t *teeConfig) BuildSchema() {
	t.baseline.BuildSchema()
	t.compare.BuildSchema()
}

// ParseEnvAsStringSlice registers a transformer function to parse an an environment variables as a []string.
func (t *teeConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	t.baseline.ParseEnvAsStringSlice(key, fn)
	t.compare.ParseEnvAsStringSlice(key, fn)
}

// ParseEnvAsMapStringInterface registers a transformer function to parse an an environment variables as a
// map[string]interface{}.
func (t *teeConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	t.baseline.ParseEnvAsMapStringInterface(key, fn)
	t.compare.ParseEnvAsMapStringInterface(key, fn)
}

// ParseEnvAsSliceMapString registers a transformer function to parse an an environment variables as a []map[string]string.
func (t *teeConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	t.baseline.ParseEnvAsSliceMapString(key, fn)
	t.compare.ParseEnvAsSliceMapString(key, fn)
}

// ParseEnvAsSlice registers a transformer function to parse an an environment variables as a
// []interface{}.
func (t *teeConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	t.baseline.ParseEnvAsSlice(key, fn)
	t.compare.ParseEnvAsSlice(key, fn)
}

// IsSet wraps Viper for concurrent access
func (t *teeConfig) IsSet(key string) bool {
	base := t.baseline.IsSet(key)
	compare := t.compare.IsSet(key)
	if base != compare {
		log.Warnf("difference in config: IsSet(%s) -> base: %v | compare %v", key, base, compare)
	}
	return base
}

func (t *teeConfig) AllKeysLowercased() []string {
	base := t.baseline.AllKeysLowercased()
	compare := t.compare.AllKeysLowercased()
	if !reflect.DeepEqual(base, compare) {
		log.Warnf("difference in config: allkeyslowercased() -> base len: %d | compare len: %d", len(base), len(compare))

		i := 0
		j := 0
		for i < len(base) && j < len(compare) {
			if base[i] != compare[j] {
				i++
				j++
				continue
			}

			log.Warnf("difference in config: allkeyslowercased() -> base[%d]: %v | compare[%d]: %v", i, base[i], j, compare[j])
			if strings.Compare(base[i], compare[j]) == -1 {
				i++
			} else {
				j++
			}
		}
	}
	return base
}

func compareResult(key, method string, base, compare interface{}) interface{} {
	if !reflect.DeepEqual(base, compare) {
		_, file, line, _ := runtime.Caller(2)
		fileParts := strings.Split(file, "DataDog/datadog-agent/")
		log.Warnf("difference in config: %s(%s) -> base: %v | compare %v from %s:%d", method, key, base, compare, fileParts[len(fileParts)-1], line)
	}
	return compare
}

// Get wraps Viper for concurrent access
func (t *teeConfig) Get(key string) interface{} {
	base := t.baseline.Get(key)
	compare := t.compare.Get(key)
	return compareResult(key, "Get", base, compare)
}

// GetAllSources returns the value of a key for each source
func (t *teeConfig) GetAllSources(key string) []model.ValueWithSource {
	base := t.baseline.GetAllSources(key)
	compare := t.compare.GetAllSources(key)
	compareResult(key, "GetAllSources", base, compare)
	return base
}

// GetString wraps Viper for concurrent access
func (t *teeConfig) GetString(key string) string {
	base := t.baseline.GetString(key)
	compare := t.compare.GetString(key)
	compareResult(key, "GetString", base, compare)
	return base

}

// GetBool wraps Viper for concurrent access
func (t *teeConfig) GetBool(key string) bool {
	base := t.baseline.GetBool(key)
	compare := t.compare.GetBool(key)
	compareResult(key, "GetBool", base, compare)
	return base

}

// GetInt wraps Viper for concurrent access
func (t *teeConfig) GetInt(key string) int {
	base := t.baseline.GetInt(key)
	compare := t.compare.GetInt(key)
	compareResult(key, "GetInt", base, compare)
	return base

}

// GetInt32 wraps Viper for concurrent access
func (t *teeConfig) GetInt32(key string) int32 {
	base := t.baseline.GetInt32(key)
	compare := t.compare.GetInt32(key)
	compareResult(key, "GetInt32", base, compare)
	return base

}

// GetInt64 wraps Viper for concurrent access
func (t *teeConfig) GetInt64(key string) int64 {
	base := t.baseline.GetInt64(key)
	compare := t.compare.GetInt64(key)
	compareResult(key, "GetInt64", base, compare)
	return base

}

// GetFloat64 wraps Viper for concurrent access
func (t *teeConfig) GetFloat64(key string) float64 {
	base := t.baseline.GetFloat64(key)
	compare := t.compare.GetFloat64(key)
	compareResult(key, "GetFloat64", base, compare)
	return base

}

// GetDuration wraps Viper for concurrent access
func (t *teeConfig) GetDuration(key string) time.Duration {
	base := t.baseline.GetDuration(key)
	compare := t.compare.GetDuration(key)
	compareResult(key, "GetDuration", base, compare)
	return base

}

// GetStringSlice wraps Viper for concurrent access
func (t *teeConfig) GetStringSlice(key string) []string {
	base := t.baseline.GetStringSlice(key)
	compare := t.compare.GetStringSlice(key)
	compareResult(key, "GetStringSlice", base, compare)
	return base

}

// GetFloat64Slice wraps Viper for concurrent access
func (t *teeConfig) GetFloat64Slice(key string) []float64 {
	base := t.baseline.GetFloat64Slice(key)
	compare := t.compare.GetFloat64Slice(key)
	compareResult(key, "GetFloat64Slice", base, compare)
	return base

}

// GetStringMap wraps Viper for concurrent access
func (t *teeConfig) GetStringMap(key string) map[string]interface{} {
	base := t.baseline.GetStringMap(key)
	compare := t.compare.GetStringMap(key)
	compareResult(key, "GetStringMap", base, compare)
	return base

}

// GetStringMapString wraps Viper for concurrent access
func (t *teeConfig) GetStringMapString(key string) map[string]string {
	base := t.baseline.GetStringMapString(key)
	compare := t.compare.GetStringMapString(key)
	compareResult(key, "GetStringMapString", base, compare)
	return base

}

// GetStringMapStringSlice wraps Viper for concurrent access
func (t *teeConfig) GetStringMapStringSlice(key string) map[string][]string {
	base := t.baseline.GetStringMapStringSlice(key)
	compare := t.compare.GetStringMapStringSlice(key)
	compareResult(key, "GetStringMapStringSlice", base, compare)
	return base

}

// GetSizeInBytes wraps Viper for concurrent access
func (t *teeConfig) GetSizeInBytes(key string) uint {
	base := t.baseline.GetSizeInBytes(key)
	compare := t.compare.GetSizeInBytes(key)
	compareResult(key, "GetSizeInBytes", base, compare)
	return base

}

// GetSource wraps Viper for concurrent access
func (t *teeConfig) GetSource(key string) model.Source {
	base := t.baseline.GetSource(key)
	compare := t.compare.GetSource(key)
	compareResult(key, "GetSource", base, compare)
	return base

}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (t *teeConfig) SetEnvPrefix(in string) {
	t.baseline.SetEnvPrefix(in)
	t.compare.SetEnvPrefix(in)
}

// BindEnv wraps Viper for concurrent access, and adds tracking of the configurable env vars
func (t *teeConfig) BindEnv(key string, envvars ...string) {
	t.baseline.BindEnv(key, envvars...)
	t.compare.BindEnv(key, envvars...)
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (t *teeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	t.baseline.SetEnvKeyReplacer(r)
	t.compare.SetEnvKeyReplacer(r)
}

// UnmarshalKey wraps Viper for concurrent access
func (t *teeConfig) UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error {
	return t.baseline.UnmarshalKey(key, rawVal, opts...)
}

// ReadInConfig wraps Viper for concurrent access
func (t *teeConfig) ReadInConfig() error {
	err1 := t.baseline.ReadInConfig()
	err2 := t.compare.ReadInConfig()
	if (err1 == nil) != (err2 == nil) {
		log.Warnf("difference in config: ReadInConfig() -> base error: %v | compare error: %v", err1, err2)
	}
	return err1
}

// ReadConfig wraps Viper for concurrent access
func (t *teeConfig) ReadConfig(in io.Reader) error {
	err1 := t.baseline.ReadConfig(in)
	err2 := t.compare.ReadConfig(in)
	if (err1 != nil && err2 == nil) || (err1 == nil && err2 != nil) {
		log.Warnf("difference in config: ReadConfig() -> base error: %v | compare error: %v", err1, err2)
	}
	return err1
}

// MergeConfig wraps Viper for concurrent access
func (t *teeConfig) MergeConfig(in io.Reader) error {
	err1 := t.baseline.MergeConfig(in)
	err2 := t.compare.MergeConfig(in)
	if (err1 != nil && err2 == nil) || (err1 == nil && err2 != nil) {
		log.Warnf("difference in config: MergeConfig() -> base error: %v | compare error: %v", err1, err2)
	}
	return err1
}

// MergeFleetPolicy merges the configuration from the reader given with an existing config
// it overrides the existing values with the new ones in the FleetPolicies source, and updates the main config
// according to sources priority order.
//
// Note: this should only be called at startup, as notifiers won't receive a notification when this loads
func (t *teeConfig) MergeFleetPolicy(configPath string) error {
	err1 := t.baseline.MergeFleetPolicy(configPath)
	err2 := t.compare.MergeFleetPolicy(configPath)
	if (err1 != nil && err2 == nil) || (err1 == nil && err2 != nil) {
		log.Warnf("difference in config: MergeFleetPolicy(%s) -> base error: %v | compare error: %v", configPath, err1, err2)
	}
	return err1
}

// AllSettings wraps Viper for concurrent access
func (t *teeConfig) AllSettings() map[string]interface{} {
	base := t.baseline.AllSettings()
	compare := t.compare.AllSettings()
	if !reflect.DeepEqual(base, compare) {
		log.Warnf("difference in config: AllSettings() -> base len: %v | compare len: %v", len(base), len(compare))
		for key := range base {
			if _, ok := compare[key]; !ok {
				log.Warnf("\titem %s missing from compare", key)
				continue
			}
			if !reflect.DeepEqual(base[key], compare[key]) {
				log.Warnf("\titem %s: %v | %v", key, base[key], compare[key])
			}
			log.Flush()
		}
	}
	return base

}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (t *teeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	base := t.baseline.AllSettingsWithoutDefault()
	compare := t.compare.AllSettingsWithoutDefault()
	compareResult("", "AllSettingsWithoutDefault", base, compare)
	return base

}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (t *teeConfig) AllSettingsBySource() map[model.Source]interface{} {
	base := t.baseline.AllSettingsBySource()
	compare := t.compare.AllSettingsBySource()
	compareResult("", "AllSettingsBySource", base, compare)
	return base

}

// AddConfigPath wraps Viper for concurrent access
func (t *teeConfig) AddConfigPath(in string) {
	t.baseline.AddConfigPath(in)
	t.compare.AddConfigPath(in)
}

// AddExtraConfigPaths allows adding additional configuration files
// which will be merged into the main configuration during the ReadInConfig call.
// Configuration files are merged sequentially. If a key already exists and the foreign value type matches the existing one, the foreign value overrides it.
// If both the existing value and the new value are nested configurations, they are merged recursively following the same principles.
func (t *teeConfig) AddExtraConfigPaths(ins []string) error {
	err1 := t.baseline.AddExtraConfigPaths(ins)
	err2 := t.compare.AddExtraConfigPaths(ins)
	if (err1 != nil && err2 == nil) || (err1 == nil && err2 != nil) {
		log.Warnf("difference in config: AddExtraConfigPaths(%s) -> base error: %v | compare error: %v", ins, err1, err2)
	}
	return err1
}

// SetConfigName wraps Viper for concurrent access
func (t *teeConfig) SetConfigName(in string) {
	t.baseline.SetConfigName(in)
	t.compare.SetConfigName(in)
}

// SetConfigFile wraps Viper for concurrent access
func (t *teeConfig) SetConfigFile(in string) {
	t.baseline.SetConfigFile(in)
	t.compare.SetConfigFile(in)
}

// SetConfigType wraps Viper for concurrent access
func (t *teeConfig) SetConfigType(in string) {
	t.baseline.SetConfigType(in)
	t.compare.SetConfigType(in)
}

// ConfigFileUsed wraps Viper for concurrent access
func (t *teeConfig) ConfigFileUsed() string {
	base := t.baseline.ConfigFileUsed()
	compare := t.compare.ConfigFileUsed()
	compareResult("", "ConfigFileUsed", base, compare)
	return base

}

//func (t *teeConfig) SetTypeByDefaultValue(in bool) {
//	t.baseline.SetTypeByDefaultValue(in)
//	t.compare.SetTypeByDefaultValue(in)
//}

// GetEnvVars implements the Config interface
func (t *teeConfig) GetEnvVars() []string {
	base := t.baseline.GetEnvVars()
	compare := t.compare.GetEnvVars()
	compareResult("", "GetEnvVars", base, compare)
	return base

}

// BindEnvAndSetDefault implements the Config interface
func (t *teeConfig) BindEnvAndSetDefault(key string, val interface{}, env ...string) {
	t.baseline.BindEnvAndSetDefault(key, val, env...)
	t.compare.BindEnvAndSetDefault(key, val, env...)
}

func (t *teeConfig) Warnings() *model.Warnings {
	return nil
}

func (t *teeConfig) Object() model.Reader {
	return t.baseline
}

// Stringify stringifies the config
func (t *teeConfig) Stringify(source model.Source) string {
	return t.baseline.Stringify(source)
}

func (t *teeConfig) GetProxies() *model.Proxy {
	base := t.baseline.GetProxies()
	compare := t.compare.GetProxies()
	compareResult("", "GetProxies", base, compare)
	return base
}

func (t *teeConfig) ExtraConfigFilesUsed() []string {
	base := t.baseline.ExtraConfigFilesUsed()
	compare := t.compare.ExtraConfigFilesUsed()
	compareResult("", "ExtraConfigFilesUsed", base, compare)
	return base
}
