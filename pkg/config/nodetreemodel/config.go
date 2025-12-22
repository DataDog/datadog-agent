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
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sources lists the known sources, following the order of hierarchy between them
var sources = []model.Source{
	model.SourceDefault,
	model.SourceUnknown,
	model.SourceInfraMode,
	model.SourceFile,
	model.SourceEnvVar,
	model.SourceFleetPolicies,
	model.SourceAgentRuntime,
	model.SourceLocalConfigProcess,
	model.SourceRC,
	model.SourceCLI,
}

var splitKeyFunc = splitKey

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

	// flag only used by tests, allows tests to treat the config as though its schema was dynamic
	// - it's okay to read the config before its schema is built
	// - it's okay to modify the config schema after it gets built
	// - unknown keys can be assigned and retrieved
	allowDynamicSchema *atomic.Bool
	// state of env vars, only used by tests to decide when to rebuild the env var layer. Necessary because
	// viper would lookup env vars at runtime, instead of storing them, and many many tests rely on this behavior
	lastEnvVarState string

	// tree debugger is used by the Stringify method, useful for debugging and test assertions
	td *treeDebugger

	// defaults contains the settings with a default value
	defaults *nodeImpl
	// unknown contains the settings set at runtime from unknown source. This should only evey be used by tests.
	unknown *nodeImpl
	// infraMode contains the settings set by infrastructure mode configurations
	infraMode *nodeImpl
	// file contains the settings pulled from YAML files
	file *nodeImpl
	// envs contains config settings created by environment variables
	envs *nodeImpl
	// runtime contains the settings set from the agent code itself at runtime (self configured values).
	runtime *nodeImpl
	// localConfigProcess contains the settings pulled from the config process (process owning the source of truth
	// for the coniguration and mirrored by other processes).
	localConfigProcess *nodeImpl
	// remoteConfig contains the settings pulled from Remote Config.
	remoteConfig *nodeImpl
	// fleetPolicies contains the settings pulled from fleetPolicies.
	fleetPolicies *nodeImpl
	// cli contains the settings set by users at runtime through the CLI.
	cli *nodeImpl

	// root contains the final configuration, it's the result of merging all other tree by ordre of priority
	root *nodeImpl

	envPrefix      string
	envKeyReplacer *strings.Replacer
	envTransform   map[string]func(string) interface{}

	notificationReceivers []model.NotificationReceiver
	sequenceID            uint64

	// Proxy settings
	proxies *model.Proxy

	configName string
	configFile string
	configType string
	// configPaths is the set of path to look for the configuration file
	configPaths []string

	// configEnvVars is the set of env vars that are consulted for
	// any given configuration key. Multiple env vars can be associated with one key
	configEnvVars map[string][]string

	// known keys are the set of valid keys to get either leaf or inner node values
	// they are defined by one of (1) SetDefault (2) BindEnv (3) SetKnown
	// the map value represents `isLeaf` for each key
	knownKeys map[string]bool

	// keys that are unknown, but are used by either the file or SetWithoutSource
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}

	// extraConfigFilePaths represents additional configuration file paths that will be merged into the main configuration when ReadInConfig() is called.
	extraConfigFilePaths []string

	// yamlWarnings contains a list of warnings about loaded YAML file.
	// TODO: remove 'findUnknownKeys' function from pkg/config/setup in favor of those warnings. We should return
	// them from ReadConfig and ReadInConfig.
	warnings []error
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

func (c *ntmConfig) getTreeBySource(source model.Source) (*nodeImpl, error) {
	switch source {
	case "root":
		return c.root, nil
	case model.SourceDefault:
		return c.defaults, nil
	case model.SourceUnknown:
		return c.unknown, nil
	case model.SourceInfraMode:
		return c.infraMode, nil
	case model.SourceFile:
		return c.file, nil
	case model.SourceEnvVar:
		return c.envs, nil
	case model.SourceAgentRuntime:
		return c.runtime, nil
	case model.SourceLocalConfigProcess:
		return c.localConfigProcess, nil
	case model.SourceRC:
		return c.remoteConfig, nil
	case model.SourceFleetPolicies:
		return c.fleetPolicies, nil
	case model.SourceCLI:
		return c.cli, nil
	}
	return nil, fmt.Errorf("invalid source: %s", source)
}

