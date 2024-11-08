// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var confYaml = `
network_devices:
  snmp_traps:
    enabled: true
    port: 1234
    bind_host: ok
    stop_timeout: 4
    namespace: abc
`

var confYaml2 = `
network_devices:
  snmp_traps:
    port: 9876
    bind_host: ko
`

func setupDefault(t *testing.T, cfg model.Config) *ntmConfig {
	// TODO: we manually create an empty tree until we can create one from the defaults settings. Once remove
	// 'SetDefault' should replace those. This entire block should be remove then.
	obj := map[string]interface{}{
		"network_devices": map[string]interface{}{
			"snmp_traps": map[string]interface{}{
				"enabled":      0,
				"port":         0,
				"bind_host":    0,
				"stop_timeout": 0,
				"namespace":    0,
			},
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)
	ntm := cfg.(*ntmConfig)
	ntm.defaults = defaults
	return ntm
}

func writeTempFile(t *testing.T, name string, data string) string {
	dir := t.TempDir()
	confPath := filepath.Join(dir, name)
	err := os.WriteFile(confPath, []byte(data), 0600)
	require.NoError(t, err)
	return confPath
}

func TestReadConfig(t *testing.T) {
	cfg := NewConfig("datadog", "DD", nil)
	setupDefault(t, cfg)

	err := cfg.ReadConfig(strings.NewReader(confYaml))
	require.NoError(t, err)

	assert.Equal(t, true, cfg.GetBool("network_devices.snmp_traps.enabled"))
	assert.Equal(t, 1234, cfg.GetInt("network_devices.snmp_traps.port"))
	assert.Equal(t, "ok", cfg.GetString("network_devices.snmp_traps.bind_host"))
	assert.Equal(t, 4, cfg.GetInt("network_devices.snmp_traps.stop_timeout"))
	assert.Equal(t, "abc", cfg.GetString("network_devices.snmp_traps.namespace"))

	err = cfg.ReadConfig(strings.NewReader(confYaml2))
	require.NoError(t, err)

	assert.Equal(t, true, cfg.GetBool("network_devices.snmp_traps.enabled"))
	assert.Equal(t, 9876, cfg.GetInt("network_devices.snmp_traps.port"))
	assert.Equal(t, "ko", cfg.GetString("network_devices.snmp_traps.bind_host"))
	assert.Equal(t, 4, cfg.GetInt("network_devices.snmp_traps.stop_timeout"))
	assert.Equal(t, "abc", cfg.GetString("network_devices.snmp_traps.namespace"))
}

func TestReadSingleFile(t *testing.T) {
	confPath := writeTempFile(t, "datadog.yaml", confYaml)

	cfg := NewConfig("datadog", "DD", nil)
	cfg.SetConfigFile(confPath)
	setupDefault(t, cfg)

	err := cfg.ReadInConfig()
	require.NoError(t, err)

	assert.Equal(t, true, cfg.GetBool("network_devices.snmp_traps.enabled"))
	assert.Equal(t, 1234, cfg.GetInt("network_devices.snmp_traps.port"))
	assert.Equal(t, "ok", cfg.GetString("network_devices.snmp_traps.bind_host"))
	assert.Equal(t, 4, cfg.GetInt("network_devices.snmp_traps.stop_timeout"))
	assert.Equal(t, "abc", cfg.GetString("network_devices.snmp_traps.namespace"))
}

func TestReadFilePathError(t *testing.T) {
	cfg := NewConfig("datadog", "DD", nil)
	cfg.SetConfigFile("does_not_exist.yaml")

	err := cfg.ReadInConfig()
	require.Error(t, err)

	confPath := writeTempFile(t, "datadog.yaml", confYaml)
	cfg.SetConfigFile(confPath)
	cfg.AddExtraConfigPaths([]string{"does_not_exist.yaml"})

	err = cfg.ReadInConfig()
	require.Error(t, err)
}

func TestReadInvalidYAML(t *testing.T) {
	confPath := writeTempFile(t, "datadog.yaml", "some invalid YAML")

	cfg := NewConfig("datadog", "DD", nil)
	cfg.SetConfigFile(confPath)

	err := cfg.ReadInConfig()
	require.Error(t, err)

	cfg = NewConfig("datadog", "DD", nil)
	err = cfg.ReadConfig(strings.NewReader("some invalid YAML"))
	require.Error(t, err)
}

func TestReadExtraFile(t *testing.T) {
	confPath := writeTempFile(t, "datadog.yaml", confYaml)
	confPath2 := writeTempFile(t, "datadog_second.yaml", confYaml2)

	cfg := NewConfig("datadog", "DD", nil)
	cfg.SetConfigFile(confPath)
	cfg.AddExtraConfigPaths([]string{confPath2})
	setupDefault(t, cfg)

	err := cfg.ReadInConfig()
	require.NoError(t, err)

	assert.Equal(t, true, cfg.GetBool("network_devices.snmp_traps.enabled"))
	assert.Equal(t, 9876, cfg.GetInt("network_devices.snmp_traps.port"))
	assert.Equal(t, "ko", cfg.GetString("network_devices.snmp_traps.bind_host"))
	assert.Equal(t, 4, cfg.GetInt("network_devices.snmp_traps.stop_timeout"))
	assert.Equal(t, "abc", cfg.GetString("network_devices.snmp_traps.namespace"))
}

func TestYAMLLoad(t *testing.T) {
	var yamlPayload = `
