// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/viper"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Source stores what edits a setting as a string
type Source string

// Declare every known Source
const (
	SourceDefault            Source = "default"
	SourceUnknown            Source = "unknown"
	SourceFile               Source = "file"
	SourceEnvVar             Source = "environment-variable"
	SourceAgentRuntime       Source = "agent-runtime"
	SourceLocalConfigProcess Source = "local-config-process"
	SourceRC                 Source = "remote-config"
	SourceCLI                Source = "cli"
)

// sources list the known sources, following the order of hierarchy between them
var sources = []Source{SourceDefault, SourceUnknown, SourceFile, SourceEnvVar, SourceAgentRuntime, SourceLocalConfigProcess, SourceRC, SourceCLI}

// ValueWithSource is a tuple for a source and a value, not necessarily the applied value in the main config
type ValueWithSource struct {
	Source Source
	Value  interface{}
}

// String casts Source into a string
func (s Source) String() string {
	// Safeguard: if we don't know the Source, we assume SourceUnknown
	if s == "" {
		return string(SourceUnknown)
	}
	return string(s)
}

// safeConfig implements Config:
// - wraps viper with a safety lock
// - implements the additional DDHelpers
type safeConfig struct {
	*viper.Viper
	configSources map[Source]*viper.Viper
	sync.RWMutex
	envPrefix      string
	envKeyReplacer *strings.Replacer

	notificationReceivers []NotificationReceiver

	// Proxy settings
	proxiesOnce sync.Once
	proxies     *Proxy

	// configEnvVars is the set of env vars that are consulted for
	// configuration values.
	configEnvVars map[string]struct{}

	// keys that have been used but are unknown
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}
}

// OnUpdate adds a callback to the list receivers to be called each time a value is changed in the configuration
// by a call to the 'Set' method.
// Callbacks are only called if the value is effectively changed.
func (c *safeConfig) OnUpdate(callback NotificationReceiver) {
	c.Lock()
	defer c.Unlock()
	c.notificationReceivers = append(c.notificationReceivers, callback)
}

// Set wraps Viper for concurrent access
func (c *safeConfig) Set(key string, newValue interface{}, source Source) {
	if source == SourceDefault {
		c.SetDefault(key, newValue)
		return
	}

	// modify the config then release the lock to avoid deadlocks while notifying
	var receivers []NotificationReceiver
	c.Lock()
	previousValue := c.Viper.Get(key)
	c.configSources[source].Set(key, newValue)
	c.mergeViperInstances(key)
	if !reflect.DeepEqual(previousValue, newValue) {
		// if the value has not changed, do not duplicate the slice so that no callback is called
		receivers = slices.Clone(c.notificationReceivers)
	}
	c.Unlock()

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, previousValue, newValue)
	}
}

// SetWithoutSource sets the given value using source Unknown
func (c *safeConfig) SetWithoutSource(key string, value interface{}) {
	c.Set(key, value, SourceUnknown)
}

// SetDefault wraps Viper for concurrent access
func (c *safeConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceDefault].Set(key, value)
	c.Viper.SetDefault(key, value)
}

// UnsetForSource wraps Viper for concurrent access
func (c *safeConfig) UnsetForSource(key string, source Source) {
	c.Lock()
	defer c.Unlock()
	c.configSources[source].Set(key, nil)
	c.mergeViperInstances(key)
}

// mergeViperInstances is called after a change in an instance of Viper
// to recompute the state of the main Viper
// (it must be used with a lock to prevent concurrent access to Viper)
func (c *safeConfig) mergeViperInstances(key string) {
	var val interface{}
	for _, source := range sources {
		if currVal := c.configSources[source].Get(key); currVal != nil {
			val = currVal
		}
	}
	c.Viper.Set(key, val)
}

// SetKnown adds a key to the set of known valid config keys
func (c *safeConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetKnown(key)
}

// IsKnown returns whether a key is known
func (c *safeConfig) IsKnown(key string) bool {
	c.RLock()
	defer c.RUnlock()

	return c.Viper.IsKnown(key)
}

// checkKnownKey checks if a key is known, and if not logs a warning
// Only a single warning will be logged per unknown key.
//
// Must be called with the lock read-locked.
// The lock can be released and re-locked.
func (c *safeConfig) checkKnownKey(key string) {
	if c.Viper.IsKnown(key) {
		return
	}

	if _, ok := c.unknownKeys[key]; ok {
		return
	}

	// need to write-lock to add the key to the unknownKeys map
	c.RUnlock()
	// but we need to have the lock in the same state (RLocked) at the end of the function
	defer c.RLock()

	c.Lock()
	c.unknownKeys[key] = struct{}{}
	c.Unlock()

	// log without holding the lock
	log.Warnf("config key %v is unknown", key)
}

// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
// Note that it returns the keys lowercased.
func (c *safeConfig) GetKnownKeysLowercased() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// GetKnownKeysLowercased returns a fresh map, so the caller may do with it
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

// IsSet wraps Viper for concurrent access
func (c *safeConfig) IsSetForSource(key string, source Source) bool {
	c.RLock()
	defer c.RUnlock()
	return c.configSources[source].IsSet(key)
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

	for _, key := range c.AllKeysLowercased() {
		if strings.HasPrefix(key, sectionPrefix) && c.IsSet(key) {
			return true
		}
	}

	// If none of the keys are set, the section is still considered as set
	// if it has been explicitly set in the config.
	return c.IsSet(section)
}

func (c *safeConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.AllKeys()
}

