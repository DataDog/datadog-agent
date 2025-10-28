// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package viperconfig provides a viper-based implementation of the config interface.
package viperconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/viper"
	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// safeConfig implements Config:
// - wraps viper with a safety lock
// - implements the additional DDHelpers
type safeConfig struct {
	*viper.Viper
	configSources map[model.Source]*viper.Viper
	sync.RWMutex
	envPrefix      string
	envKeyReplacer *strings.Replacer

	notificationReceivers []model.NotificationReceiver
	sequenceID            uint64

	// ready is whether the schema has been built, which marks the config as ready for use
	ready *atomic.Bool

	// Proxy settings
	proxies *model.Proxy

	// configEnvVars is the set of env vars that are consulted for
	// configuration values.
	configEnvVars map[string]struct{}

	// tree of env vars, lets us properly build parent-child linking
	envVarTree map[string]interface{}

	// keys that have been used but are unknown
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}

	// extraConfigFilePaths represents additional configuration file paths that will be merged into the main configuration when ReadInConfig() is called.
	extraConfigFilePaths []string

	// warnings contains the warnings that were logged during the configuration loading
	warnings []error

	existingTransformers map[string]bool
}

// OnUpdate adds a callback to the list receivers to be called each time a value is changed in the configuration
// by a call to the 'Set' method.
// Callbacks are only called if the value is effectively changed.
func (c *safeConfig) OnUpdate(callback model.NotificationReceiver) {
	c.Lock()
	defer c.Unlock()
	c.notificationReceivers = append(c.notificationReceivers, callback)
}

func getCallerLocation(nbStack int) string {
	_, file, line, _ := runtime.Caller(nbStack + 1)
	fileParts := strings.Split(file, "DataDog/datadog-agent/")
	return fmt.Sprintf("%s:%d", fileParts[len(fileParts)-1], line)
}

// Set wraps Viper for concurrent access
func (c *safeConfig) Set(key string, newValue interface{}, source model.Source) {
	if source == model.SourceDefault {
		c.SetDefault(key, newValue)
		return
	}

	// modify the config then release the lock to avoid deadlocks while notifying
	var receivers []model.NotificationReceiver
	c.Lock()
	oldValue := c.Viper.Get(key)

	// First we check if the layer changed
	previousValueFromLayer := c.configSources[source].Get(key)
	if !reflect.DeepEqual(previousValueFromLayer, newValue) {
		c.configSources[source].Set(key, newValue)
		c.mergeViperInstances(key)
	} else {
		// nothing changed:w
		log.Debugf("Updating setting '%s' for source '%s' with the same value, skipping notification", key, source)
		c.Unlock()
		return
	}

	// We might have updated a layer that is itself overridden by another (ex: updating a setting a the 'file' level
	// already overridden at the 'cli' level. If it the case we do nothing.
	latestValue := c.Viper.Get(key)
	if !reflect.DeepEqual(oldValue, latestValue) {
		log.Debugf("Updating setting '%s' for source '%s' with new value. notifying %d listeners", key, source, len(c.notificationReceivers))
		// if the value has not changed, do not duplicate the slice so that no callback is called
		receivers = slices.Clone(c.notificationReceivers)
	} else {
		log.Debugf("Updating setting '%s' for source '%s' with the same value, skipping notification", key, source)
		c.Unlock()
		return
	}
	// Increment the sequence ID only if the value has changed
	c.sequenceID++
	c.Unlock()

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		log.Debugf("notifying %s about configuration change for '%s'", getCallerLocation(1), key)
		receiver(key, source, oldValue, latestValue, c.sequenceID)
	}
}

// SetWithoutSource sets the given value using source Unknown, may only be called from tests
func (c *safeConfig) SetWithoutSource(key string, value interface{}) {
	c.assertIsTest("SetWithoutSource")
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		panic("SetWithoutSource cannot assign struct to a setting")
	}
	c.Set(key, value, model.SourceUnknown)
}

// SetDefault wraps Viper for concurrent access
func (c *safeConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()
	c.configSources[model.SourceDefault].Set(key, value)
	c.Viper.SetDefault(key, value)
}

