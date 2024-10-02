// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"path/filepath"

	"github.com/DataDog/viper"
	"github.com/mohae/deepcopy"
	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sources list the known sources, following the order of hierarchy between them
var sources = []model.Source{
	model.SourceDefault,
	model.SourceUnknown,
	model.SourceFile,
	model.SourceEnvVar,
	model.SourceFleetPolicies,
	model.SourceAgentRuntime,
	model.SourceLocalConfigProcess,
	model.SourceRC,
	model.SourceCLI,
}

// ntmConfig implements Config
// - wraps a tree of node that represent config data
// - uses a lock to synchronize all methods
// - contains metadata about known keys, env var support
type ntmConfig struct {
	sync.RWMutex
	root   Node
	noimpl notImplementedMethods

	envPrefix      string
	envKeyReplacer *strings.Replacer

	notificationReceivers []model.NotificationReceiver

	// Proxy settings
	proxies *model.Proxy

	configName string
	configFile string
	configType string

	// configEnvVars is the set of env vars that are consulted for
	// configuration values.
	configEnvVars map[string]struct{}

	// known keys are all the keys that meet at least one of these criteria:
	// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
	knownKeys map[string]struct{}
	// keys that have been used but are unknown
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}

	// extraConfigFilePaths represents additional configuration file paths that will be merged into the main configuration when ReadInConfig() is called.
	extraConfigFilePaths []string
}

// OnUpdate adds a callback to the list receivers to be called each time a value is changed in the configuration
// by a call to the 'Set' method.
// Callbacks are only called if the value is effectively changed.
func (c *ntmConfig) OnUpdate(callback model.NotificationReceiver) {
	c.Lock()
	defer c.Unlock()
	c.notificationReceivers = append(c.notificationReceivers, callback)
}

// getValue gets a value, should only be called within a locked mutex
func (c *ntmConfig) getValue(key string) (interface{}, error) {
	return c.leafAtPath(key).GetAny()
}

func (c *ntmConfig) setValueSource(key string, newValue interface{}, source model.Source) {
	err := c.leafAtPath(key).SetWithSource(newValue, source)
	if err != nil {
		log.Errorf("%s", err)
	}
}

// Set wraps Viper for concurrent access
func (c *ntmConfig) Set(key string, newValue interface{}, source model.Source) {
	if source == model.SourceDefault {
		c.SetDefault(key, newValue)
		return
	}

	// modify the config then release the lock to avoid deadlocks while notifying
	var receivers []model.NotificationReceiver
	c.Lock()
	previousValue, _ := c.getValue(key)
	c.setValueSource(key, newValue, source)
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
func (c *ntmConfig) SetWithoutSource(key string, value interface{}) {
	c.Set(key, value, model.SourceUnknown)
}

// SetDefault wraps Viper for concurrent access
func (c *ntmConfig) SetDefault(key string, value interface{}) {
	c.Set(key, value, model.SourceDefault)
}

// UnsetForSource unsets a config entry for a given source
func (c *ntmConfig) UnsetForSource(key string, source model.Source) {
	// modify the config then release the lock to avoid deadlocks while notifying
	//var receivers []model.NotificationReceiver
	// TODO: Is this needed by anything?
	c.Lock()
	c.logErrorNotImplemented("UnsetForSource")
	c.Unlock()

	// notifying all receiver about the updated setting
	//for _, receiver := range receivers {
	//receiver(key, previousValue, newValue)
	//}
}

// SetKnown adds a key to the set of known valid config keys
func (c *ntmConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	key = strings.ToLower(key)
	c.knownKeys[key] = struct{}{}
}

// IsKnown returns whether a key is known
func (c *ntmConfig) IsKnown(key string) bool {
	c.RLock()
	defer c.RUnlock()
	key = strings.ToLower(key)
	_, found := c.knownKeys[key]
	return found
}

// checkKnownKey checks if a key is known, and if not logs a warning
// Only a single warning will be logged per unknown key.
//
// Must be called with the lock read-locked.
// The lock can be released and re-locked.
func (c *ntmConfig) checkKnownKey(key string) {
	if c.IsKnown(key) {
		return
	}

	if _, ok := c.unknownKeys[key]; ok {
		return
	}

	c.unknownKeys[key] = struct{}{}
	log.Warnf("config key %v is unknown", key)
}

// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
// Note that it returns the keys lowercased.
func (c *ntmConfig) GetKnownKeysLowercased() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// GetKnownKeysLowercased returns a fresh map, so the caller may do with it
	// as they please without holding the lock.
	ret := make(map[string]interface{})
	for key, value := range c.knownKeys {
		ret[key] = value
	}
	return ret
}

