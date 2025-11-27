// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"maps"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/mohae/deepcopy"
	"github.com/spf13/cast"
)

func (c *ntmConfig) leafAtPath(key string) LeafNode {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return missingLeaf
	}

	return c.leafAtPathFromNode(key, c.root)
}

// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded or 3) have been SetKnown()
// Note that it returns the keys lowercased.
//
// TODO: remove once viper is no longer used. This is only used to detect unknown configuration from YAML which we do
// natively now (see 'warnings').
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

// GetEnvVars gets all environment variables
func (c *ntmConfig) GetEnvVars() []string {
	c.RLock()
	defer c.RUnlock()
	vars := make([]string, 0, len(c.configEnvVars))
	for _, v := range c.configEnvVars {
		vars = append(vars, v...)
	}
	return vars
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

	if c.GetString("proxy.http") == "" && c.GetString("proxy.https") == "" && len(c.GetStringSlice("proxy.no_proxy")) == 0 {
		return nil
	}
	p := &model.Proxy{
		HTTP:    c.GetString("proxy.http"),
		HTTPS:   c.GetString("proxy.https"),
		NoProxy: c.GetStringSlice("proxy.no_proxy"),
	}
	c.Lock()
	c.proxies = p
	c.Unlock()
	return c.proxies
}

func (c *ntmConfig) inferTypeFromDefault(key string, value interface{}) (interface{}, error) {
	// Viper infer the type from the default value for Get. This reproduce the same behavior.
	// Once all settings have a default value we could move this logic where we load data into the config rather
	// than out.
	defaultNode := c.leafAtPathFromNode(key, c.defaults)
	if defaultNode != missingLeaf {
		switch defaultNode.Get().(type) {
		case bool:
			return cast.ToBoolE(value)
		case string:
			return cast.ToStringE(value)
		case int32, int16, int8, int:
			return cast.ToIntE(value)
		case int64:
			return cast.ToInt64E(value)
		case float64, float32:
			return cast.ToFloat64E(value)
		case time.Time:
			return cast.ToTimeE(value)
		case time.Duration:
			return cast.ToDurationE(value)
		case []string:
			return cast.ToStringSliceE(value)
		}
	}

	// if we don't have a default and the value is a map[interface{}]interface{} we try to cast is as a
	// map[string]interface{}. This mimic the behavior from viper that default to that type.
	//
	// TODO: once all settings in the config have a default value we can remove this logic
	if m, ok := value.(map[interface{}]interface{}); ok {
		res := map[string]interface{}{}

		for k, v := range m {
			if keyString, ok := k.(string); ok {
				res[keyString] = deepcopy.Copy(v)
			} else {
				goto simplyCopy
			}
		}
		return res, nil
	}

	// NOTE: should only need to deepcopy for `Get`, because it can be an arbitrary value,
	// and we shouldn't ever return complex types like maps and slices that could be modified
	// by callers accidentally or on purpose. By copying, the caller may modify the result safetly
simplyCopy:
	return deepcopy.Copy(value), nil
}

func (c *ntmConfig) getNodeValue(key string) interface{} {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
		log.Errorf("attempt to read key before config is constructed: %s", key)
		return ""
	}

	node := c.nodeAtPathFromNode(key, c.root)

	if leaf, ok := node.(LeafNode); ok {
		return leaf.Get()
	}

	// When querying an InnerNode we convert it as a map[string]interface{} to mimic Viper's logic
	var converter func(node InnerNode) map[string]interface{}
	converter = func(node InnerNode) map[string]interface{} {
		res := map[string]interface{}{}
		for _, name := range node.ChildrenKeys() {
			child, _ := node.GetChild(name)

			if leaf, ok := child.(LeafNode); ok {
				res[name] = leaf.Get()
			} else {
				res[name] = converter(child.(InnerNode))
			}
		}
		return res
	}

	return converter(node.(InnerNode))
}

// Get returns a copy of the value for the given key
func (c *ntmConfig) Get(key string) interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)

	val, err := c.inferTypeFromDefault(key, c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetAllSources returns all values for a key for each source in sorted from lower to higher priority
func (c *ntmConfig) GetAllSources(key string) []model.ValueWithSource {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	vals := make([]model.ValueWithSource, len(sources))
	for idx, source := range sources {
		tree, err := c.getTreeBySource(source)
		if err != nil {
			log.Errorf("unknown source '%s'", source)
			continue
		}
		vals[idx].Source = source
		vals[idx].Value = c.leafAtPathFromNode(key, tree).Get()
	}
	return vals
}

// GetString returns a string-typed value for the given key
func (c *ntmConfig) GetString(key string) string {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	str, err := cast.ToStringE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return str
}

// GetBool returns a bool-typed value for the given key
func (c *ntmConfig) GetBool(key string) bool {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	b, err := cast.ToBoolE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return b
}

// GetInt returns an int-typed value for the given key
func (c *ntmConfig) GetInt(key string) int {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToIntE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetInt32 returns an int32-typed value for the given key
func (c *ntmConfig) GetInt32(key string) int32 {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToInt32E(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return int32(val)
}

// GetInt64 returns an int64-typed value for the given key
func (c *ntmConfig) GetInt64(key string) int64 {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToInt64E(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return int64(val)
}

// GetFloat64 returns a float64-typed value for the given key
func (c *ntmConfig) GetFloat64(key string) float64 {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToFloat64E(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetFloat64 returns a float64-typed value for the given key
func (c *ntmConfig) GetFloat64Slice(key string) []float64 {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)

	list, err := cast.ToStringSliceE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}

	res := make([]float64, 0, len(list))
	for _, item := range list {
		nb, err := cast.ToFloat64E(item)
		if err != nil {
			log.Errorf("value '%v' from '%v' is not a float64", item, key)
			return nil
		}
		res = append(res, nb)
	}
	return res
}

// GetDuration returns a duration-typed value for the given key
func (c *ntmConfig) GetDuration(key string) time.Duration {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToDurationE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return val
}

// GetStringSlice returns a string slice value for the given key
func (c *ntmConfig) GetStringSlice(key string) []string {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToStringSliceE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return slices.Clone(val)
}

// GetStringMap returns a map[string]interface value for the given key
func (c *ntmConfig) GetStringMap(key string) map[string]interface{} {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToStringMapE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return maps.Clone(val)
}

// GetStringMapString returns a map[string]string value for the given key
func (c *ntmConfig) GetStringMapString(key string) map[string]string {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToStringMapStringE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	return maps.Clone(val)
}

// GetStringMapStringSlice returns a map[string][]string value for the given key
func (c *ntmConfig) GetStringMapStringSlice(key string) map[string][]string {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	val, err := cast.ToStringMapStringSliceE(c.getNodeValue(key))
	if err != nil {
		log.Warnf("failed to get configuration value for key %q: %s", key, err)
	}
	// We don't use maps.Clone since we also want to clone the slices
	res := map[string][]string{}
	for k, v := range val {
		res[k] = slices.Clone(v)
	}
	return res
}

// GetSizeInBytes returns the size in bytes of the filename for the given key
func (c *ntmConfig) GetSizeInBytes(key string) uint {
	return parseSizeInBytes(c.GetString(key))
}

// GetSource returns the source of the given key
func (c *ntmConfig) GetSource(key string) model.Source {
	c.maybeRebuild()

	c.RLock()
	defer c.RUnlock()
	c.checkKnownKey(key)
	return c.leafAtPath(key).Source()
}