// UnsetForSource unsets a config entry for a given source
func (c *safeConfig) UnsetForSource(key string, source model.Source) {
	// modify the config then release the lock to avoid deadlocks while notifying
	var receivers []model.NotificationReceiver
	c.Lock()
	defer c.Unlock()
	previousValue := c.Viper.Get(key)
	c.configSources[source].Set(key, nil)
	c.mergeViperInstances(key)
	newValue := c.Viper.Get(key) // Can't use nil, so we get the newly computed value
	if previousValue != nil && !reflect.DeepEqual(previousValue, newValue) {
		// if the value has not changed, do not duplicate the slice so that no callback is called
		receivers = slices.Clone(c.notificationReceivers)
		c.sequenceID++
	}

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, source, previousValue, newValue, c.sequenceID)
	}
}

// mergeViperInstances is called after a change in an instance of Viper
// to recompute the state of the main Viper
// (it must be used with a lock to prevent concurrent access to Viper)
func (c *safeConfig) mergeViperInstances(key string) {
	var val interface{}
	for _, source := range model.Sources {
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
	c.Viper.SetKnown(key) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
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

	// log without holding the lock. We use stack depth +3 to use the caller function location instead of checkKnownKey
	log.WarnfStackDepth(3, "config key %q is unknown", key)
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
	c.ready.Store(true)
}

func (c *safeConfig) setEnvTransformer(key string, fn func(string) interface{}) {
	c.Lock()
	defer c.Unlock()

	// We need to set it on both the final config and the env layer. If not, when setting something at runtime the
	// mergeViperInstances will pull the value from SourceEnvVar and overwrites it in c.Viper changing the type.
	//
	// This is yet another edge case of working with Viper, this edge cases is already handled by the nodetremodel
	// replacement.
	if _, exists := c.existingTransformers[key]; exists {
		panic(fmt.Sprintf("env transform for %s already exists", key))
	}
	c.existingTransformers[key] = true
	c.configSources[model.SourceEnvVar].SetEnvKeyTransformer(key, fn)
	c.Viper.SetEnvKeyTransformer(key, fn)
}

// ParseEnvAsStringSlice registers a transformer function to parse an an environment variables as a []string.
func (c *safeConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.setEnvTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsMapStringInterface registers a transformer function to parse an an environment variables as a
// map[string]interface{}.
func (c *safeConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.setEnvTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSliceMapString registers a transformer function to parse an an environment variables as a []map[string]string.
func (c *safeConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.setEnvTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSlice registers a transformer function to parse an an environment variables as a
// []interface{}.
func (c *safeConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.setEnvTransformer(key, func(data string) interface{} { return fn(data) })
}

// IsSet wraps Viper for concurrent access
func (c *safeConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.IsSet(key)
}

// IsConfigured returns true if a settings was configured by the user (ie: the value doesn't come from defaults)
func (c *safeConfig) IsConfigured(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.IsConfigured(key)
}

func (c *safeConfig) AllKeysLowercased() []string {
	c.Lock()
	defer c.Unlock()
	res := c.Viper.AllKeys()
	slices.Sort(res)
	return res
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
func (c *safeConfig) GetAllSources(key string) []model.ValueWithSource {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	vals := make([]model.ValueWithSource, len(model.Sources))
	for i, source := range model.Sources {
		vals[i] = model.ValueWithSource{
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
func (c *safeConfig) GetFloat64Slice(key string) []float64 {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)

	// We're using GetStringSlice because viper can only parse list of string from env variables
	list, err := c.Viper.GetStringSliceE(key)
	if err != nil {
		log.Warnf("'%v' is not a list", key)
		return nil
	}

	res := []float64{}
	for _, item := range list {
		nb, err := strconv.ParseFloat(item, 64)
		if err != nil {
			log.Warnf("value '%v' from '%v' is not a float64", item, key)
			return nil
		}
		res = append(res, nb)
	}
	return res
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
func (c *safeConfig) GetSource(key string) model.Source {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	var source model.Source
	for _, s := range model.Sources {
		if c.configSources[s].Get(key) != nil {
			source = s
		}
	}
	return source
}

func (c *safeConfig) isReady() bool {
	return c.ready.Load()
}

// RevertFinishedBackToBuilder returns an interface that can build more on
// the current config, instead of treating it as sealed
// NOTE: Only used by OTel, no new uses please!
func (c *safeConfig) RevertFinishedBackToBuilder() model.BuildableConfig {
	c.ready.Store(false)
	return c
}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (c *safeConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() {
		panic("cannot SetEnvPrefix() once the config has been marked as ready for use")
	}
	c.existingTransformers = make(map[string]bool)
	c.configSources[model.SourceEnvVar].SetEnvPrefix(in)
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

	for _, envname := range envKeys {
		// apply EnvKeyReplacer to each key
		if c.envKeyReplacer != nil {
			envname = c.envKeyReplacer.Replace(envname)
		}
		c.configEnvVars[envname] = struct{}{}
	}

	// Add the env var into a tree, so we know which setting has children that use env vars
	currTree := c.envVarTree
	parts := strings.Split(key, ".")
	for _, part := range parts {
		if elem, found := currTree[part].(map[string]interface{}); found {
			currTree = elem
		} else {
			alloc := make(map[string]interface{})
			currTree[part] = alloc
			currTree = alloc
		}
	}

	newKeys := append([]string{key}, envvars...)
	_ = c.configSources[model.SourceEnvVar].BindEnv(newKeys...) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	_ = c.Viper.BindEnv(newKeys...)                             //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (c *safeConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() {
		panic("cannot SetEnvPrefix() once the config has been marked as ready for use")
	}
	c.configSources[model.SourceEnvVar].SetEnvKeyReplacer(r)
	c.Viper.SetEnvKeyReplacer(r)
	c.envKeyReplacer = r
}

// UnmarshalKey wraps Viper for concurrent access
// DEPRECATED: use pkg/config/structure.UnmarshalKey instead
func (c *safeConfig) UnmarshalKey(key string, rawVal interface{}, opts ...func(*mapstructure.DecoderConfig)) error {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)

	decodeOptions := []viper.DecoderConfigOption{}
	for _, opt := range opts {
		decodeOptions = append(decodeOptions, viper.DecoderConfigOption(opt))
	}

	return c.Viper.UnmarshalKey(key, rawVal, decodeOptions...)
}

// ReadInConfig wraps Viper for concurrent access
func (c *safeConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()
	// ReadInConfig reset configuration with the main config file
	err := errors.Join(c.Viper.ReadInConfig(), c.configSources[model.SourceFile].ReadInConfig())
	if err != nil {
		return model.NewConfigFileNotFoundError(err) // nolint: forbidigo // constructing proper error
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
		err = errors.Join(c.Viper.MergeConfig(bytes.NewReader(confFile.content)), c.configSources[model.SourceFile].MergeConfig(bytes.NewReader(confFile.content)))
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
	return c.configSources[model.SourceFile].ReadConfig(bytes.NewReader(b))
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

	c.configSources[model.SourceFleetPolicies].SetConfigType("yaml")
	err = c.configSources[model.SourceFleetPolicies].MergeConfigOverride(in)
	if err != nil {
		return err
	}
	for _, key := range c.configSources[model.SourceFleetPolicies].AllKeys() {
		c.mergeViperInstances(key)
	}
	log.Infof("Fleet policies configuration %s successfully merged", path.Base(configPath))
	return nil
}

// AllSettings wraps Viper for concurrent access
func (c *safeConfig) AllSettings() map[string]interface{} {
	c.Lock()
	defer c.Unlock()

	// AllSettings returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettings()
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *safeConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.Lock()
	defer c.Unlock()

	// AllSettingsWithoutDefault returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	return c.Viper.AllSettingsWithoutDefault()
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *safeConfig) AllSettingsBySource() map[model.Source]interface{} {
	c.Lock()
	defer c.Unlock()

	res := map[model.Source]interface{}{}
	for _, source := range model.Sources {
		res[source] = c.configSources[source].AllSettingsWithoutDefault()
	}
	res[model.SourceProvided] = c.Viper.AllSettingsWithoutDefault()
	return res
}

// AllSettingsWithSequenceID returns the settings and the sequence ID.
func (c *safeConfig) AllSettingsWithSequenceID() (map[string]interface{}, uint64) {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.AllSettings(), c.sequenceID
}

// AddConfigPath wraps Viper for concurrent access
func (c *safeConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[model.SourceFile].AddConfigPath(in)
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
	c.configSources[model.SourceFile].SetConfigName(in)
	c.Viper.SetConfigName(in)
}

// SetConfigFile wraps Viper for concurrent access
func (c *safeConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[model.SourceFile].SetConfigFile(in)
	c.Viper.SetConfigFile(in)
}

// SetConfigType wraps Viper for concurrent access
func (c *safeConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.configSources[model.SourceFile].SetConfigType(in)
	c.Viper.SetConfigType(in)
}

// ConfigFileUsed wraps Viper for concurrent access
func (c *safeConfig) ConfigFileUsed() string {
	c.RLock()
	defer c.RUnlock()
	return c.Viper.ConfigFileUsed()
}

// GetSubfields returns the names of additional settings under the given key
func (c *safeConfig) GetSubfields(key string) []string {
	c.Lock()
	defer c.Unlock()

	res := []string{}
	for _, s := range model.Sources {
		if s == model.SourceEnvVar {
			// Viper doesn't store env vars in the actual configSource layer, instead
			// use the envVarTree built by this wrapper to lookup which env vars exist
			currTree := c.envVarTree
			parts := strings.Split(key, ".")
			for _, part := range parts {
				if elem, found := currTree[part].(map[string]interface{}); found {
					currTree = elem
				} else {
					currTree = nil
					break
				}
			}
			for k := range currTree {
				res = append(res, k)
			}
			continue
		}
		if layer, ok := c.configSources[s].Get(key).(map[string]interface{}); ok {
			for k := range layer {
				res = append(res, k)
			}
		}
	}

	sort.Strings(res)
	res = slices.Compact(res)
	return res
}

func (c *safeConfig) SetTypeByDefaultValue(in bool) {
	c.Lock()
	defer c.Unlock()
	for _, source := range model.Sources {
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
	c.BindEnv(key, envvars...) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv' //nolint:errcheck
}

func (c *safeConfig) Warnings() *model.Warnings {
	return &model.Warnings{Errors: c.warnings}
}

func (c *safeConfig) Object() model.Reader {
	return c
}

// NewConfig returns a new viper config
// Deprecated: instead use pkg/config/create.NewConfig or NewViperConfig
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) model.BuildableConfig {
	return NewViperConfig(name, envPrefix, envKeyReplacer)
}

// NewViperConfig returns a new Config object.
func NewViperConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) model.BuildableConfig {
	config := safeConfig{
		Viper:                viper.New(),
		configSources:        map[model.Source]*viper.Viper{},
		sequenceID:           0,
		ready:                atomic.NewBool(false),
		envVarTree:           make(map[string]interface{}),
		configEnvVars:        map[string]struct{}{},
		unknownKeys:          map[string]struct{}{},
		existingTransformers: make(map[string]bool),
	}

	// load one Viper instance per source of setting change
	for _, source := range model.Sources {
		config.configSources[source] = viper.New()
	}

	config.SetTypeByDefaultValue(true)
	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)

	return &config
}

// Stringify stringifies the config, but only for nodetremodel with the test build tag
func (c *safeConfig) Stringify(_ model.Source, _ ...model.StringifyOption) string {
	return "safeConfig{...}"
}

// GetProxies returns the proxy settings from the configuration
func (c *safeConfig) GetProxies() *model.Proxy {
	c.Lock()
	defer c.Unlock()
	if c.proxies != nil {
		return c.proxies
	}
	if c.Viper.GetBool("fips.enabled") {
		return nil
	}
	if c.Viper.GetString("proxy.http") == "" && c.Viper.GetString("proxy.https") == "" && len(c.Viper.GetStringSlice("proxy.no_proxy")) == 0 {
		return nil
	}
	p := &model.Proxy{
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

func (c *safeConfig) SetTestOnlyDynamicSchema(_ bool) {
}

func (c *safeConfig) GetSequenceID() uint64 {
	c.RLock()
	defer c.RUnlock()
	return c.sequenceID
}
