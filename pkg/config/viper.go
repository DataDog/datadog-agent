// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// safeConfig implements Config:
// - wraps viper with a safety lock
// - implements the additional DDHelpers
type safeConfig struct {
	*viper.Viper
	sync.RWMutex
	envPrefix     string
	configEnvVars []string
}

// Set wraps Viper for concurrent access
func (c *safeConfig) Set(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.Set(key, value)
}

// SetDefault wraps Viper for concurrent access
func (c *safeConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetDefault(key, value)
}

// SetFs wraps Viper for concurrent access
func (c *safeConfig) SetFs(fs afero.Fs) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetFs(fs)
}

// IsSet wraps Viper for concurrent access
func (c *safeConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.IsSet(key)
}

// Get wraps Viper for concurrent access
func (c *safeConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.Get(key)
}

// GetString wraps Viper for concurrent access
func (c *safeConfig) GetString(key string) string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetString(key)
}

// GetBool wraps Viper for concurrent access
func (c *safeConfig) GetBool(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetBool(key)
}

// GetInt wraps Viper for concurrent access
func (c *safeConfig) GetInt(key string) int {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetInt(key)
}

// GetInt64 wraps Viper for concurrent access
func (c *safeConfig) GetInt64(key string) int64 {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetInt64(key)
}

// GetFloat64 wraps Viper for concurrent access
func (c *safeConfig) GetFloat64(key string) float64 {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetFloat64(key)
}

// GetTime wraps Viper for concurrent access
func (c *safeConfig) GetTime(key string) time.Time {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetTime(key)
}

// GetDuration wraps Viper for concurrent access
func (c *safeConfig) GetDuration(key string) time.Duration {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetDuration(key)
}

// GetStringSlice wraps Viper for concurrent access
func (c *safeConfig) GetStringSlice(key string) []string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringSlice(key)
}

// GetStringMap wraps Viper for concurrent access
func (c *safeConfig) GetStringMap(key string) map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMap(key)
}

// GetStringMapString wraps Viper for concurrent access
func (c *safeConfig) GetStringMapString(key string) map[string]string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMapString(key)
}

// GetStringMapStringSlice wraps Viper for concurrent access
func (c *safeConfig) GetStringMapStringSlice(key string) map[string][]string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMapStringSlice(key)
}

// GetSizeInBytes wraps Viper for concurrent access
func (c *safeConfig) GetSizeInBytes(key string) uint {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetSizeInBytes(key)
}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (c *safeConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvPrefix(in)
	c.envPrefix = in
}

// BindEnv wraps Viper for concurrent access, and adds tracking of the configurable env vars
func (c *safeConfig) BindEnv(input ...string) error {
	c.Lock()
	defer c.Unlock()
	if len(input) == 1 {
		// FIXME: for the purposes of GetEnvVars implementation, we only track env var keys
		// that are interpolated by viper from the config option key name
		key := input[0]
		if !strings.Contains(key, "_key") {
			// `_key`-suffixed env vars are sensitive: don't track them
			envVarName := strings.Join([]string{c.envPrefix, strings.ToUpper(key)}, "_")
			c.configEnvVars = append(c.configEnvVars, envVarName)
		}
	}
	return c.Viper.BindEnv(input...)
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (c *safeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.RLock()
	defer c.RUnlock()
	c.Viper.SetEnvKeyReplacer(r)
}

// UnmarshalKey wraps Viper for concurrent access
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.UnmarshalKey(key, rawVal)
}

// Unmarshal wraps Viper for concurrent access
func (c *safeConfig) Unmarshal(rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.Unmarshal(rawVal)
}

// UnmarshalExact wraps Viper for concurrent access
func (c *safeConfig) UnmarshalExact(rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.UnmarshalExact(rawVal)
}

// ReadInConfig wraps Viper for concurrent access
func (c *safeConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ReadInConfig()
}

// ReadConfig wraps Viper for concurrent access
func (c *safeConfig) ReadConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ReadConfig(in)
}

// MergeConfig wraps Viper for concurrent access
func (c *safeConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.MergeConfig(in)
}

// AllSettings wraps Viper for concurrent access
func (c *safeConfig) AllSettings() map[string]interface{} {
	c.Lock()
	defer c.Unlock()
	return c.Viper.AllSettings()
}

// AddConfigPath wraps Viper for concurrent access
func (c *safeConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.AddConfigPath(in)
}

// SetConfigName wraps Viper for concurrent access
func (c *safeConfig) SetConfigName(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigName(in)
}

// SetConfigFile wraps Viper for concurrent access
func (c *safeConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigFile(in)
}

// SetConfigType wraps Viper for concurrent access
func (c *safeConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigType(in)
}

// ConfigFileUsed wraps Viper for concurrent access
func (c *safeConfig) ConfigFileUsed() string {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ConfigFileUsed()
}

// BindPFlag wraps Viper for concurrent access
func (c *safeConfig) BindPFlag(key string, flag *pflag.Flag) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.BindPFlag(key, flag)
}

// GetEnvVars implements the Config interface
func (c *safeConfig) GetEnvVars() []string {
	return c.configEnvVars
}

// BindEnvAndSetDefault implements the Config interface
func (c *safeConfig) BindEnvAndSetDefault(key string, val interface{}) {
	c.SetDefault(key, val)
	c.BindEnv(key)
}

// NewConfig returns a new Config object.
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) Config {
	config := safeConfig{
		Viper: viper.New(),
	}
	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)
	config.SetTypeByDefaultValue(true)
	return &config
}