// SetTestOnlyDynamicSchema allows more flexible usage of the config, should only be used by tests
func (c *ntmConfig) SetTestOnlyDynamicSchema(allow bool) {
	c.allowDynamicSchema.Store(allow)
}

// RevertFinishedBackToBuilder returns an interface that can build more on the current
// config, instead of treating it as sealed
// NOTE: Only used by OTel, no new uses please!
func (c *ntmConfig) RevertFinishedBackToBuilder() model.BuildableConfig {
	c.ready.Store(false)
	return c
}

// Set assigns the newValue to the given key and marks it as originating from the given source
func (c *ntmConfig) Set(key string, newValue interface{}, source model.Source) {
	if source == model.SourceEnvVar {
		panicInTest("Writing to env var layers is not allowed, use SourceAgentRuntime instead.")
	}

	c.maybeRebuild()

	c.Lock()

	if !c.isKnownKey(key) {
		if c.allowDynamicSchema.Load() {
			log.Errorf("set value for unknown key '%s'", key)
		} else {
			log.Errorf("could not set '%s' unknown key", key)
			c.Unlock()
			return
		}
	}
	declaredNode := c.nodeAtPathFromNode(key, c.defaults)
	if declaredNode.IsInnerNode() {
		panicInTest("Key '%s' is partial path of a setting. 'Set' does not allow configuring multiple settings at once using maps", key)
		c.Unlock()
		return
	}

	// convert the key to lower case for the logs line and the notification
	key = strings.ToLower(key)

	previousValue := c.leafAtPathFromNode(key, c.root).Get()

	newTree, err := c.insertValueIntoTree(key, newValue, source)
	if err != nil {
		log.Errorf("could not insert value: %s", err)
		c.Unlock()
		return
	} else if newTree != nil {
		// a new node was allocated, merge it into root
		c.root, _ = c.root.Merge(newTree)
	}

	receivers := slices.Clone(c.notificationReceivers)

	// if no value has changed we don't notify
	if reflect.DeepEqual(previousValue, newValue) {
		c.Unlock()
		return
	}

	c.sequenceID++
	c.Unlock()

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, source, previousValue, newValue, c.sequenceID)
	}
}

func (c *ntmConfig) insertValueIntoTree(key string, value interface{}, source model.Source) (*nodeImpl, error) {
	tree, err := c.getTreeBySource(source)
	if err != nil {
		return nil, log.Errorf("Set invalid source: %s", source)
	}

	parts := splitKey(key)
	err = tree.setAt(parts, value, source)
	return tree, err
}

// SetWithoutSource assigns the value to the given key using source Unknown, may only be called from tests
func (c *ntmConfig) SetWithoutSource(key string, value interface{}) {
	c.assertIsTest("SetWithoutSource")
	if !viperconfig.ValidateBasicTypes(value) {
		panic(fmt.Errorf("SetWithoutSource can only be called with basic types (int, string, slice, map, etc), got %v", value))
	}
	c.Set(key, value, model.SourceUnknown)
	c.Lock()
	defer c.Unlock()
	if !c.isKnownKey(key) {
		c.unknownKeys[key] = struct{}{}
	}
}

// SetDefault assigns the value to the given key using source Default
func (c *ntmConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()

	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetDefault() once the config has been marked as ready for use")
	}

	key = strings.ToLower(key)
	c.setDefault(key, value)
	c.addToKnownKeys(key)
}

func (c *ntmConfig) setDefault(key string, value interface{}) {
	parts := splitKey(key)
	_ = c.defaults.setAt(parts, value, model.SourceDefault)
}

