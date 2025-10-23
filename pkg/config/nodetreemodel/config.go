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

	mapstructure "github.com/go-viper/mapstructure/v2"

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

	// schema holds all the settings with or without value. Settings are added to the schema through BindEnv and
	// SetDefault.
	//
	// This solved the difference between 'AllKeysLowercased' which returns the configuration schema and
	// 'AllSettings' which only returns settings with a value.
	//
	// A setting register with BindEnv without default might not have a value depending on the environment. Such
	// settings are part of the schema but won't appear in the configuration (through Get, AllSettings, ...). This
	// mimic the behavior from Viper. Once we enfore a default value for all settings we will be able to merge
	// 'schema' and 'defaults' fields.
	schema InnerNode

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

	// known keys are all the keys that meet at least one of these criteria:
	// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
	knownKeys map[string]struct{}
	// keys that have been used but are unknown
	// used to warn (a single time) on use
	unknownKeys map[string]struct{}
	// allSettings contains all settings that we have a value for in the default tree
	allSettings []string

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

func (c *ntmConfig) addToSchema(key string, source model.Source) {
	parts := splitKey(key)
	_ = c.schema.SetAt(parts, nil, source)
	c.addToKnownKeys(key)
}

func (c *ntmConfig) getTreeBySource(source model.Source) (InnerNode, error) {
	switch source {
	case "root":
		return c.root, nil
	case model.SourceDefault:
		return c.defaults, nil
	case model.SourceUnknown:
		return c.unknown, nil
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
	schemaNode := c.nodeAtPathFromNode(key, c.schema)
	if _, ok := schemaNode.(LeafNode); schemaNode != missingLeaf && !ok {
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
		c.root, _ = c.root.Merge(newTree.(InnerNode))
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

func (c *ntmConfig) insertValueIntoTree(key string, value interface{}, source model.Source) (Node, error) {
	tree, err := c.getTreeBySource(source)
	if err != nil {
		return nil, log.Errorf("Set invalid source: %s", source)
	}

	parts := splitKey(key)
	err = tree.SetAt(parts, value, source)
	return tree, err
}

// SetWithoutSource assigns the value to the given key using source Unknown, may only be called from tests
func (c *ntmConfig) SetWithoutSource(key string, value interface{}) {
	c.assertIsTest("SetWithoutSource")

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		panic("SetWithoutSource cannot assign struct to a setting")
	}
	c.Set(key, value, model.SourceUnknown)

	c.Lock()
	defer c.Unlock()
	c.computeAllSettings()
}

// SetDefault assigns the value to the given key using source Default
func (c *ntmConfig) SetDefault(key string, value interface{}) {
	c.Lock()
	defer c.Unlock()

	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetDefault() once the config has been marked as ready for use")
	}
	key = strings.ToLower(key)
	c.addToSchema(key, model.SourceDefault)

	parts := splitKey(key)
	// TODO: Ensure that for default tree, setting nil to a node will not override
	// an existing value
	_ = c.defaults.SetAt(parts, value, model.SourceDefault)
}

func (c *ntmConfig) findPreviousSourceNode(key string, source model.Source) (Node, error) {
	iter := source
	for iter != model.SourceDefault {
		iter = iter.PreviousSource()
		tree, err := c.getTreeBySource(iter)
		if err != nil {
			return nil, err
		}
		node := c.leafAtPathFromNode(key, tree)
		if _, isMissing := node.(*missingLeafImpl); !isMissing {
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
		if _, ok := child.(LeafNode); ok {
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

func (c *ntmConfig) parentOfNode(node Node, key string) (InnerNode, string, error) {
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
	innerNode, ok := node.(InnerNode)
	if !ok {
		return nil, "", ErrNotFound
	}
	return innerNode, lastPart, nil
}

func (c *ntmConfig) addToKnownKeys(key string) {
	base := ""
	keyParts := splitKey(key)
	for _, part := range keyParts {
		base = joinKey(base, part)
		c.knownKeys[base] = struct{}{}
	}
}

// SetKnown adds a key to the set of known valid config keys.
//
// Important: this doesn't add the key to the schema. The "known keys" are a legacy feature we inherited from our Viper
// wrapper. Once all settings have a default we'll be able to remove this concept entirely.
func (c *ntmConfig) SetKnown(key string) {
	c.Lock()
	defer c.Unlock()
	if c.isReady() && !c.allowDynamicSchema.Load() {
		panic("cannot SetKnown() once the config has been marked as ready for use")
	}

	c.addToSchema(key, model.SourceSchema)
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
	// We intentionally don't merge the schema layer as it hold no values
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

	var merged InnerNode = newInnerNode(nil)
	for _, tree := range treeList {
		next, err := merged.Merge(tree)
		if err != nil {
			return err
		}
		merged = next
	}

	c.root = merged
	// recompile allSettings now that we have the full config
	c.computeAllSettings()
	return nil
}

func (c *ntmConfig) computeAllSettings() {
	keySet := make(map[string]struct{})

	// 1. Collect all known keys from schema
	c.collectKeysFromNode(c.schema, "", keySet, true)

	// 2. Collect all keys from merged tree (only ones with values)
	c.collectKeysFromNode(c.root, "", keySet, false)

	allKeys := slices.Collect(maps.Keys(keySet))
	slices.Sort(allKeys)
	c.allSettings = allKeys
}

func (c *ntmConfig) collectKeysFromNode(node InnerNode, path string, keySet map[string]struct{}, includeAllKeys bool) {
	for _, name := range node.ChildrenKeys() {
		newPath := joinKey(path, name)

		child, _ := node.GetChild(name)

		if leaf, ok := child.(LeafNode); ok {
			// Include all keys if requested
			// For other nodes, only include if they have actual values
			if includeAllKeys || leaf != missingLeaf {
				keySet[newPath] = struct{}{}
			}
		} else if inner, ok := child.(InnerNode); ok {
			c.collectKeysFromNode(inner, newPath, keySet, includeAllKeys)
		}
	}
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
	c.computeAllSettings()
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

func (c *ntmConfig) insertNodeFromString(curr InnerNode, key string, envval string) error {
	var actualValue interface{} = envval
	// TODO: When nodetreemodel has a schema with type information, we should
	// use this type to convert the value, instead of requiring a transformer
	if transformer, found := c.envTransform[key]; found {
		actualValue = transformer(envval)
	}
	parts := splitKeyFunc(key)
	return curr.SetAt(parts, actualValue, model.SourceEnvVar)
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
	var curr Node = c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return false
		}
		curr = next
	}
	return true
}

func hasNoneDefaultsLeaf(node InnerNode) bool {
	// We're on an InnerNode, we need to check if any child leaf are not defaults
	for _, name := range node.ChildrenKeys() {
		child, _ := node.GetChild(name)
		if leaf, ok := child.(LeafNode); ok {
			if leaf.Source().IsGreaterThan(model.SourceDefault) {
				return true
			}
			continue
		}
		if hasNoneDefaultsLeaf(child.(InnerNode)) {
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
	var curr Node = c.root
	for _, part := range pathParts {
		next, err := curr.GetChild(part)
		if err != nil {
			return false
		}
		curr = next
	}
	// if key is a leaf, we just check the source
	if leaf, ok := curr.(LeafNode); ok {
		return leaf.Source().IsGreaterThan(model.SourceDefault)
	}

	// if the key was an InnerNode we need to check all the inner leaf node to check if one was set by the user
	return hasNoneDefaultsLeaf(curr.(InnerNode))
}

// AllKeysLowercased returns all keys lower-cased from the default tree, including keys that are merely marked as known
func (c *ntmConfig) AllKeysLowercased() []string {
	c.RLock()
	defer c.RUnlock()

	return slices.Clone(c.allSettings)
}

func (c *ntmConfig) leafAtPathFromNode(key string, curr Node) LeafNode {
	node := c.nodeAtPathFromNode(key, curr)
	if leaf, ok := node.(LeafNode); ok {
		return leaf
	}
	return missingLeaf
}

func (c *ntmConfig) nodeAtPathFromNode(key string, curr Node) Node {
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

// GetNode returns a Node for the given key
func (c *ntmConfig) GetNode(key string) (Node, error) {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
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

	c.addToSchema(key, model.SourceEnvVar)
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

// UnmarshalKey unmarshals the data for the given key
// DEPRECATED: use pkg/config/structure.UnmarshalKey instead
func (c *ntmConfig) UnmarshalKey(key string, _rawVal interface{}, _opts ...func(*mapstructure.DecoderConfig)) error {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return fmt.Errorf("nodetreemodel.UnmarshalKey not available, use pkg/config/structure.UnmarshalKey instead")
}

// MergeConfig merges in another config
func (c *ntmConfig) MergeConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()

	if !c.isReady() && !c.allowDynamicSchema.Load() {
		return fmt.Errorf("attempt to MergeConfig before config is constructed")
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

	return c.root.DumpSettings(func(model.Source) bool { return true })
}

// AllSettingsWithoutDefault returns a copy of the all the settings in the configuration without defaults
func (c *ntmConfig) AllSettingsWithoutDefault() map[string]interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()

	// We only want to include leaf with a source higher than SourceDefault
	return c.root.DumpSettings(func(source model.Source) bool { return source.IsGreaterThan(model.SourceDefault) })
}

// AllSettingsBySource returns the settings from each source (file, env vars, ...)
func (c *ntmConfig) AllSettingsBySource() map[model.Source]interface{} {
	c.maybeRebuild()

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
		model.SourceProvided:           c.root.DumpSettings(func(src model.Source) bool { return src != model.SourceDefault }),
	}
}

// AllSettingsWithSequenceID returns the settings and the sequence ID.
func (c *ntmConfig) AllSettingsWithSequenceID() (map[string]interface{}, uint64) {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	return c.root.DumpSettings(func(model.Source) bool { return true }), c.sequenceID
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
	if inner, ok := n.(*innerNode); ok {
		return inner.ChildrenKeys()
	}
	return nil
}

// BindEnvAndSetDefault binds an environment variable and sets a default for the given key
func (c *ntmConfig) BindEnvAndSetDefault(key string, val interface{}, envvars ...string) {
	c.BindEnv(key, envvars...) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv' //nolint:errcheck
	c.SetDefault(key, val)
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
		knownKeys:          map[string]struct{}{},
		allSettings:        []string{},
		unknownKeys:        map[string]struct{}{},
		schema:             newInnerNode(nil),
		defaults:           newInnerNode(nil),
		file:               newInnerNode(nil),
		unknown:            newInnerNode(nil),
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