// Get wraps Viper for concurrent access
func (c *safeConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.Viper.GetE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetAllSources returns the value of a key for each source
func (c *safeConfig) GetAllSources(key string) []ValueWithSource {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	vals := make([]ValueWithSource, len(sources))
	for i, source := range sources {
		vals[i] = ValueWithSource{
			Source: source,
			Value:  c.configSources[source].Get(key),
		}
	}
	return vals
}

// GetString wraps Viper for concurrent access
func (c *safeConfig) GetString(key string) string {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)

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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
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
	c.checkKnownKey(key)
	val, err := c.Viper.GetSizeInBytesE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetSource wraps Viper for concurrent access
func (c *safeConfig) GetSource(key string) Source {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	var source Source
	for _, s := range sources {
		if c.configSources[s].Get(key) != nil {
			source = s
		}
	}
	return source
}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (c *safeConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceEnvVar].SetEnvPrefix(in)
	c.Viper.SetEnvPrefix(in)
	c.envPrefix = in
}

// mergeWithEnvPrefix derives the environment variable that Viper will use for a given key.
// mergeWithEnvPrefix must be called while holding the config log (read or write).
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

	_ = c.configSources[SourceEnvVar].BindEnv(input...)
	_ = c.Viper.BindEnv(input...)
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (c *safeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceEnvVar].SetEnvKeyReplacer(r)
	c.Viper.SetEnvKeyReplacer(r)
	c.envKeyReplacer = r
}

// UnmarshalKey wraps Viper for concurrent access
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return c.Viper.UnmarshalKey(key, rawVal, opts...)
}

// Unmarshal wraps Viper for concurrent access
func (c *safeConfig) Unmarshal(rawVal interface{}) error {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.Unmarshal(rawVal)
}

// UnmarshalExact wraps Viper for concurrent access
func (c *safeConfig) UnmarshalExact(rawVal interface{}) error {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.UnmarshalExact(rawVal)
}

// ReadInConfig wraps Viper for concurrent access
func (c *safeConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()
	err := c.Viper.ReadInConfig()
	if err != nil {
		return err
	}
	return c.configSources[SourceFile].ReadInConfig()
}

// ReadConfig wraps Viper for concurrent access
func (c *safeConfig) ReadConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	b, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	err = c.Viper.ReadConfig(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return c.configSources[SourceFile].ReadConfig(bytes.NewReader(b))
}

// MergeConfig wraps Viper for concurrent access
func (c *safeConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.MergeConfig(in)
}

// MergeConfigMap merges the configuration from the map given with an existing config.
// Note that the map given may be modified.
func (c *safeConfig) MergeConfigMap(cfg map[string]any) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.MergeConfigMap(cfg)
}

// AllSettings wraps Viper for concurrent access
func (c *safeConfig) AllSettings() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// AllSettings returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettings()
}

// AllSettingsWithoutDefault wraps Viper for concurrent access
func (c *safeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// AllSettingsWithoutDefault returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettingsWithoutDefault()
}

// AllSourceSettingsWithoutDefault wraps Viper for concurrent access
func (c *safeConfig) AllSourceSettingsWithoutDefault(source Source) map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// AllSourceSettingsWithoutDefault returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.configSources[source].AllSettingsWithoutDefault()
}

// AddConfigPath wraps Viper for concurrent access
func (c *safeConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceFile].AddConfigPath(in)
	c.Viper.AddConfigPath(in)
}

// SetConfigName wraps Viper for concurrent access
func (c *safeConfig) SetConfigName(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceFile].SetConfigName(in)
	c.Viper.SetConfigName(in)
}

// SetConfigFile wraps Viper for concurrent access
func (c *safeConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceFile].SetConfigFile(in)
	c.Viper.SetConfigFile(in)
}

// SetConfigType wraps Viper for concurrent access
func (c *safeConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceFile].SetConfigType(in)
	c.Viper.SetConfigType(in)
}

// ConfigFileUsed wraps Viper for concurrent access
func (c *safeConfig) ConfigFileUsed() string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.ConfigFileUsed()
}

func (c *safeConfig) SetTypeByDefaultValue(in bool) {
	c.Lock()
	defer c.Unlock()
	for _, source := range sources {
		c.configSources[source].SetTypeByDefaultValue(in)
	}
	c.Viper.SetTypeByDefaultValue(in)
}

// BindPFlag wraps Viper for concurrent access
func (c *safeConfig) BindPFlag(key string, flag *pflag.Flag) error {
	c.Lock()
	defer c.Unlock()
	return c.Viper.BindPFlag(key, flag)
}

// GetEnvVars implements the Config interface
func (c *safeConfig) GetEnvVars() []string {
	c.RLock()
	defer c.RUnlock()
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

func (c *safeConfig) Warnings() *Warnings {
	return nil
}

func (c *safeConfig) Object() Reader {
	return c
}

// NewConfig returns a new Config object.
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) Config {
	config := safeConfig{
		Viper:         viper.New(),
		configSources: map[Source]*viper.Viper{},
		configEnvVars: map[string]struct{}{},
		unknownKeys:   map[string]struct{}{},
	}

	// load one Viper instance per source of setting change
	for _, source := range sources {
		config.configSources[source] = viper.New()
	}

	config.SetTypeByDefaultValue(true)
	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)

	return &config
}

// CopyConfig copies the given config to the receiver config. This should only be used in tests as replacing
// the global config reference is unsafe.
func (c *safeConfig) CopyConfig(cfg Config) {
	c.Lock()
	defer c.Unlock()

	if cfg, ok := cfg.(*safeConfig); ok {
		c.Viper = cfg.Viper
		c.configSources = cfg.configSources
		c.envPrefix = cfg.envPrefix
		c.envKeyReplacer = cfg.envKeyReplacer
		c.configEnvVars = cfg.configEnvVars
		c.unknownKeys = cfg.unknownKeys
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