func (c *ntmConfig) findPreviousSourceNode(key string, source model.Source) (*nodeImpl, error) {
	iter := source
	for iter != model.SourceDefault {
		iter = iter.PreviousSource()
		tree, err := c.getTreeBySource(iter)
		if err != nil {
			return nil, err
		}
		node := c.leafAtPathFromNode(key, tree)
		if node != missingLeaf {
			return node, nil
		}
	}
	return nil, ErrNotFound
}

// UnsetForSource unsets a config entry for a given source
func (c *ntmConfig) UnsetForSource(key string, source model.Source) {
	c.Lock()
	defer c.Unlock()

	key = strings.ToLower(key)
	previousValue := c.leafAtPathFromNode(key, c.root).Get()

	// Remove it from the original source tree
	tree, err := c.getTreeBySource(source)
	if err != nil {
		log.Errorf("%s", err)
		return
	}
	parentNode, childName, err := c.parentOfNode(tree, key)
	if err != nil {
		return
	}
	// Only remove if the setting is a leaf
	if child, err := parentNode.GetChild(childName); err == nil {
		if child.IsLeafNode() {
			parentNode.RemoveChild(childName)
		} else {
			log.Errorf("cannot remove setting %q, not a leaf", key)
			return
		}
	}

	// If the node in the merged tree doesn't match the source we expect, we're done
	if c.leafAtPathFromNode(key, c.root).Source() != source {
		return
	}

	// Find what the previous value used to be, based upon the previous source
	prevNode, findPreviousSourceError := c.findPreviousSourceNode(key, source)

	// Get the parent node of the leaf we're unsetting
	parentNode, childName, err = c.parentOfNode(c.root, key)
	if err != nil {
		return
	}

	// If there was no previous source with a node of this name, simply remove it from the parent
	if findPreviousSourceError != nil {
		parentNode.RemoveChild(childName)
		return
	}

	// Replace the child with the node from the previous layer
	parentNode.InsertChildNode(childName, prevNode.Clone())

	newValue := c.leafAtPathFromNode(key, c.root).Get()

	// Value has not changed, do not notify
	if reflect.DeepEqual(previousValue, newValue) {
		return
	}

	c.sequenceID++
	receivers := slices.Clone(c.notificationReceivers)

	// notifying all receiver about the updated setting
	for _, receiver := range receivers {
		receiver(key, source, previousValue, newValue, c.sequenceID)
	}
}

func (c *ntmConfig) parentOfNode(node *nodeImpl, key string) (*nodeImpl, string, error) {
	parts := splitKey(key)
	lastPart := parts[len(parts)-1]
	parts = parts[:len(parts)-1]
	var err error
	for _, p := range parts {
		node, err = node.GetChild(p)
		if err != nil {
			return nil, "", err
		}
	}
	if node.IsLeafNode() {
		return nil, "", ErrNotFound
	}
	return node, lastPart, nil
}

func (c *ntmConfig) addToKnownKeys(key string) {
	if _, ok := c.knownKeys[key]; ok {
		return
	}
	base := ""
	keyParts := splitKey(key)
	for i, part := range keyParts {
		base = joinKey(base, part)
		// Set true if leaf, false for inner nodes
		c.knownKeys[base] = i == len(keyParts)-1
	}
}

// SetKnown adds a key to the set of known valid config keys.
//
// Important: this doesn't add the key to the default layer. The "known keys" are a legacy feature we inherited from our Viper
// wrapper. Once all settings have a default we'll be able to remove this concept entirely.
func (c *ntmConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetKnown() once the config has been marked as ready for use")
	}
	key = strings.ToLower(key)
	c.addToKnownKeys(key)
}

// IsKnown returns whether a key is in the set of "known keys", which is a legacy feature from Viper
func (c *ntmConfig) IsKnown(key string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.isKnownKey(key)
}

