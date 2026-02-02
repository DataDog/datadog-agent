// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func setupDefault(_ *testing.T, cfg model.BuildableConfig) *ntmConfig {
	cfg.SetDefault("network_devices.snmp_traps.enabled", false)
	cfg.SetDefault("network_devices.snmp_traps.port", 0)
	cfg.SetDefault("network_devices.snmp_traps.bind_host", "")
	cfg.SetDefault("network_devices.snmp_traps.stop_timeout", 0)
	cfg.SetDefault("network_devices.snmp_traps.namespace", "")

	cfg.BuildSchema()

	return cfg.(*ntmConfig)
}

func writeTempFile(t *testing.T, name string, data string) string {
	dir := t.TempDir()
	confPath := filepath.Join(dir, name)
	err := os.WriteFile(confPath, []byte(data), 0600)
	require.NoError(t, err)
	return confPath
}

func TestReadConfig(t *testing.T) {
	cfg := NewNodeTreeConfig("datadog", "DD", nil)
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

	// by reading confYaml2, we override the values set by confYaml, causing snmp_traps.enabled,
	// snmp_traps.stop_timeout, and snmp_traps.namespace to be set to their default values.
	assert.Equal(t, false, cfg.GetBool("network_devices.snmp_traps.enabled"))
	assert.Equal(t, 9876, cfg.GetInt("network_devices.snmp_traps.port"))
	assert.Equal(t, "ko", cfg.GetString("network_devices.snmp_traps.bind_host"))
	assert.Equal(t, 0, cfg.GetInt("network_devices.snmp_traps.stop_timeout"))
	assert.Equal(t, "", cfg.GetString("network_devices.snmp_traps.namespace"))
}

func TestReadSingleFile(t *testing.T) {
	confPath := writeTempFile(t, "datadog.yaml", confYaml)

	cfg := NewNodeTreeConfig("datadog", "DD", nil)
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
	cfg := NewNodeTreeConfig("datadog", "DD", nil)
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

	cfg := NewNodeTreeConfig("datadog", "DD", nil)
	cfg.SetConfigFile(confPath)

	err := cfg.ReadInConfig()
	require.Error(t, err)

	cfg = NewNodeTreeConfig("datadog", "DD", nil)
	err = cfg.ReadConfig(strings.NewReader("some invalid YAML"))
	require.Error(t, err)
}

func TestReadExtraFile(t *testing.T) {
	confPath := writeTempFile(t, "datadog.yaml", confYaml)
	confPath2 := writeTempFile(t, "datadog_second.yaml", confYaml2)

	cfg := NewNodeTreeConfig("datadog", "DD", nil)
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
	cfg := NewNodeTreeConfig("test", "TEST", nil)

	cfg.SetDefault("a", "apple")
	cfg.SetDefault("b", 123)
	cfg.SetDefault("c.d", 1)
	cfg.SetDefault("c.e.f", 456)

	cfg.BuildSchema()

	err := cfg.ReadConfig(strings.NewReader(yamlPayload))
	require.NoError(t, err)

	c := cfg.(*ntmConfig)
	assert.Empty(t, c.warnings)

	assert.Equal(t, "orange", cfg.Get("a"))
	assert.Equal(t, model.SourceFile, cfg.GetSource("a"))
	assert.Equal(t, 123, cfg.Get("b"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("b"))
	assert.Equal(t, 1234, cfg.Get("c.d"))
	assert.Equal(t, model.SourceFile, cfg.GetSource("c.d"))
	assert.Equal(t, 456, cfg.Get("c.e.f"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("c.e.f"))

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "orange", source: model.SourceFile},
			"c": {
				children: map[string]*nodeImpl{
					"d": {val: 1234, source: model.SourceFile},
				},
			},
		},
	}
	assert.Equal(t, expected, c.file)
}

