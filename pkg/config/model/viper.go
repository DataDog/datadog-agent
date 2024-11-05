// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"path/filepath"

	"github.com/DataDog/viper"
	"github.com/mohae/deepcopy"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Source stores what edits a setting as a string
type Source string

// Declare every known Source
const (
	// SourceDefault are the values from defaults.
	SourceDefault Source = "default"
	// SourceUnknown are the values from unknown source. This should only be used in tests when calling
	// SetWithoutSource.
	SourceUnknown Source = "unknown"
	// SourceFile are the values loaded from configuration file.
	SourceFile Source = "file"
	// SourceEnvVar are the values loaded from the environment variables.
	SourceEnvVar Source = "environment-variable"
	// SourceAgentRuntime are the values configured by the agent itself. The agent can dynamically compute the best
	// value for some settings when not set by the user.
	SourceAgentRuntime Source = "agent-runtime"
	// SourceLocalConfigProcess are the values mirrored from the config process. The config process is the
	// core-agent. This is used when side process like security-agent or trace-agent pull their configuration from
	// the core-agent.
	SourceLocalConfigProcess Source = "local-config-process"
	// SourceRC are the values loaded from remote-config (aka Datadog backend)
	SourceRC Source = "remote-config"
	// SourceFleetPolicies are the values loaded from remote-config file
	SourceFleetPolicies Source = "fleet-policies"
	// SourceCLI are the values set by the user at runtime through the CLI.
	SourceCLI Source = "cli"
	// SourceProvided are all values set by any source but default.
	SourceProvided Source = "provided" // everything but defaults
)

// sources list the known sources, following the order of hierarchy between them
var sources = []Source{
	SourceDefault,
	SourceUnknown,
	SourceFile,
	SourceEnvVar,
	SourceFleetPolicies,
	SourceAgentRuntime,
	SourceLocalConfigProcess,
	SourceRC,
	SourceCLI,
}

// sourcesPriority give each source a priority, the higher the more important a source. This is used when merging
// configuration tree (a higher priority overwrites a lower one).
var sourcesPriority = map[Source]int{
	SourceDefault:            0,
	SourceUnknown:            1,
	SourceFile:               2,
	SourceEnvVar:             3,
	SourceFleetPolicies:      4,
	SourceAgentRuntime:       5,
	SourceLocalConfigProcess: 6,
	SourceRC:                 7,
	SourceCLI:                8,
}

// ValueWithSource is a tuple for a source and a value, not necessarily the applied value in the main config
type ValueWithSource struct {
	Source Source
	Value  interface{}
}

// IsGreaterOrEqualThan returns true if the current source is of higher priority than the one given as a parameter
func (s Source) IsGreaterOrEqualThan(x Source) bool {
	return sourcesPriority[s] >= sourcesPriority[x]
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
	proxies *Proxy

	// configEnvVars is the set of env vars that are consulted for
	// configuration values.
	configEnvVars map[string]struct{}

	// keys that have been used but are unknown
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}

	// extraConfigFilePaths represents additional configuration file paths that will be merged into the main configuration when ReadInConfig() is called.
	extraConfigFilePaths []string
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

// UnsetForSource unsets a config entry for a given source
func (c *safeConfig) UnsetForSource(key string, source Source) {
	// modify the config then release the lock to avoid deadlocks while notifying
	var receivers []NotificationReceiver
	c.Lock()
	previousValue := c.Viper.Get(key)
	c.configSources[source].Set(key, nil)
	c.mergeViperInstances(key)
	newValue := c.Viper.Get(key) // Can't use nil, so we get the newly computed value
	if previousValue != nil {
		// if the value has not changed, do not duplicate the slice so that no callback is called
		receivers = slices.Clone(c.notificationReceivers)
	}
	c.Unlock()

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, previousValue, newValue)
	}
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

// BuildSchema is a no-op for the viper based config
func (c *safeConfig) BuildSchema() {
	// pass
}