a: orange
c:
  d: 1234
`
	yamlData := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(yamlPayload), &yamlData)
	require.NoError(t, err)

	// TODO: we manually create an empty tree until we can create one from the defaults settings
	obj := map[string]interface{}{
		"a": "apple",
		"b": 123,
		"c": map[string]interface{}{
			"d": true,
			"e": map[string]interface{}{
				"f": 456,
			},
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNode(nil)

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	assert.Empty(t, warnings)

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		children: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d"},
				children: map[string]Node{
					"d": &leafNodeImpl{val: 1234, source: model.SourceFile},
				},
			},
		},
	}
	assert.Equal(t, expected, tree)
}

func TestWarningUnknownKey(t *testing.T) {
	var yamlPayload = `
a: orange
c:
  d: 1234
  unknown: key
`
	yamlData := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(yamlPayload), &yamlData)
	require.NoError(t, err)

	// TODO: we manually create an empty tree until we can create one from the defaults settings
	obj := map[string]interface{}{
		"a": "apple",
		"c": map[string]interface{}{
			"d": true,
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNode(nil)

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "unknown key from YAML: c.unknown", warnings[0])

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		children: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d"},
				children: map[string]Node{
					"d": &leafNodeImpl{val: 1234, source: model.SourceFile},
				},
			},
		},
	}
	assert.Equal(t, expected, tree)
}

func TestWarningConflictDataType(t *testing.T) {
	var yamlPayload = `
a: orange
c: 1234
`
	yamlData := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(yamlPayload), &yamlData)
	require.NoError(t, err)

	// TODO: we manually create an empty tree until we can create one from the defaults settings
	obj := map[string]interface{}{
		"a": "apple",
		"c": map[string]interface{}{
			"d": true,
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNode(nil)

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "invalid type from configuration for key 'c'", warnings[0])

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		children: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{},
				children:  map[string]Node{},
			},
		},
	}
	assert.Equal(t, expected, tree)
}

func TestWarningConflictNodeTypeInnerToLeaf(t *testing.T) {
	var yamlPayload = `
a:
  b:
    c: 1234
`
	yamlData := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(yamlPayload), &yamlData)
	require.NoError(t, err)

	// TODO: we manually create an empty tree until we can create one from the defaults settings
	obj := map[string]interface{}{
		"a": map[string]interface{}{
			"b": true,
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNode(nil)
	tree.SetAt([]string{"a", "b", "c"}, 9876, model.SourceFile)

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "invalid tree: default and dest tree don't have the same layout", warnings[0])
}

func TestWarningConflictNodeTypeLeafToInner(t *testing.T) {
	var yamlPayload = `
a:
  b:
    c: 1234
`
	yamlData := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(yamlPayload), &yamlData)
	require.NoError(t, err)

	// TODO: we manually create an empty tree until we can create one from the defaults settings
	obj := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1324,
			},
		},
	}
	newNode, err := NewNodeTree(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNode(nil)
	tree.SetAt([]string{"a", "b"}, 9876, model.SourceFile)

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "invalid tree: default and dest tree don't have the same layout", warnings[0])
}

func TestToMapStringInterface(t *testing.T) {
	_, err := toMapStringInterface(nil, "key")
	assert.Error(t, err)
	_, err = toMapStringInterface(1, "key")
	assert.Error(t, err)
	_, err = toMapStringInterface("test", "key")
	assert.Error(t, err)
	_, err = toMapStringInterface(map[int]string{1: "test"}, "key")
	assert.Error(t, err)
	_, err = toMapStringInterface(map[interface{}]string{1: "test"}, "key")
	assert.Error(t, err)
	_, err = toMapStringInterface(map[interface{}]string{1: "test", "test2": "test2"}, "key")
	assert.Error(t, err)

	data, err := toMapStringInterface(map[string]string{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = toMapStringInterface(map[string]interface{}{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = toMapStringInterface(map[interface{}]interface{}{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = toMapStringInterface(map[interface{}]string{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)
}
