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
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sources lists the known sources, following the order of hierarchy between them
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
	noimpl notImplementedMethods

	// ready is whether the schema has been built, which marks the config as ready for use
	ready *atomic.Bool
	// defaults contains the settings with a default value
	defaults InnerNode
	// file contains the settings pulled from YAML files
	file InnerNode
	// root contains the final configuration, it's the result of merging all other tree by ordre of priority
	root InnerNode

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

	// yamlWarnings contains a list of warnings about loaded YAML file.
	// TODO: remove 'findUnknownKeys' function from pkg/config/setup in favor of those warnings. We should return
	// them from ReadConfig and ReadInConfig.
	warnings []string
}

// NodeTreeConfig is an interface that gives access to nodes
type NodeTreeConfig interface {
	GetNode(string) (Node, error)
}

// OnUpdate adds a callback to the list of receivers to be called each time a value is changed in the configuration
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

func (c *ntmConfig) setValueSource(key string, newValue interface{}, source model.Source) { // nolint: unused // TODO fix
	err := c.leafAtPath(key).SetWithSource(newValue, source)
	if err != nil {
		log.Errorf("%s", err)
	}
}

func (c *ntmConfig) set(key string, value interface{}, tree InnerNode, source model.Source) (bool, error) {
	parts := strings.Split(strings.ToLower(key), ",")
	return tree.SetAt(parts, value, source)
}

func (c *ntmConfig) setDefault(key string, value interface{}) {
	parts := strings.Split(strings.ToLower(key), ",")
	// TODO: Ensure that for default tree, setting nil to a node will not override
	// an existing value
	_, _ = c.defaults.SetAt(parts, value, model.SourceDefault)
}

// Set assigns the newValue to the given key and marks it as originating from the given source
func (c *ntmConfig) Set(key string, newValue interface{}, source model.Source) {
	var tree InnerNode

	// TODO: have a dedicated mapping in ntmConfig instead of a switch case
	// TODO: Default and File source should use SetDefault or ReadConfig instead. Once the defaults are handle we
	// should remove those two tree from here and consider this a bug.
	switch source {
	case model.SourceDefault:
		tree = c.defaults
	case model.SourceFile:
		tree = c.file
	}

	c.Lock()

	previousValue, _ := c.getValue(key)
	_, _ = c.set(key, newValue, tree, source)
	updated, _ := c.set(key, newValue, c.root, source)

	// if no value has changed we don't notify
	if !updated || reflect.DeepEqual(previousValue, newValue) {
		return
	}

	receivers := slices.Clone(c.notificationReceivers)
	c.Unlock()

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, previousValue, newValue)
	}
}

// SetWithoutSource assigns the value to the given key using source Unknown
func (c *ntmConfig) SetWithoutSource(key string, value interface{}) {
	c.Set(key, value, model.SourceUnknown)
}

// SetDefault assigns the value to the given key using source Default
func (c *ntmConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()

	if c.isReady() {
		panic("cannot SetDefault() once the config has been marked as ready for use")
	}
	key = strings.ToLower(key)
	c.knownKeys[key] = struct{}{}
	c.setDefault(key, value)
}

// UnsetForSource unsets a config entry for a given source
func (c *ntmConfig) UnsetForSource(_key string, _source model.Source) {
	c.Lock()
	c.logErrorNotImplemented("UnsetForSource")
	c.Unlock()
}

