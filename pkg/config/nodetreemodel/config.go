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
	"strings"
	"sync"

	"path/filepath"

	"github.com/DataDog/viper"
	"go.uber.org/atomic"
	"golang.org/x/exp/maps"
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

	// ready is whether the schema has been built, which marks the config as ready for use
	ready *atomic.Bool

	// Bellow are all the different configuration layers. Each layers represents a source for our configuration.
	// They are merge into the 'root' tree following order of importance (see pkg/model/viper.go:sourcesPriority).

	// defaults contains the settings with a default value
	defaults InnerNode
	// unknown contains the settings set at runtime from unknown source. This should only evey be used by tests.
	unknown InnerNode
	// file contains the settings pulled from YAML files
	file InnerNode
	// envs contains config settings created by environment variables
	envs InnerNode
	// runtime contains the settings set from the agent code itself at runtime (self configured values).
	runtime InnerNode
	// localConfigProcess contains the settings pulled from the config process (process owning the source of truth
	// for the coniguration and mirrored by other processes).
	localConfigProcess InnerNode
	// remoteConfig contains the settings pulled from Remote Config.
	remoteConfig InnerNode
	// fleetPolicies contains the settings pulled from fleetPolicies.
	fleetPolicies InnerNode
	// cli contains the settings set by users at runtime through the CLI.
	cli InnerNode

	// root contains the final configuration, it's the result of merging all other tree by ordre of priority
	root InnerNode

	envPrefix      string
	envKeyReplacer *strings.Replacer
	envTransform   map[string]func(string) interface{}

	notificationReceivers []model.NotificationReceiver

	// Proxy settings
	proxies *model.Proxy

	configName string
	configFile string
	configType string

	// configEnvVars is the set of env vars that are consulted for
	// any given configuration key. Multiple env vars can be associated with one key
	configEnvVars map[string]string

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

func (c *ntmConfig) logErrorNotImplemented(method string) error {
	err := fmt.Errorf("not implemented: %s", method)
	log.Error(err)
	return err
}

// OnUpdate adds a callback to the list of receivers to be called each time a value is changed in the configuration
// by a call to the 'Set' method.
// Callbacks are only called if the value is effectively changed.
func (c *ntmConfig) OnUpdate(callback model.NotificationReceiver) {
	c.Lock()
	defer c.Unlock()
	c.notificationReceivers = append(c.notificationReceivers, callback)
}

func (c *ntmConfig) setDefault(key string, value interface{}) {
	parts := splitKey(key)
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
	case model.SourceUnknown:
		tree = c.unknown
	case model.SourceFile:
		tree = c.file
	case model.SourceEnvVar:
		tree = c.envs
	case model.SourceAgentRuntime:
		tree = c.runtime
	case model.SourceLocalConfigProcess:
		tree = c.localConfigProcess
	case model.SourceRC:
		tree = c.remoteConfig
	case model.SourceFleetPolicies:
		tree = c.fleetPolicies
	case model.SourceCLI:
		tree = c.cli
	default:
		log.Errorf("unknown source tree: %s\n", source)
	}

	if !c.IsKnown(key) {
		log.Errorf("could not set '%s' unknown key", key)
		return
	}

	c.Lock()
	previousValue := c.leafAtPath(key).Get()

	parts := splitKey(key)

	_, err := tree.SetAt(parts, newValue, source)
	if err != nil {
		log.Errorf("could not set '%s' invalid key: %s", key, err)
	}

	updated, err := c.root.SetAt(parts, newValue, source)
	if err != nil {
		log.Errorf("could not set '%s' invalid key: %s", key, err)
	}

	receivers := slices.Clone(c.notificationReceivers)
	c.Unlock()

	// if no value has changed we don't notify
	if !updated || reflect.DeepEqual(previousValue, newValue) {
		return
	}

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

func (c *ntmConfig) mergeAllLayers() error {
	treeList := []InnerNode{
		c.defaults,
		c.unknown,
		c.file,
		c.envs,
		c.fleetPolicies,
		c.runtime,
		c.localConfigProcess,
		c.remoteConfig,
		c.cli,
	}

	root := newInnerNode(nil)
	for _, tree := range treeList {
		err := root.Merge(tree)
		if err != nil {
			return err
		}
	}

	c.root = root
	return nil
}

// BuildSchema is called when Setup is complete, and the config is ready to be used
func (c *ntmConfig) BuildSchema() {
	c.Lock()
	defer c.Unlock()
	c.buildEnvVars()
	c.ready.Store(true)
	if err := c.mergeAllLayers(); err != nil {
		c.warnings = append(c.warnings, err.Error())
	}
}

func (c *ntmConfig) isReady() bool {
	return c.ready.Load()
}

func (c *ntmConfig) buildEnvVars() {
	root := newInnerNode(nil)
	envWarnings := []string{}
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) != 2 {
			continue
		}
		envkey := pair[0]
		envval := pair[1]

		if configKey, found := c.configEnvVars[envkey]; found {
			if err := c.insertNodeFromString(root, configKey, envval); err != nil {
				envWarnings = append(envWarnings, fmt.Sprintf("inserting env var: %s", err))
			}
		}
	}
	c.envs = root
	c.warnings = append(c.warnings, envWarnings...)
}

