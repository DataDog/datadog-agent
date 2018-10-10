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

// safeConfig wraps viper with a safety lock
type safeConfig struct {
	*viper.Viper
	sync.RWMutex
}

// Set is wrapped for concurrent access
func (c *safeConfig) Set(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.Set(key, value)
}

// SetDefault is wrapped for concurrent access
func (c *safeConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetDefault(key, value)
}

// SetFs is wrapped for concurrent access
func (c *safeConfig) SetFs(fs afero.Fs) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetFs(fs)
}

// IsSet is wrapped for concurrent access
func (c *safeConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.IsSet(key)
}

// Get is wrapped for concurrent access
func (c *safeConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.Get(key)
}

// GetString is wrapped for concurrent access
func (c *safeConfig) GetString(key string) string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetString(key)
}

// GetBool is wrapped for concurrent access
func (c *safeConfig) GetBool(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetBool(key)
}

// GetInt is wrapped for concurrent access
func (c *safeConfig) GetInt(key string) int {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetInt(key)
}

// GetInt64 is wrapped for concurrent access
func (c *safeConfig) GetInt64(key string) int64 {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetInt64(key)
}

// GetFloat64 is wrapped for concurrent access
func (c *safeConfig) GetFloat64(key string) float64 {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetFloat64(key)
}

// GetTime is wrapped for concurrent access
func (c *safeConfig) GetTime(key string) time.Time {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetTime(key)
}

// GetDuration is wrapped for concurrent access
func (c *safeConfig) GetDuration(key string) time.Duration {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetDuration(key)
}

// GetStringSlice is wrapped for concurrent access
func (c *safeConfig) GetStringSlice(key string) []string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringSlice(key)
}

// GetStringMap is wrapped for concurrent access
func (c *safeConfig) GetStringMap(key string) map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMap(key)
}

// GetStringMapString is wrapped for concurrent access
func (c *safeConfig) GetStringMapString(key string) map[string]string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMapString(key)
}

// GetStringMapStringSlice is wrapped for concurrent access
func (c *safeConfig) GetStringMapStringSlice(key string) map[string][]string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetStringMapStringSlice(key)
}

// GetSizeInBytes is wrapped for concurrent access
func (c *safeConfig) GetSizeInBytes(key string) uint {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.GetSizeInBytes(key)
}

// SetEnvPrefix is wrapped for concurrent access
func (c *safeConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvPrefix(in)
}

// BindEnv is wrapped for concurrent access
func (c *safeConfig) BindEnv(input ...string) error {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.BindEnv(input...)
}

// SetEnvKeyReplacer is wrapped for concurrent access
func (c *safeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.RLock()
	defer c.RUnlock()
	c.Viper.SetEnvKeyReplacer(r)
}

// UnmarshalKey is wrapped for concurrent access
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.UnmarshalKey(key, rawVal)
}

// Unmarshal is wrapped for concurrent access
func (c *safeConfig) Unmarshal(rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.Unmarshal(rawVal)
}

// UnmarshalExact is wrapped for concurrent access
func (c *safeConfig) UnmarshalExact(rawVal interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.UnmarshalExact(rawVal)
}

// ReadInConfig is wrapped for concurrent access
func (c *safeConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ReadInConfig()
}

// ReadConfig is wrapped for concurrent access
func (c *safeConfig) ReadConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ReadConfig(in)
}

// MergeConfig is wrapped for concurrent access
func (c *safeConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.MergeConfig(in)
}

// AllSettings is wrapped for concurrent access
func (c *safeConfig) AllSettings() map[string]interface{} {
	c.Lock()
	defer c.Unlock()
	return c.Viper.AllSettings()
}

// AddConfigPath is wrapped for concurrent access
func (c *safeConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.AddConfigPath(in)
}

// SetConfigName is wrapped for concurrent access
func (c *safeConfig) SetConfigName(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigName(in)
}

// SetConfigFile is wrapped for concurrent access
func (c *safeConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigFile(in)
}

// SetConfigType is wrapped for concurrent access
func (c *safeConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetConfigType(in)
}

// ConfigFileUsed is wrapped for concurrent access
func (c *safeConfig) ConfigFileUsed() string {
	c.Lock()
	defer c.Unlock()
	return c.Viper.ConfigFileUsed()
}

// BindPFlag is wrapped for concurrent access
func (c *safeConfig) BindPFlag(key string, flag *pflag.Flag) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.BindPFlag(key, flag)
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