// isKnownKey returns whether the key is known.
// Must be called with the lock read-locked.
func (c *ntmConfig) isKnownKey(key string) bool {
	key = strings.ToLower(key)
	_, found := c.knownKeys[key]
	return found
}

func (c *ntmConfig) maybeRebuild() {
	if c.allowDynamicSchema.Load() {
		// Write-lock because the root will be written to in order to rebuild the state
		c.Lock()
		defer c.Unlock()

		// Only need to rebuild if env vars have different state than last rebuild
		envs := os.Environ()
		sort.Strings(envs)
		envVarState := strings.Join(envs, "$")
		if c.lastEnvVarState == envVarState {
			return
		}
		c.lastEnvVarState = envVarState

		// building the schema may access data from the config, disable the dynamic schema
		// flag to prevent recursive rebuilds
		c.allowDynamicSchema.Store(false)
		defer func() { c.allowDynamicSchema.Store(true) }()

		c.buildSchema()
	}
}

// checkKnownKey checks if a key is known, and if not logs a warning
// Only a single warning will be logged per unknown key.
//
// Must be called with the lock read-locked.
func (c *ntmConfig) checkKnownKey(key string) {
	if c.isKnownKey(key) {
		return
	}

	key = strings.ToLower(key)
	if _, ok := c.unknownKeys[key]; ok {
		return
	}

	c.unknownKeys[key] = struct{}{}
	log.Warnf("config key %v is unknown", key)
}

