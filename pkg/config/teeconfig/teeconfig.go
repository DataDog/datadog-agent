// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package teeconfig is a tee of two configs that writes to both but reads from only one
package teeconfig

import (
	"io"
	"strings"
	"time"

	"github.com/DataDog/viper"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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
	return t.baseline.IsKnown(key)
}

// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
// Note that it returns the keys lowercased.
func (t *teeConfig) GetKnownKeysLowercased() map[string]interface{} {
	return t.baseline.GetKnownKeysLowercased()
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
	return t.baseline.IsSet(key)
}

func (t *teeConfig) AllKeysLowercased() []string {
	return t.baseline.AllKeysLowercased()
}

// Get wraps Viper for concurrent access
func (t *teeConfig) Get(key string) interface{} {
	return t.baseline.Get(key)
}

// GetAllSources returns the value of a key for each source
func (t *teeConfig) GetAllSources(key string) []model.ValueWithSource {
	return t.baseline.GetAllSources(key)
}

// GetString wraps Viper for concurrent access
func (t *teeConfig) GetString(key string) string {
	return t.baseline.GetString(key)
}

// GetBool wraps Viper for concurrent access
func (t *teeConfig) GetBool(key string) bool {
	return t.baseline.GetBool(key)
}

// GetInt wraps Viper for concurrent access
func (t *teeConfig) GetInt(key string) int {
	return t.baseline.GetInt(key)
}

// GetInt32 wraps Viper for concurrent access
func (t *teeConfig) GetInt32(key string) int32 {
	return t.baseline.GetInt32(key)
}

// GetInt64 wraps Viper for concurrent access
func (t *teeConfig) GetInt64(key string) int64 {
	return t.baseline.GetInt64(key)
}

// GetFloat64 wraps Viper for concurrent access
func (t *teeConfig) GetFloat64(key string) float64 {
	return t.baseline.GetFloat64(key)
}

// GetTime wraps Viper for concurrent access
func (t *teeConfig) GetTime(key string) time.Time {
	return t.baseline.GetTime(key)
}

// GetDuration wraps Viper for concurrent access
func (t *teeConfig) GetDuration(key string) time.Duration {
	return t.baseline.GetDuration(key)
}

// GetStringSlice wraps Viper for concurrent access
func (t *teeConfig) GetStringSlice(key string) []string {
	return t.baseline.GetStringSlice(key)
}

// GetFloat64SliceE loads a key as a []float64
func (t *teeConfig) GetFloat64SliceE(key string) ([]float64, error) {
	return t.baseline.GetFloat64SliceE(key)
}

// GetStringMap wraps Viper for concurrent access
func (t *teeConfig) GetStringMap(key string) map[string]interface{} {
	return t.baseline.GetStringMap(key)
}

// GetStringMapString wraps Viper for concurrent access
func (t *teeConfig) GetStringMapString(key string) map[string]string {
	return t.baseline.GetStringMapString(key)
}

// GetStringMapStringSlice wraps Viper for concurrent access
func (t *teeConfig) GetStringMapStringSlice(key string) map[string][]string {
	return t.baseline.GetStringMapStringSlice(key)
}

// GetSizeInBytes wraps Viper for concurrent access
func (t *teeConfig) GetSizeInBytes(key string) uint {
	return t.baseline.GetSizeInBytes(key)
}

// GetSource wraps Viper for concurrent access
func (t *teeConfig) GetSource(key string) model.Source {
	return t.baseline.GetSource(key)
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
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// ReadConfig wraps Viper for concurrent access
func (t *teeConfig) ReadConfig(in io.Reader) error {
	err1 := t.baseline.ReadConfig(in)
	err2 := t.compare.ReadConfig(in)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil

}

// MergeConfig wraps Viper for concurrent access
func (t *teeConfig) MergeConfig(in io.Reader) error {
	err1 := t.baseline.MergeConfig(in)
	err2 := t.compare.MergeConfig(in)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil

}

// MergeFleetPolicy merges the configuration from the reader given with an existing config
// it overrides the existing values with the new ones in the FleetPolicies source, and updates the main config
// according to sources priority order.
//
// Note: this should only be called at startup, as notifiers won't receive a notification when this loads
func (t *teeConfig) MergeFleetPolicy(configPath string) error {
	err1 := t.baseline.MergeFleetPolicy(configPath)
	err2 := t.compare.MergeFleetPolicy(configPath)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// MergeConfigMap merges the configuration from the map given with an existing config.
// Note that the map given may be modified.
func (t *teeConfig) MergeConfigMap(cfg map[string]any) error {
	err1 := t.baseline.MergeConfigMap(cfg)
	err2 := t.compare.MergeConfigMap(cfg)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// AllSettings wraps Viper for concurrent access
func (t *teeConfig) AllSettings() map[string]interface{} {
	return t.baseline.AllSettings()
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (t *teeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	return t.baseline.AllSettingsWithoutDefault()
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (t *teeConfig) AllSettingsBySource() map[model.Source]interface{} {
	return t.baseline.AllSettingsBySource()
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
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
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
	return t.baseline.ConfigFileUsed()
}

//func (t *teeConfig) SetTypeByDefaultValue(in bool) {
//	t.baseline.SetTypeByDefaultValue(in)
//	t.compare.SetTypeByDefaultValue(in)
//}

// GetEnvVars implements the Config interface
func (t *teeConfig) GetEnvVars() []string {
	return t.baseline.GetEnvVars()
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

// CopyConfig copies the given config to the receiver config. This should only be used in tests as replacing
// the global config reference is unsafe.
func (t *teeConfig) CopyConfig(cfg model.Config) {
	t.baseline.CopyConfig(cfg)
	t.compare.CopyConfig(cfg)
}

func (t *teeConfig) GetProxies() *model.Proxy {
	return t.baseline.GetProxies()
}

func (t *teeConfig) ExtraConfigFilesUsed() []string {
	return t.baseline.ExtraConfigFilesUsed()
}