// ParseEnvAsStringSlice registers a transformer function to parse an an environment variables as a []string.
func (c *ntmConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsMapStringInterface registers a transformer function to parse an an environment variables as a
// map[string]interface{}.
func (c *ntmConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSliceMapString registers a transformer function to parse an an environment variables as a []map[string]string.
func (c *ntmConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSlice registers a transformer function to parse an an environment variables as a
// []interface{}.
func (c *ntmConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// SetFs wraps Viper for concurrent access
func (c *ntmConfig) SetFs(fs afero.Fs) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetFs(fs)
}

// IsSet wraps Viper for concurrent access
func (c *ntmConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.noimpl.IsSet(key)
}

func (c *ntmConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()
	return c.noimpl.AllKeys()
}

func (c *ntmConfig) leafAtPath(key string) LeafNode {
	pathParts := strings.Split(key, ".")
	curr := c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return &missingLeaf
		}
		curr = next
	}
	if leaf, ok := curr.(LeafNode); ok {
		return leaf
	}
	return &missingLeaf
}

// Get wraps Viper for concurrent access
func (c *ntmConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.getValue(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	// TODO: should only need to deepcopy for `Get`, because it can be an arbitrary value,
	// and we shouldn't ever return complex types like maps and slices that could be modified
	// by callers accidentally or on purpose. By copying, the caller may modify the result safetly
	return deepcopy.Copy(val)
}

// GetAllSources returns the value of a key for each source
func (c *ntmConfig) GetAllSources(key string) []model.ValueWithSource {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	vals := make([]model.ValueWithSource, len(sources))
	c.logErrorNotImplemented("GetAllSources")
	return vals
}

// GetString wraps Viper for concurrent access
func (c *ntmConfig) GetString(key string) string {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	str, err := c.leafAtPath(key).GetString()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return str
}

// GetBool wraps Viper for concurrent access
func (c *ntmConfig) GetBool(key string) bool {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	b, err := c.leafAtPath(key).GetBool()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return b
}

// GetInt wraps Viper for concurrent access
func (c *ntmConfig) GetInt(key string) int {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetInt()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetInt32 wraps Viper for concurrent access
func (c *ntmConfig) GetInt32(key string) int32 {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetInt()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return int32(val)
}

// GetInt64 wraps Viper for concurrent access
func (c *ntmConfig) GetInt64(key string) int64 {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetInt()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return int64(val)
}

// GetFloat64 wraps Viper for concurrent access
func (c *ntmConfig) GetFloat64(key string) float64 {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetFloat()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetTime wraps Viper for concurrent access
func (c *ntmConfig) GetTime(key string) time.Time {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetTime()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetDuration wraps Viper for concurrent access
func (c *ntmConfig) GetDuration(key string) time.Duration {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.leafAtPath(key).GetDuration()
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetStringSlice wraps Viper for concurrent access
func (c *ntmConfig) GetStringSlice(key string) []string {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.noimpl.GetStringSliceE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return slices.Clone(val)
}

// GetFloat64SliceE loads a key as a []float64
func (c *ntmConfig) GetFloat64SliceE(key string) ([]float64, error) {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)

	// We're using GetStringSlice because viper can only parse list of string from env variables
	list, err := c.noimpl.GetStringSliceE(key)
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
func (c *ntmConfig) GetStringMap(key string) map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.noimpl.GetStringMapE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return deepcopy.Copy(val).(map[string]interface{})
}

// GetStringMapString wraps Viper for concurrent access
func (c *ntmConfig) GetStringMapString(key string) map[string]string {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.noimpl.GetStringMapStringE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return deepcopy.Copy(val).(map[string]string)
}

// GetStringMapStringSlice wraps Viper for concurrent access
func (c *ntmConfig) GetStringMapStringSlice(key string) map[string][]string {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.noimpl.GetStringMapStringSliceE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return deepcopy.Copy(val).(map[string][]string)
}

// GetSizeInBytes wraps Viper for concurrent access
func (c *ntmConfig) GetSizeInBytes(key string) uint {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.noimpl.GetSizeInBytesE(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

func (n *ntmConfig) logErrorNotImplemented(method string) error {
	err := fmt.Errorf("not implemented: %s", method)
	log.Error(err)
	return err
}

// GetSource wraps Viper for concurrent access
func (c *ntmConfig) GetSource(key string) model.Source {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	c.logErrorNotImplemented("GetSource")
	return model.SourceUnknown
}

// SetEnvPrefix wraps Viper for concurrent access, and keeps the envPrefix for
// future reference
func (c *ntmConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.envPrefix = in
}

// mergeWithEnvPrefix derives the environment variable that Viper will use for a given key.
// mergeWithEnvPrefix must be called while holding the config log (read or write).
func (c *ntmConfig) mergeWithEnvPrefix(key string) string {
	return strings.Join([]string{c.envPrefix, strings.ToUpper(key)}, "_")
}

// BindEnv wraps Viper for concurrent access, and adds tracking of the configurable env vars
func (c *ntmConfig) BindEnv(input ...string) {
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

	c.logErrorNotImplemented("BindEnv")
}

// SetEnvKeyReplacer wraps Viper for concurrent access
func (c *ntmConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("SetEnvKeyReplacer")
}

// UnmarshalKey wraps Viper for concurrent access
// DEPRECATED: use pkg/config/structure.UnmarshalKey instead
func (c *ntmConfig) UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return c.logErrorNotImplemented("UnmarshalKey")
}

// Unmarshal wraps Viper for concurrent access
func (c *ntmConfig) Unmarshal(rawVal interface{}) error {
	c.RLock()
	defer c.RUnlock()
	return c.logErrorNotImplemented("Unmarshal")
}

// UnmarshalExact wraps Viper for concurrent access
func (c *ntmConfig) UnmarshalExact(rawVal interface{}) error {
	c.RLock()
	defer c.RUnlock()
	return c.logErrorNotImplemented("UnmarshalExact")
}

// MergeConfig wraps Viper for concurrent access
func (c *ntmConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()
	return c.logErrorNotImplemented("MergeConfig")
}

// MergeFleetPolicy merges the configuration from the reader given with an existing config
// it overrides the existing values with the new ones in the FleetPolicies source, and updates the main config
// according to sources priority order.
//
// Note: this should only be called at startup, as notifiers won't receive a notification when this loads
func (c *ntmConfig) MergeFleetPolicy(configPath string) error {
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

	// TOOD: Implement merging, merge in the policy that was read
	return c.logErrorNotImplemented("MergeFleetPolicy")
}

// MergeConfigMap merges the configuration from the map given with an existing config.
// Note that the map given may be modified.
func (c *ntmConfig) MergeConfigMap(cfg map[string]any) error {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("AllSettings")
	return nil
}

// AllSettings wraps Viper for concurrent access
func (c *ntmConfig) AllSettings() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	c.logErrorNotImplemented("AllSettings")
	return nil
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *ntmConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()
	c.logErrorNotImplemented("AllSettingsWithoutDefault")
	return nil
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *ntmConfig) AllSettingsBySource() map[model.Source]interface{} {
	c.RLock()
	defer c.RUnlock()

	res := map[model.Source]interface{}{}
	c.logErrorNotImplemented("AllSettingsBySource")
	return res
}

// AddConfigPath wraps Viper for concurrent access
func (c *ntmConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("AddConfigPath")
}

// AddExtraConfigPaths allows adding additional configuration files
// which will be merged into the main configuration during the ReadInConfig call.
// Configuration files are merged sequentially. If a key already exists and the foreign value type matches the existing one, the foreign value overrides it.
// If both the existing value and the new value are nested configurations, they are merged recursively following the same principles.
func (c *ntmConfig) AddExtraConfigPaths(ins []string) error {
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
func (c *ntmConfig) SetConfigName(in string) {
	c.Lock()
	defer c.Unlock()
	c.configName = in
	c.configFile = ""
}

// SetConfigFile wraps Viper for concurrent access
func (c *ntmConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.configFile = in
}

// SetConfigType wraps Viper for concurrent access
func (c *ntmConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.configType = in
}

// ConfigFileUsed wraps Viper for concurrent access
func (c *ntmConfig) ConfigFileUsed() string {
	c.RLock()
	defer c.RUnlock()
	return c.configFile
}

func (c *ntmConfig) SetTypeByDefaultValue(in bool) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("SetTypeByDefaultValue")
}

// GetEnvVars implements the Config interface
func (c *ntmConfig) GetEnvVars() []string {
	c.RLock()
	defer c.RUnlock()
	vars := make([]string, 0, len(c.configEnvVars))
	for v := range c.configEnvVars {
		vars = append(vars, v)
	}
	return vars
}

// BindEnvAndSetDefault implements the Config interface
func (c *ntmConfig) BindEnvAndSetDefault(key string, val interface{}, env ...string) {
	c.SetDefault(key, val)
	c.BindEnv(append([]string{key}, env...)...) //nolint:errcheck
}

func (c *ntmConfig) Warnings() *model.Warnings {
	return nil
}

func (c *ntmConfig) Object() model.Reader {
	return c
}

// NewConfig returns a new Config object.
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) model.Config {
	config := ntmConfig{
		noimpl:        &notImplMethodsImpl{},
		configEnvVars: map[string]struct{}{},
		knownKeys:     map[string]struct{}{},
		unknownKeys:   map[string]struct{}{},
	}

	config.SetTypeByDefaultValue(true)
	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)

	return &config
}

// CopyConfig copies the given config to the receiver config. This should only be used in tests as replacing
// the global config reference is unsafe.
func (c *ntmConfig) CopyConfig(cfg model.Config) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("CopyConfig")
	if cfg, ok := cfg.(*ntmConfig); ok {
		// TODO: Probably a bug, should be a deep copy, add a test and verify
		c.root = cfg.root
		c.envPrefix = cfg.envPrefix
		c.envKeyReplacer = cfg.envKeyReplacer
		c.proxies = cfg.proxies
		c.configEnvVars = cfg.configEnvVars
		c.unknownKeys = cfg.unknownKeys
		c.notificationReceivers = cfg.notificationReceivers
		return
	}
	panic("Replacement config must be an instance of ntmConfig")
}

// GetProxies returns the proxy settings from the configuration
func (c *ntmConfig) GetProxies() *model.Proxy {
	c.Lock()
	hasProxies := c.proxies
	c.Unlock()
	if hasProxies != nil {
		return hasProxies
	}
	if c.GetBool("fips.enabled") {
		return nil
	}
	if !c.IsSet("proxy.http") && !c.IsSet("proxy.https") && !c.IsSet("proxy.no_proxy") {
		return nil
	}
	p := &model.Proxy{
		HTTP:    c.GetString("proxy.http"),
		HTTPS:   c.GetString("proxy.https"),
		NoProxy: c.GetStringSlice("proxy.no_proxy"),
	}
	c.proxies = p
	return c.proxies
}

func (c *ntmConfig) ExtraConfigFilesUsed() []string {
	c.Lock()
	defer c.Unlock()
	res := make([]string, len(c.extraConfigFilePaths))
	copy(res, c.extraConfigFilePaths)
	return res
}