func (c *ntmConfig) mergeAllLayers() error {
	treeList := []*nodeImpl{
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

	merged := newInnerNode(nil)
	for _, tree := range treeList {
		next, err := merged.Merge(tree)
		if err != nil {
			return err
		}
		merged = next
	}

	c.root = merged
	return nil
}

// BuildSchema is called when Setup is complete, and the config is ready to be used
func (c *ntmConfig) BuildSchema() {
	c.Lock()
	defer c.Unlock()
	c.buildSchema()
}

func (c *ntmConfig) buildSchema() {
	c.buildEnvVars()
	c.ready.Store(true)
	if err := c.mergeAllLayers(); err != nil {
		c.warnings = append(c.warnings, err)
	}
}

// Stringify stringifies the config, but only with the test build tag
func (c *ntmConfig) Stringify(source model.Source, opts ...model.StringifyOption) string {
	c.Lock()
	defer c.Unlock()
	// only does anything if the build tag "test" is enabled
	text, err := c.toDebugString(source, opts...)
	if err != nil {
		return fmt.Sprintf("Stringify error: %s", err)
	}
	return text
}

func (c *ntmConfig) isReady() bool {
	return c.ready.Load()
}

func (c *ntmConfig) buildEnvVars() {
	root := newInnerNode(nil)
	envWarnings := []error{}

	for configKey, listEnvVars := range c.configEnvVars {
		for _, envVar := range listEnvVars {
			if value, ok := os.LookupEnv(envVar); ok && value != "" {
				if err := c.insertNodeFromString(root, configKey, value); err != nil {
					envWarnings = append(envWarnings, err)
				} else {
					// Stop looping since we set the config key with the value of the highest precedence env var
					break
				}
			}
		}
	}
	c.envs = root
	c.warnings = append(c.warnings, envWarnings...)
}

func (c *ntmConfig) insertNodeFromString(curr *nodeImpl, key string, envval string) error {
	var actualValue interface{} = envval
	// TODO: When nodetreemodel has a schema with type information, we should
	// use this type to convert the value, instead of requiring a transformer
	if transformer, found := c.envTransform[key]; found {
		actualValue = transformer(envval)
	}
	parts := splitKeyFunc(key)
	return curr.setAt(parts, actualValue, model.SourceEnvVar)
}

// ParseEnvAsStringSlice registers a transform function to parse an environment variable as a []string.
func (c *ntmConfig) ParseEnvAsStringSlice(key string, fn func(string) []string) {
	c.Lock()
	defer c.Unlock()
	if _, exists := c.envTransform[strings.ToLower(key)]; exists {
		panic(fmt.Sprintf("env transform for %s already exists", key))
	}
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsMapStringInterface registers a transform function to parse an environment variable as a map[string]interface{}
func (c *ntmConfig) ParseEnvAsMapStringInterface(key string, fn func(string) map[string]interface{}) {
	c.Lock()
	defer c.Unlock()
	if _, exists := c.envTransform[strings.ToLower(key)]; exists {
		panic(fmt.Sprintf("env transform for %s already exists", key))
	}
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsSliceMapString registers a transform function to parse an environment variable as a []map[string]string
func (c *ntmConfig) ParseEnvAsSliceMapString(key string, fn func(string) []map[string]string) {
	c.Lock()
	defer c.Unlock()
	if _, exists := c.envTransform[strings.ToLower(key)]; exists {
		panic(fmt.Sprintf("env transform for %s already exists", key))
	}
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// ParseEnvAsSlice registers a transform function to parse an environment variable as a []interface
func (c *ntmConfig) ParseEnvAsSlice(key string, fn func(string) []interface{}) {
	c.Lock()
	defer c.Unlock()
	if _, exists := c.envTransform[strings.ToLower(key)]; exists {
		panic(fmt.Sprintf("env transform for %s already exists", key))
	}
	c.envTransform[strings.ToLower(key)] = func(k string) interface{} { return fn(k) }
}

// IsSet checks if a key is set in the config
func (c *ntmConfig) IsSet(key string) bool {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()

	if !c.isReady() && !c.allowDynamicSchema.Load() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return false
	}

	pathParts := splitKey(key)
	curr := c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return false
		}
		curr = next
	}
	return true
}

func hasNonDefaultLeaf(node *nodeImpl) bool {
	// We're on an InnerNode, we need to check if any child leaf are not defaults
	for _, name := range node.ChildrenKeys() {
		child, _ := node.GetChild(name)
		if child.IsLeafNode() {
			// Leaf has to be on a non-default layer and have a non-nil value
			if child.Source().IsGreaterThan(model.SourceDefault) && child.Get() != nil {
				return true
			}
			continue
		}
		if hasNonDefaultLeaf(child) {
			return true
		}
	}
	return false
}

// IsConfigured checks if a key is set in the config but not from the defaults
func (c *ntmConfig) IsConfigured(key string) bool {
	c.RLock()
	defer c.RUnlock()

	if !c.isReady() && !c.allowDynamicSchema.Load() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return false
	}

	pathParts := splitKey(key)
	curr := c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return false
		}
		curr = next
	}
	// if key is a leaf, we just check the source
	if curr.IsLeafNode() {
		return curr.Source().IsGreaterThan(model.SourceDefault) && curr.Get() != nil
	}

	// if the key was an InnerNode we need to check all the inner leaf node to check if one was set by the user
	return hasNonDefaultLeaf(curr)
}

func isInnerOrLeafWithNilValue(node *nodeImpl) bool {
	if node == missingLeaf {
		return false
	}
	if node.IsInnerNode() {
		return true
	}
	if node.IsLeafNode() {
		return node.Get() == nil
	}
	return false
}

// HasSection returns true if the setting is either an inner node,
// or a leaf node with a nil value
func (c *ntmConfig) HasSection(key string) bool {
	c.RLock()
	defer c.RUnlock()

	for _, src := range model.Sources {
		if src == model.SourceDefault {
			continue
		}
		tree, _ := c.getTreeBySource(src)
		if isInnerOrLeafWithNilValue(c.nodeAtPathFromNode(key, tree)) {
			return true
		}
	}
	return false
}