// SetKnown adds a key to the set of known valid config keys
func (c *ntmConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() {
		panic("cannot SetKnown() once the config has been marked as ready for use")
	}
	key = strings.ToLower(key)
	c.knownKeys[key] = struct{}{}
	c.setDefault(key, nil)
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

// BuildSchema is called when Setup is complete, and the config is ready to be used
func (c *ntmConfig) BuildSchema() {
	c.Lock()
	defer c.Unlock()
	c.ready.Store(true)
	// TODO: Build the environment variable tree
	// TODO: Instead of assigning defaultSource to root, merge the trees
	c.root = c.defaults
}

func (c *ntmConfig) isReady() bool {
	return c.ready.Load()
}

// ParseEnvAsStringSlice registers a transform function to parse an environment variable as a []string.
func (c *ntmConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsMapStringInterface registers a transform function to parse an environment variable as a map[string]interface{}
func (c *ntmConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSliceMapString registers a transform function to parse an environment variable as a []map[string]string
func (c *ntmConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// ParseEnvAsSlice registers a transform function to parse an environment variable as a []interface
func (c *ntmConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetEnvKeyTransformer(key, func(data string) interface{} { return fn(data) })
}

// SetFs assigns a filesystem to the config
func (c *ntmConfig) SetFs(fs afero.Fs) {
	c.Lock()
	defer c.Unlock()
	c.noimpl.SetFs(fs)
}

// IsSet checks if a key is set in the config
func (c *ntmConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.noimpl.IsSet(key)
}

// AllKeysLowercased returns all keys lower-cased
func (c *ntmConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()
	return c.noimpl.AllKeys()
}

func (c *ntmConfig) leafAtPath(key string) LeafNode {
	if !c.isReady() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return missingLeaf
	}

	pathParts := strings.Split(strings.ToLower(key), ".")
	var curr Node = c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return missingLeaf
		}
		curr = next
	}
	if leaf, ok := curr.(LeafNode); ok {
		return leaf
	}
	return missingLeaf
}

// GetNode returns a Node for the given key
func (c *ntmConfig) GetNode(key string) (Node, error) {
	if !c.isReady() {
		return nil, log.Errorf("attempt to read key before config is constructed: %s", key)
	}
	pathParts := strings.Split(key, ".")
	var curr Node = c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return nil, err
		}
		curr = next
	}
	return curr, nil
}