func TestWarningUnknownKey(t *testing.T) {
	var yamlPayload = `
a: orange
c:
  d: 1234
  unknown: key
`
	cfg := NewNodeTreeConfig("test", "TEST", nil)

	cfg.SetDefault("a", "apple")
	cfg.SetDefault("c.d", true)

	cfg.BuildSchema()

	err := cfg.ReadConfig(strings.NewReader(yamlPayload))
	require.NoError(t, err)

	c := cfg.(*ntmConfig)

	require.Len(t, c.warnings, 1)
	assert.Equal(t, errors.New("unknown key from YAML: c.unknown"), c.warnings[0])

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "orange", source: model.SourceFile},
			"c": {
				children: map[string]*nodeImpl{
					"d":       {val: 1234, source: model.SourceFile},
					"unknown": {val: "key", source: model.SourceFile},
				},
			},
		},
	}
	assert.Equal(t, expected, c.file)
}

func TestWarningConflictDataType(t *testing.T) {
	var yamlPayload = `
a: orange
c: 1234
`
	cfg := NewNodeTreeConfig("test", "TEST", nil)

	cfg.SetDefault("a", "apple")
	cfg.SetDefault("c.d", true)

	cfg.BuildSchema()

	err := cfg.ReadConfig(strings.NewReader(yamlPayload))
	require.NoError(t, err)

	c := cfg.(*ntmConfig)

	require.Len(t, c.warnings, 1)
	assert.Equal(t, errors.New("expected map at 'c' got: 1234"), c.warnings[0])

	// The file node with "1234" still exists, but it was not merged because it didn't match
	// the schema layer.
	expected := `tree(#ptr<000000>) source=root
> a
    leaf(#ptr<000001>), val:"orange", source:file
> c
    leaf(#ptr<000002>), val:1234, source:file
tree(#ptr<000003>) source=default
> a
    leaf(#ptr<000004>), val:"apple", source:default
> c
  inner(#ptr<000005>)
  > d
      leaf(#ptr<000006>), val:true, source:default
tree(#ptr<000007>) source=file
> a
    leaf(#ptr<000001>), val:"orange", source:file
> c
    leaf(#ptr<000002>), val:1234, source:file`
	assert.Equal(t, expected, c.Stringify("all", model.OmitPointerAddr))
}

func TestToMapStringInterface(t *testing.T) {
	_, err := ToMapStringInterface(nil, "key")
	assert.Error(t, err)
	_, err = ToMapStringInterface(1, "key")
	assert.Error(t, err)
	_, err = ToMapStringInterface("test", "key")
	assert.Error(t, err)

	data, err := ToMapStringInterface(map[int]string{1: "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"1": "test"}, data)
	data, err = ToMapStringInterface(map[interface{}]string{1: "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"1": "test"}, data)
	data, err = ToMapStringInterface(map[interface{}]string{1: "test", "test2": "test2"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"1": "test", "test2": "test2"}, data)

	data, err = ToMapStringInterface(map[string]string{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = ToMapStringInterface(map[string]interface{}{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = ToMapStringInterface(map[interface{}]interface{}{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)

	data, err = ToMapStringInterface(map[interface{}]string{"test": "test"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"test": "test"}, data)
}

func TestReadConfigBeforeReady(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST", nil)
	err := cfg.ReadConfig(strings.NewReader(""))
	require.Error(t, err)
	assert.Equal(t, "attempt to ReadConfig before config is constructed", err.Error())

	cfg = NewNodeTreeConfig("test", "TEST", nil)
	err = cfg.ReadInConfig()
	require.Error(t, err)
	assert.Equal(t, "attempt to ReadInConfig before config is constructed", err.Error())
}

func TestReadConfigInvalidPath(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST", nil)
	cfg.SetConfigFile("does not exists")
	cfg.BuildSchema()

	err := cfg.ReadInConfig()
	require.Error(t, err)
}

func TestReadConfigInvalidYaml(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST", nil)
	cfg.BuildSchema()

	err := cfg.ReadConfig(strings.NewReader("123"))
	require.Error(t, err)
}

func TestBuildNestedMap(t *testing.T) {
	m := buildNestedMap([]string{"a", "b", "c"}, 123)
	expect := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 123,
			},
		},
	}
	require.Equal(t, expect, m)
}

func TestNilValueFromFileAreIgnored(t *testing.T) {
	var yamlPayload = `
a:
`
	cfg := NewNodeTreeConfig("test", "TEST", nil)

	cfg.SetDefault("a", true)

	cfg.BuildSchema()

	err := cfg.ReadConfig(strings.NewReader(yamlPayload))
	require.NoError(t, err)

	assert.Equal(t, true, cfg.GetBool("a"))
}