// AllKeysLowercased returns all keys, including unknown keys and those without default values
// Unlike AllSettings, this returns keys defined by SetKnown or BindEnv
func (c *ntmConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()

	// collect keys from known set
	allKeys := map[string]struct{}{}
	for k, isLeaf := range c.knownKeys {
		if isLeaf {
			allKeys[k] = struct{}{}
		}
	}
	// collect keys from unknown set
	for k := range c.unknownKeys {
		allKeys[k] = struct{}{}
	}

	keylist := slices.Collect(maps.Keys(allKeys))
	sort.Strings(keylist)
	return keylist
}

func (c *ntmConfig) leafAtPathFromNode(key string, curr *nodeImpl) *nodeImpl {
	node := c.nodeAtPathFromNode(key, curr)
	if node.IsLeafNode() {
		return node
	}
	return missingLeaf
}

func (c *ntmConfig) nodeAtPathFromNode(key string, curr *nodeImpl) *nodeImpl {
	pathParts := splitKey(key)
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return missingLeaf
		}
		curr = next
	}
	return curr
}

// GetNode returns a *nodeImpl for the given key
func (c *ntmConfig) GetNode(key string) (Node, error) {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
		return nil, log.Errorf("attempt to read key before config is constructed: %s", key)
	}
	pathParts := splitKey(key)
	curr := c.root
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
	c.envTransform = make(map[string]func(string) interface{})
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

	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot BindEnv() once the config has been marked as ready for use")
	}

	key = strings.ToLower(key)
	c.bindEnv(key, envvars)
	c.addToKnownKeys(key)
}

func (c *ntmConfig) bindEnv(key string, envvars []string) {
	// If only a key was given, with no associated envvars, then derive
	// an envvar from the key name
	if len(envvars) == 0 {
		envvars = []string{c.mergeWithEnvPrefix(key)}
	}

	for _, envvar := range envvars {
		if c.envKeyReplacer != nil {
			envvar = c.envKeyReplacer.Replace(envvar)
		}
		c.configEnvVars[key] = append(c.configEnvVars[key], envvar)
	}
}

// SetEnvKeyReplacer binds a replacer function for keys
func (c *ntmConfig) SetEnvKeyReplacer(r *strings.Replacer) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetEnvKeyReplacer() once the config has been marked as ready for use")
	}
	c.envKeyReplacer = r
}

// MergeConfig merges in another config
func (c *ntmConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()

	if !c.isReady() && !c.allowDynamicSchema.Load() {
		return errors.New("attempt to MergeConfig before config is constructed")
	}

	content, err := io.ReadAll(in)
	if err != nil {
		return err
	}

	other := newInnerNode(nil)
	if err = c.readConfigurationContent(other, model.SourceFile, content); err != nil {
		return err
	}

	merged, err := c.root.Merge(other)
	if err != nil {
		return err
	}
	c.root = merged
	return nil
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

	content, err := io.ReadAll(in)
	if err != nil {
		return err
	}

	other := newInnerNode(nil)
	if err = c.readConfigurationContent(other, model.SourceFleetPolicies, content); err != nil {
		return err
	}

	merged, err := c.root.Merge(other)
	if err != nil {
		return err
	}
	c.root = merged
	return nil
}

// AllSettings returns all settings from the config
func (c *ntmConfig) AllSettings() map[string]interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()

	return c.root.dumpSettings(true)
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *ntmConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()

	// Dump settings but don't include defaults
	return c.root.dumpSettings(false)
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *ntmConfig) AllSettingsBySource() map[model.Source]interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()

	// We don't return include unknown settings
	return map[model.Source]interface{}{
		model.SourceDefault:            c.defaults.dumpSettings(true),
		model.SourceUnknown:            c.unknown.dumpSettings(true),
		model.SourceInfraMode:          c.infraMode.dumpSettings(true),
		model.SourceFile:               c.file.dumpSettings(true),
		model.SourceEnvVar:             c.envs.dumpSettings(true),
		model.SourceFleetPolicies:      c.fleetPolicies.dumpSettings(true),
		model.SourceAgentRuntime:       c.runtime.dumpSettings(true),
		model.SourceLocalConfigProcess: c.localConfigProcess.dumpSettings(true),
		model.SourceRC:                 c.remoteConfig.dumpSettings(true),
		model.SourceCLI:                c.cli.dumpSettings(true),
		model.SourceProvided:           c.root.dumpSettings(false),
	}
}