func (c *ntmConfig) insertNodeFromString(curr InnerNode, key string, envval string) error {
	var actualValue interface{} = envval
	// TODO: When the nodetreemodel config is further along, we should get the default[key] node
	// and use its type to convert the envval into something appropriate.
	if transformer, found := c.envTransform[key]; found {
		actualValue = transformer(envval)
	}
	parts := splitKey(key)
	_, err := curr.SetAt(parts, actualValue, model.SourceEnvVar)
	return err
}

// ParseEnvAsStringSlice registers a transform function to parse an environment variable as a []string.
func (c *ntmConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.Lock()
	defer c.Unlock()
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsMapStringInterface registers a transform function to parse an environment variable as a map[string]interface{}
func (c *ntmConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.Lock()
	defer c.Unlock()
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsSliceMapString registers a transform function to parse an environment variable as a []map[string]string
func (c *ntmConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.Lock()
	defer c.Unlock()
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsSlice registers a transform function to parse an environment variable as a []interface
func (c *ntmConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.Lock()
	defer c.Unlock()
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// IsSet checks if a key is set in the config
func (c *ntmConfig) IsSet(key string) bool {
	c.RLock()
	defer c.RUnlock()

	return c.IsKnown(key)
}

// AllKeysLowercased returns all keys lower-cased
func (c *ntmConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()

	return maps.Keys(c.knownKeys)
}

func (c *ntmConfig) leafAtPath(key string) LeafNode {
	if !c.isReady() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return missingLeaf
	}

	pathParts := splitKey(key)
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
	pathParts := splitKey(key)
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
		if c.envKeyReplacer != nil {
			envvar = c.envKeyReplacer.Replace(envvar)
		}
		c.configEnvVars[envvar] = key
	}

	c.knownKeys[key] = struct{}{}
	c.setDefault(key, nil)
}

// SetEnvKeyReplacer binds a replacer function for keys
func (c *ntmConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() {
		panic("cannot SetEnvKeyReplacer() once the config has been marked as ready for use")
	}
	c.envKeyReplacer = r
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

// AllSettings returns all settings from the config
func (c *ntmConfig) AllSettings() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	return c.root.DumpSettings(func(model.Source) bool { return true })
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *ntmConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.RLock()
	defer c.RUnlock()

	// We only want to include leaf with a source higher than SourceDefault
	return c.root.DumpSettings(func(source model.Source) bool { return source.IsGreaterOrEqualThan(model.SourceUnknown) })
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *ntmConfig) AllSettingsBySource() map[model.Source]interface{} {
	c.RLock()
	defer c.RUnlock()

	// We don't return include unknown settings
	return map[model.Source]interface{}{
		model.SourceDefault:            c.defaults.DumpSettings(func(model.Source) bool { return true }),
		model.SourceUnknown:            c.unknown.DumpSettings(func(model.Source) bool { return true }),
		model.SourceFile:               c.file.DumpSettings(func(model.Source) bool { return true }),
		model.SourceEnvVar:             c.envs.DumpSettings(func(model.Source) bool { return true }),
		model.SourceFleetPolicies:      c.fleetPolicies.DumpSettings(func(model.Source) bool { return true }),
		model.SourceAgentRuntime:       c.runtime.DumpSettings(func(model.Source) bool { return true }),
		model.SourceLocalConfigProcess: c.localConfigProcess.DumpSettings(func(model.Source) bool { return true }),
		model.SourceRC:                 c.remoteConfig.DumpSettings(func(model.Source) bool { return true }),
		model.SourceCLI:                c.cli.DumpSettings(func(model.Source) bool { return true }),
	}
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

// BindEnvAndSetDefault binds an environment variable and sets a default for the given key
func (c *ntmConfig) BindEnvAndSetDefault(key string, val interface{}, envvars ...string) {
	c.BindEnv(key, envvars...) //nolint:errcheck
	c.SetDefault(key, val)
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
		ready:              atomic.NewBool(false),
		configEnvVars:      map[string]string{},
		knownKeys:          map[string]struct{}{},
		unknownKeys:        map[string]struct{}{},
		defaults:           newInnerNode(nil),
		file:               newInnerNode(nil),
		unknown:            newInnerNode(nil),
		envs:               newInnerNode(nil),
		runtime:            newInnerNode(nil),
		localConfigProcess: newInnerNode(nil),
		remoteConfig:       newInnerNode(nil),
		fleetPolicies:      newInnerNode(nil),
		cli:                newInnerNode(nil),
		envTransform:       make(map[string]func(string) interface{}),
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

// ExtraConfigFilesUsed returns the additional config files used
func (c *ntmConfig) ExtraConfigFilesUsed() []string {
	c.Lock()
	defer c.Unlock()
	res := make([]string, len(c.extraConfigFilePaths))
	copy(res, c.extraConfigFilePaths)
	return res
}