// ParseEnvAsStringSlice registers a transformer function to parse an an environment variables as a []string.
func (c *safeConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsMapStringInterface registers a transformer function to parse an an environment variables as a
// map[string]interface{}.
func (c *safeConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSliceMapString registers a transformer function to parse an an environment variables as a []map[string]string.
func (c *safeConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSlice registers a transformer function to parse an an environment variables as a
// []interface{}.
func (c *safeConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.Lock()
	defer c.Unlock()
	c.Viper.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// IsSet wraps Viper for concurrent access
func (c *safeConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.IsSet(key)
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
	return deepcopy.Copy(val)
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
			Value:  deepcopy.Copy(c.configSources[source].Get(key)),
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
	return slices.Clone(val)
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
	return deepcopy.Copy(val).(map[string]interface{})
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
	return deepcopy.Copy(val).(map[string]string)
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
	return deepcopy.Copy(val).(map[string][]string)
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
func (c *safeConfig) BindEnv(key string, envvars ...string) {
	c.Lock()
	defer c.Unlock()
	var envKeys []string

	// If one input is given, viper derives an env key from it; otherwise, all inputs after
	// the first are literal env vars.
	if len(envvars) == 0 {
		envKeys = []string{c.mergeWithEnvPrefix(key)}
	} else {
		envKeys = envvars
	}

	for _, key := range envKeys {
		// apply EnvKeyReplacer to each key
		if c.envKeyReplacer != nil {
			key = c.envKeyReplacer.Replace(key)
		}
		c.configEnvVars[key] = struct{}{}
	}

	newKeys := append([]string{key}, envvars...)
	_ = c.configSources[SourceEnvVar].BindEnv(newKeys...)
	_ = c.Viper.BindEnv(newKeys...)
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
// DEPRECATED: use pkg/config/structure.UnmarshalKey instead
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return c.Viper.UnmarshalKey(key, rawVal, opts...)
}

// ReadInConfig wraps Viper for concurrent access
func (c *safeConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()
	// ReadInConfig reset configuration with the main config file
	err := errors.Join(c.Viper.ReadInConfig(), c.configSources[SourceFile].ReadInConfig())
	if err != nil {
		return err
	}

	type extraConf struct {
		path    string
		content []byte
	}

	// Read extra config files
	extraConfContents := []extraConf{}
	for _, path := range c.extraConfigFilePaths {
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("could not read extra config file '%s': %w", path, err)
		}
		extraConfContents = append(extraConfContents, extraConf{path: path, content: b})
	}

	// Merge with base config and 'file' config
	for _, confFile := range extraConfContents {
		err = errors.Join(c.Viper.MergeConfig(bytes.NewReader(confFile.content)), c.configSources[SourceFile].MergeConfig(bytes.NewReader(confFile.content)))
		if err != nil {
			return fmt.Errorf("error merging %s config file: %w", confFile.path, err)
		}
		log.Infof("extra configuration file %s was loaded successfully", confFile.path)
	}
	return nil
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

// MergeFleetPolicy merges the configuration from the reader given with an existing config
// it overrides the existing values with the new ones in the FleetPolicies source, and updates the main config
// according to sources priority order.
//
// Note: this should only be called at startup, as notifiers won't receive a notification when this loads
func (c *safeConfig) MergeFleetPolicy(configPath string) error {
	c.Lock()
	defer c.Unlock()

	// Check file existence & open it
	_, err := os.Stat(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to open config file %s: %w", configPath, err)
	} else if err != nil && os.IsNotExist(err) {
		return nil
	}
	in, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("unable to open config file %s: %w", configPath, err)
	}
	defer in.Close()

	c.configSources[SourceFleetPolicies].SetConfigType("yaml")
	err = c.configSources[SourceFleetPolicies].MergeConfigOverride(in)
	if err != nil {
		return err
	}
	for _, key := range c.configSources[SourceFleetPolicies].AllKeys() {
		c.mergeViperInstances(key)
	}
	log.Infof("Fleet policies configuration %s successfully merged", path.Base(configPath))
	return nil
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

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *safeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// AllSettingsWithoutDefault returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettingsWithoutDefault()
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *safeConfig) AllSettingsBySource() map[Source]interface{} {
	c.RLock()
	defer c.RUnlock()

	sources := []Source{
		SourceDefault,
		SourceUnknown,
		SourceFile,
		SourceEnvVar,
		SourceFleetPolicies,
		SourceAgentRuntime,
		SourceRC,
		SourceCLI,
		SourceLocalConfigProcess,
	}
	res := map[Source]interface{}{}
	for _, source := range sources {
		res[source] = c.configSources[source].AllSettingsWithoutDefault()
	}
	res[SourceProvided] = c.Viper.AllSettingsWithoutDefault()
	return res
}

// AddConfigPath wraps Viper for concurrent access
func (c *safeConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[SourceFile].AddConfigPath(in)
	c.Viper.AddConfigPath(in)
}

// AddExtraConfigPaths allows adding additional configuration files
// which will be merged into the main configuration during the ReadInConfig call.
// Configuration files are merged sequentially. If a key already exists and the foreign value type matches the existing one, the foreign value overrides it.
// If both the existing value and the new value are nested configurations, they are merged recursively following the same principles.
func (c *safeConfig) AddExtraConfigPaths(ins []string) error {
	if len(ins) == 0 {
		return nil
	}
	c.Lock()
	defer c.Unlock()
	var pathsToAdd []string
	var errs []error
	for _, in := range ins {
		in, err := filepath.Abs(in)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not get absolute path of extra config file '%s': %s", in, err))
			continue
		}
		if slices.Index(c.extraConfigFilePaths, in) == -1 && slices.Index(pathsToAdd, in) == -1 {
			pathsToAdd = append(pathsToAdd, in)
		}
	}
	err := errors.Join(errs...)
	if err == nil {
		c.extraConfigFilePaths = append(c.extraConfigFilePaths, pathsToAdd...)
	}
	return err
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
func (c *safeConfig) BindEnvAndSetDefault(key string, val interface{}, envvars ...string) {
	c.SetDefault(key, val)
	c.BindEnv(key, envvars...) //nolint:errcheck
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
		c.proxies = cfg.proxies
		c.configEnvVars = cfg.configEnvVars
		c.unknownKeys = cfg.unknownKeys
		c.notificationReceivers = cfg.notificationReceivers
		return
	}
	panic("Replacement config must be an instance of safeConfig")
}

// GetProxies returns the proxy settings from the configuration
func (c *safeConfig) GetProxies() *Proxy {
	c.Lock()
	defer c.Unlock()
	if c.proxies != nil {
		return c.proxies
	}
	if c.Viper.GetBool("fips.enabled") {
		return nil
	}
	if !c.Viper.IsSet("proxy.http") && !c.Viper.IsSet("proxy.https") && !c.Viper.IsSet("proxy.no_proxy") {
		return nil
	}
	p := &Proxy{
		HTTP:    c.Viper.GetString("proxy.http"),
		HTTPS:   c.Viper.GetString("proxy.https"),
		NoProxy: c.Viper.GetStringSlice("proxy.no_proxy"),
	}

	c.proxies = p
	return c.proxies
}

func (c *safeConfig) ExtraConfigFilesUsed() []string {
	c.Lock()
	defer c.Unlock()
	res := make([]string, len(c.extraConfigFilePaths))
	copy(res, c.extraConfigFilePaths)
	return res
}