// AllSettingsWithSequenceID returns the settings and the sequence ID.
func (c *ntmConfig) AllSettingsWithSequenceID() (map[string]interface{}, uint64) {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	return c.root.dumpSettings(true), c.sequenceID
}

// AddConfigPath adds another config for the given path
func (c *ntmConfig) AddConfigPath(in string) {
	c.Lock()
	defer c.Unlock()

	if !filepath.IsAbs(in) {
		var err error
		in, err = filepath.Abs(in)
		if err != nil {
			log.Errorf("could not get absolute path for configuration %q: %s", in, err)
			return
		}
	}

	in = filepath.Clean(in)
	if !slices.Contains(c.configPaths, in) {
		c.configPaths = append(c.configPaths, in)
	}
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

// GetSubfields returns the names of child fields of this setting
func (c *ntmConfig) GetSubfields(key string) []string {
	n, err := c.GetNode(key)
	if err != nil {
		return nil
	}
	if n.IsInnerNode() {
		return n.ChildrenKeys()
	}
	return nil
}

// BindEnvAndSetDefault fully declares a setting with a default value and optional env var overrides
// If no env vars are declared, one will be derived from the key name
// This is the preferred method to declare a setting
func (c *ntmConfig) BindEnvAndSetDefault(key string, defaultVal interface{}, envvars ...string) {
	c.Lock()
	defer c.Unlock()

	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetDefault() once the config has been marked as ready for use")
	}

	c.bindEnv(key, envvars)
	c.setDefault(key, defaultVal)
	c.addToKnownKeys(key)
}

// Warnings just returns nil
func (c *ntmConfig) Warnings() *model.Warnings {
	return &model.Warnings{Errors: c.warnings}
}

// Object returns the config as a Reader interface
func (c *ntmConfig) Object() model.Reader {
	return c
}

// NewNodeTreeConfig returns a new Config object.
func NewNodeTreeConfig(name string, envPrefix string, envKeyReplacer *strings.Replacer) model.BuildableConfig {
	config := ntmConfig{
		ready:              atomic.NewBool(false),
		allowDynamicSchema: atomic.NewBool(false),
		sequenceID:         0,
		configEnvVars:      map[string][]string{},
		knownKeys:          map[string]bool{},
		unknownKeys:        map[string]struct{}{},
		defaults:           newInnerNode(nil),
		file:               newInnerNode(nil),
		unknown:            newInnerNode(nil),
		infraMode:          newInnerNode(nil),
		envs:               newInnerNode(nil),
		runtime:            newInnerNode(nil),
		localConfigProcess: newInnerNode(nil),
		remoteConfig:       newInnerNode(nil),
		fleetPolicies:      newInnerNode(nil),
		cli:                newInnerNode(nil),
		root:               newInnerNode(nil),
		envTransform:       make(map[string]func(string) interface{}),
		configName:         "datadog",
	}

	config.SetConfigName(name)
	config.SetEnvPrefix(envPrefix)
	config.SetEnvKeyReplacer(envKeyReplacer)

	return &config
}

// ExtraConfigFilesUsed returns the additional config files used
func (c *ntmConfig) ExtraConfigFilesUsed() []string {
	c.Lock()
	defer c.Unlock()
	res := make([]string, len(c.extraConfigFilePaths))
	copy(res, c.extraConfigFilePaths)
	return res
}

func (c *ntmConfig) GetSequenceID() uint64 {
	c.RLock()
	defer c.RUnlock()
	return c.sequenceID
}
