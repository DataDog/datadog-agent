// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/viper"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
)

// safeConfig implements Config:
// - wraps viper with a safety lock
// - implements the additional DDHelpers
type safeConfig struct {
	*viper.Viper
	sync.RWMutex
	envPrefix      string
	envKeyReplacer *strings.Replacer

	// Proxy settings
	proxiesOnce sync.Once
	proxies     *Proxy

	// configEnvVars is the set of env vars that are consulted for
	// configuration values.
	configEnvVars map[string]struct{}
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

// SetKnown adds a key to the set of known valid config keys
func (c *safeConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetKnown(key)
}

// IsKnown adds a key to the set of known valid config keys
func (c *safeConfig) IsKnown(key string) bool {
	c.Lock()
	defer c.Unlock()

	keys := c.Viper.GetKnownKeys()
	_, ok := keys[key]
	return ok
}

// GetKnownKeys returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
func (c *safeConfig) GetKnownKeys() map[string]interface{} {
	c.Lock()
	defer c.Unlock()

	// GetKnownKeys returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.GetKnownKeys()
}

// SetEnvKeyTransformer allows defining a transformer function which decides
// how an environment variables value gets assigned to key.
func (c *safeConfig) SetEnvKeyTransformer(key string, fn func(string) interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvKeyTransformer(key, fn)
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

// IsSectionSet checks if a section is set by checking if either it
// or any of its subkeys is set.
func (c *safeConfig) IsSectionSet(section string) bool {
	// The section is considered set if any of the keys
	// inside it is set.
	// This is needed when keys within the section
	// are set through env variables.

	// Add trailing . to make sure we don't take into account unrelated
	// settings, eg. IsSectionSet("section") shouldn't return true
	// if "section_key" is set.
	sectionPrefix := section + "."

	for _, key := range c.AllKeys() {
		if strings.HasPrefix(key, sectionPrefix) && c.IsSet(key) {
			return true
		}
	}

	// Is none of the keys are set, the section is still considered as set
	// if it has been explicitly set in the config.
	return c.IsSet(section)
}

func (c *safeConfig) AllKeys() []string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.AllKeys()
}

// Get wraps Viper for concurrent access
func (c *safeConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetString wraps Viper for concurrent access
func (c *safeConfig) GetString(key string) string {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetStringE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetBool wraps Viper for concurrent access
func (c *safeConfig) GetBool(key string) bool {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetBoolE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetInt wraps Viper for concurrent access
func (c *safeConfig) GetInt(key string) int {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetIntE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetInt32 wraps Viper for concurrent access
func (c *safeConfig) GetInt32(key string) int32 {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetInt32E(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetInt64 wraps Viper for concurrent access
func (c *safeConfig) GetInt64(key string) int64 {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetInt64E(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetFloat64 wraps Viper for concurrent access
func (c *safeConfig) GetFloat64(key string) float64 {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetFloat64E(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetTime wraps Viper for concurrent access
func (c *safeConfig) GetTime(key string) time.Time {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetTimeE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetDuration wraps Viper for concurrent access
func (c *safeConfig) GetDuration(key string) time.Duration {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetDurationE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetStringSlice wraps Viper for concurrent access
func (c *safeConfig) GetStringSlice(key string) []string {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetStringSliceE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetFloat64SliceE loads a key as a []float64
func (c *safeConfig) GetFloat64SliceE(key string) ([]float64, error) {
	c.RLock()
	defer c.RUnlock()

	// We're using GetStringSlice because viper can only parse list of string from env variables
	list, err := c.Viper.GetStringSliceE(key)
	if err != nil {
		return nil, fmt.Errorf("'%v' is not a list", key)
	}

	res := []float64{}
	for _, item := range list {
		nb, err := strconv.ParseFloat(item, 64)
		if err != nil {
			return nil, fmt.Errorf("value '%v' from '%v' is not a float64", item, key)
		}
		res = append(res, nb)
	}
	return res, nil
}

// GetStringMap wraps Viper for concurrent access
func (c *safeConfig) GetStringMap(key string) map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetStringMapE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetStringMapString wraps Viper for concurrent access
func (c *safeConfig) GetStringMapString(key string) map[string]string {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetStringMapStringE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetStringMapStringSlice wraps Viper for concurrent access
func (c *safeConfig) GetStringMapStringSlice(key string) map[string][]string {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetStringMapStringSliceE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetSizeInBytes wraps Viper for concurrent access
func (c *safeConfig) GetSizeInBytes(key string) uint {
	c.RLock()
	defer c.RUnlock()
	val, err := c.Viper.GetSizeInBytesE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (c *safeConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvPrefix(in)
	c.envPrefix = in
}

// mergeWithEnvPrefix derives the environment variable that Viper will use for a given key.
func (c *safeConfig) mergeWithEnvPrefix(key string) string {
	return strings.Join([]string{c.envPrefix, strings.ToUpper(key)}, "_")
}

// BindEnv wraps Viper for concurrent access, and adds tracking of the configurable env vars
func (c *safeConfig) BindEnv(input ...string) {
	c.Lock()
	defer c.Unlock()
	var envKeys []string

	// If one input is given, viper derives an env key from it; otherwise, all inputs after
	// the first are literal env vars.
	if len(input) == 1 {
		envKeys = []string{c.mergeWithEnvPrefix(input[0])}
	} else {
		envKeys = input[1:]
	}

	for _, key := range envKeys {
		// apply EnvKeyReplacer to each key
		if c.envKeyReplacer != nil {
			key = c.envKeyReplacer.Replace(key)
		}
		c.configEnvVars[key] = struct{}{}
	}

	_ = c.Viper.BindEnv(input...)
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (c *safeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.RLock()
	defer c.RUnlock()
	c.Viper.SetEnvKeyReplacer(r)
	c.envKeyReplacer = r
}

// UnmarshalKey wraps Viper for concurrent access
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.UnmarshalKey(key, rawVal, opts...)
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
	err := c.Viper.ReadInConfig()
	c.Unlock()
	return err
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

// MergeConfigOverride wraps Viper for concurrent access
func (c *safeConfig) MergeConfigOverride(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.MergeConfigOverride(in)
}

// AllSettings wraps Viper for concurrent access
func (c *safeConfig) AllSettings() map[string]interface{} {
	c.Lock()
	defer c.Unlock()

	// AllSettings returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettings()
}

// AllSettingsWithoutDefault wraps Viper for concurrent access
func (c *safeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.Lock()
	defer c.Unlock()

	// AllSettingsWithoutDefault returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettingsWithoutDefault()
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
	vars := make([]string, 0, len(c.configEnvVars))
	for v := range c.configEnvVars {
		vars = append(vars, v)
	}
	return vars
}

// BindEnvAndSetDefault implements the Config interface
func (c *safeConfig) BindEnvAndSetDefault(key string, val interface{}, env ...string) {
	c.SetDefault(key, val)
	c.BindEnv(append([]string{key}, env...)...) //nolint:errcheck
}

// NewConfig returns a new Config object.
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) Config {
	config := safeConfig{
		Viper:         viper.New(),
		configEnvVars: map[string]struct{}{},
	}
	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)
	config.SetTypeByDefaultValue(true)
	return &config
}

// CopyConfig copies the internal config to the current config. This should only be used in tests as replacing
// the global config reference is unsafe.
func (c *safeConfig) CopyConfig(cfg Config) {
	c.Lock()
	defer c.Unlock()

	if cfg, ok := cfg.(*safeConfig); ok {
		c.Viper = cfg.Viper
		c.envPrefix = cfg.envPrefix
		c.envKeyReplacer = cfg.envKeyReplacer
		c.configEnvVars = cfg.configEnvVars
		return
	}
	panic("Replacement config must be an instance of safeConfig")
}

// GetProxies returns the proxy settings from the configuration
func (c *safeConfig) GetProxies() *Proxy {
	c.proxiesOnce.Do(func() {
		if c.GetBool("fips.enabled") {
			return
		}
		if !c.IsSet("proxy.http") && !c.IsSet("proxy.https") && !c.IsSet("proxy.no_proxy") {
			return
		}
		p := &Proxy{
			HTTP:    c.GetString("proxy.http"),
			HTTPS:   c.GetString("proxy.https"),
			NoProxy: c.GetStringSlice("proxy.no_proxy"),
		}

		c.proxies = p
	})
	return c.proxies
}