// Get returns a copy of the value for the given key
func (c *ntmConfig) Get(key string) interface{} {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := c.getValue(key)
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	// NOTE: should only need to deepcopy for `Get`, because it can be an arbitrary value,
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

// GetString returns a string-typed value for the given key
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

// GetBool returns a bool-typed value for the given key
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

// GetInt returns an int-typed value for the given key
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

// GetInt32 returns an int32-typed value for the given key
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

// GetInt64 returns an int64-typed value for the given key
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

// GetFloat64 returns a float64-typed value for the given key
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

// GetTime returns a time-typed value for the given key
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

// GetDuration returns a duration-typed value for the given key
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

// GetStringSlice returns a string slice value for the given key
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

// GetFloat64SliceE returns a float slice value for the given key, or an error
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

// GetStringMap returns a map[string]interface value for the given key
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

// GetStringMapString returns a map[string]string value for the given key
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

// GetStringMapStringSlice returns a map[string][]string value for the given key
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

// GetSizeInBytes returns the size in bytes of the filename for the given key
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

func (c *ntmConfig) logErrorNotImplemented(method string) error {
	err := fmt.Errorf("not implemented: %s", method)
	log.Error(err)
	return err
}

// GetSource returns the source of the given key
func (c *ntmConfig) GetSource(key string) model.Source {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	c.logErrorNotImplemented("GetSource")
	return model.SourceUnknown
}

// SetEnvPrefix sets the environment variable prefix to use
func (c *ntmConfig) SetEnvPrefix(in string) {
	c.Lock()
	defer c.Unlock()
	c.envPrefix = in
}

// mergeWithEnvPrefix derives the environment variable to use for a given key.
func (c *ntmConfig) mergeWithEnvPrefix(key string) string {
	return strings.Join([]string{c.envPrefix, strings.ToUpper(key)}, "_")
}

// BindEnv binds one or more environment variables to the given key
func (c *ntmConfig) BindEnv(key string, envvars ...string) {
	c.Lock()
	defer c.Unlock()

	if c.isReady() {
		panic("cannot BindEnv() once the config has been marked as ready for use")
	}
	key = strings.ToLower(key)

	// If only a key was given, with no associated envvars, then derive
	// an envvar from the key name
	if len(envvars) == 0 {
		envvars = []string{c.mergeWithEnvPrefix(key)}
	}

	for _, envvar := range envvars {
		// apply EnvKeyReplacer to each key
		if c.envKeyReplacer != nil {
			envvar = c.envKeyReplacer.Replace(envvar)
		}
		// TODO: Use envvar to build the envvar source tree
		c.configEnvVars[envvar] = struct{}{}
	}

	c.knownKeys[key] = struct{}{}
	c.setDefault(key, nil)
}

// SetEnvKeyReplacer binds a replacer function for keys
func (c *ntmConfig) SetEnvKeyReplacer(_r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("SetEnvKeyReplacer")
}

// UnmarshalKey unmarshals the data for the given key
// DEPRECATED: use pkg/config/structure.UnmarshalKey instead
func (c *ntmConfig) UnmarshalKey(key string, _rawVal interface{}, _opts ...viper.DecoderConfigOption) error {
	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return c.logErrorNotImplemented("UnmarshalKey")
}

// MergeConfig merges in another config
func (c *ntmConfig) MergeConfig(_in io.Reader) error {
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

	// TODO: Implement merging, merge in the policy that was read
	return c.logErrorNotImplemented("MergeFleetPolicy")
}

// MergeConfigMap merges the configuration from the map given with an existing config.
// Note that the map given may be modified.
func (c *ntmConfig) MergeConfigMap(_cfg map[string]any) error {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("AllSettings")
	return nil
}

// AllSettings returns all settings from the config
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

// AddConfigPath adds another config for the given path
func (c *ntmConfig) AddConfigPath(_in string) {
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

// SetConfigName sets the name of the config
func (c *ntmConfig) SetConfigName(in string) {
	c.Lock()
	defer c.Unlock()
	c.configName = in
	c.configFile = ""
}

// SetConfigFile sets the config file
func (c *ntmConfig) SetConfigFile(in string) {
	c.Lock()
	defer c.Unlock()
	c.configFile = in
}

// SetConfigType sets the type of the config
func (c *ntmConfig) SetConfigType(in string) {
	c.Lock()
	defer c.Unlock()
	c.configType = in
}

// ConfigFileUsed returns the config file
func (c *ntmConfig) ConfigFileUsed() string {
	c.RLock()
	defer c.RUnlock()
	return c.configFile
}

// SetTypeByDefaultValue enables typing using default values
func (c *ntmConfig) SetTypeByDefaultValue(_in bool) {
	c.Lock()
	defer c.Unlock()
	c.logErrorNotImplemented("SetTypeByDefaultValue")
}

// GetEnvVars gets all environment variables
func (c *ntmConfig) GetEnvVars() []string {
	c.RLock()
	defer c.RUnlock()
	vars := make([]string, 0, len(c.configEnvVars))
	for v := range c.configEnvVars {
		vars = append(vars, v)
	}
	return vars
}

// BindEnvAndSetDefault binds an environment variable and sets a default for the given key
func (c *ntmConfig) BindEnvAndSetDefault(key string, val interface{}, envvars ...string) {
	c.SetDefault(key, val)
	c.BindEnv(key, envvars...) //nolint:errcheck
}

// Warnings just returns nil
func (c *ntmConfig) Warnings() *model.Warnings {
	return nil
}

// Object returns the config as a Reader interface
func (c *ntmConfig) Object() model.Reader {
	return c
}

// NewConfig returns a new Config object.
func NewConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) model.Config {
	config := ntmConfig{
		ready:         atomic.NewBool(false),
		noimpl:        &notImplMethodsImpl{},
		configEnvVars: map[string]struct{}{},
		knownKeys:     map[string]struct{}{},
		unknownKeys:   map[string]struct{}{},
		defaults:      newInnerNodeImpl(),
		file:          newInnerNodeImpl(),
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

// ExtraConfigFilesUsed returns the additional config files used
func (c *ntmConfig) ExtraConfigFilesUsed() []string {
	c.Lock()
	defer c.Unlock()
	res := make([]string, len(c.extraConfigFilePaths))
	copy(res, c.extraConfigFilePaths)
	return res
}
