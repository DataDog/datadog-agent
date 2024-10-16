// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

//var confYaml = `
//network_devices:
//  snmp_traps:
//    enabled: true
//    port: 1234
//    community_strings: ["a","b","c"]
//    users:
//    - user:         alice
//      authKey:      hunter2
//      authProtocol: MD5
//      privKey:      pswd
//      privProtocol: AE5
//    - user:         bob
//      authKey:      "123456"
//      authProtocol: MD5
//      privKey:      secret
//      privProtocol: AE5
//    bind_host: ok
//    stop_timeout: 4
//    namespace: abc
//`

//func TestReadConfigAndGetValues(t *testing.T) {
//	cfg := NewConfig("datadog", "DD", nil)
//	err := cfg.ReadConfig(strings.NewReader(confYaml))
//	if err != nil {
//		panic(err)
//	}
//
//	enabled := cfg.GetBool("network_devices.snmp_traps.enabled")
//	port := cfg.GetInt("network_devices.snmp_traps.port")
//	bindHost := cfg.GetString("network_devices.snmp_traps.bind_host")
//	stopTimeout := cfg.GetInt("network_devices.snmp_traps.stop_timeout")
//	namespace := cfg.GetString("network_devices.snmp_traps.namespace")
//
//	assert.Equal(t, enabled, true)
//	assert.Equal(t, port, 1234)
//	assert.Equal(t, bindHost, "ok")
//	assert.Equal(t, stopTimeout, 4)
//	assert.Equal(t, namespace, "abc")
//}

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
	newNode, err := NewNode(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNodeImpl()

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	assert.Empty(t, warnings)

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		val: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d"},
				val: map[string]Node{
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
	newNode, err := NewNode(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNodeImpl()

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "unknown key from YAML: c.unknown", warnings[0])

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		val: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d"},
				val: map[string]Node{
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
	newNode, err := NewNode(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNodeImpl()

	warnings := loadYamlInto(defaults, tree, yamlData, "")

	require.Len(t, warnings, 1)
	assert.Equal(t, "invalid type from configuration for key 'c'", warnings[0])

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "c": "c"},
		val: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{},
				val:       map[string]Node{},
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
	newNode, err := NewNode(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNodeImpl()
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
	newNode, err := NewNode(obj, model.SourceDefault)
	require.NoError(t, err)
	defaults, ok := newNode.(InnerNode)
	require.True(t, ok)

	tree := newInnerNodeImpl()
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
