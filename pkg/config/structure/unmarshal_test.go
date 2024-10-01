// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package structure

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

// Struct that is used within the config
type UserV3 struct {
	Username       string `yaml:"user"`
	UsernameLegacy string `yaml:"username"`
	AuthKey        string `yaml:"authKey"`
	AuthProtocol   string `yaml:"authProtocol"`
	PrivKey        string `yaml:"privKey"`
	PrivProtocol   string `yaml:"privProtocol"`
}

// Type that gets parsed out of config
type TrapsConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Port             uint16   `yaml:"port"`
	Users            []UserV3 `yaml:"users"`
	CommunityStrings []string `yaml:"community_strings"`
	BindHost         string   `yaml:"bind_host"`
	StopTimeout      int      `yaml:"stop_timeout"`
	Namespace        string   `yaml:"namespace"`
}

func TestUnmarshalKeyTrapsConfig(t *testing.T) {
	confYaml := `
network_devices:
  snmp_traps:
    enabled: true
    port: 1234
    community_strings: ["a","b","c"]
    users:
    - user:         alice
      authKey:      hunter2
      authProtocol: MD5
      privKey:      pswd
      privProtocol: AE5
    - user:         bob
      authKey:      "123456"
      authProtocol: MD5
      privKey:      secret
      privProtocol: AE5
    bind_host: ok
    stop_timeout: 4
    namespace: abc
`
	mockConfig := mock.NewFromYAML(t, confYaml)

	var trapsCfg = TrapsConfig{}
	err := UnmarshalKey(mockConfig, "network_devices.snmp_traps", &trapsCfg)
	assert.NoError(t, err)

	assert.Equal(t, trapsCfg.Enabled, true)
	assert.Equal(t, trapsCfg.Port, uint16(1234))
	assert.Equal(t, trapsCfg.CommunityStrings, []string{"a", "b", "c"})

	assert.Equal(t, len(trapsCfg.Users), 2)
	assert.Equal(t, trapsCfg.Users[0].Username, "alice")
	assert.Equal(t, trapsCfg.Users[0].AuthKey, "hunter2")
	assert.Equal(t, trapsCfg.Users[0].AuthProtocol, "MD5")
	assert.Equal(t, trapsCfg.Users[0].PrivKey, "pswd")
	assert.Equal(t, trapsCfg.Users[0].PrivProtocol, "AE5")
	assert.Equal(t, trapsCfg.Users[1].Username, "bob")
	assert.Equal(t, trapsCfg.Users[1].AuthKey, "123456")
	assert.Equal(t, trapsCfg.Users[1].AuthProtocol, "MD5")
	assert.Equal(t, trapsCfg.Users[1].PrivKey, "secret")
	assert.Equal(t, trapsCfg.Users[1].PrivProtocol, "AE5")

	assert.Equal(t, trapsCfg.BindHost, "ok")
	assert.Equal(t, trapsCfg.StopTimeout, 4)
	assert.Equal(t, trapsCfg.Namespace, "abc")
}

type Endpoint struct {
	Name   string `yaml:"name"`
	APIKey string `yaml:"apikey"`
}

func TestUnmarshalKeySliceOfStructures(t *testing.T) {
	confYaml := `
endpoints:
- name: intake
  apikey: abc1
- name: config
  apikey: abc2
- name: health
  apikey: abc3
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("endpoints")

	var endpoints = []Endpoint{}
	err := UnmarshalKey(mockConfig, "endpoints", &endpoints)
	assert.NoError(t, err)

	assert.Equal(t, len(endpoints), 3)
	assert.Equal(t, endpoints[0].Name, "intake")
	assert.Equal(t, endpoints[0].APIKey, "abc1")
	assert.Equal(t, endpoints[1].Name, "config")
	assert.Equal(t, endpoints[1].APIKey, "abc2")
	assert.Equal(t, endpoints[2].Name, "health")
	assert.Equal(t, endpoints[2].APIKey, "abc3")
}

type FeatureConfig struct {
	Enabled bool `yaml:"enabled"`
}

func TestUnmarshalKeyParseStringAsBool(t *testing.T) {
	confYaml := `
feature:
  enabled: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	var feature = FeatureConfig{}
	err := UnmarshalKey(mockConfig, "feature", &feature)
	assert.NoError(t, err)

	assert.Equal(t, feature.Enabled, true)
}

type FeatureConfigDiffCase struct {
	ENaBLEd bool
}

func TestUnmarshalKeyCaseInsensitive(t *testing.T) {
	confYaml := `
feature:
  EnABLeD: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	var feature = FeatureConfig{}
	err := UnmarshalKey(mockConfig, "feature", &feature)
	assert.NoError(t, err)

	assert.Equal(t, feature.Enabled, true)

	var diffcase = FeatureConfigDiffCase{}
	err = UnmarshalKey(mockConfig, "feature", &diffcase)
	assert.NoError(t, err)

	assert.Equal(t, diffcase.ENaBLEd, true)
}

func TestUnmarshalKeyMissing(t *testing.T) {
	confYaml := `
feature:
  enabled: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	// If the data from the config is missing, UnmarshalKey is a no-op, does
	// nothing, and returns no error
	var endpoints = []Endpoint{}
	err := UnmarshalKey(mockConfig, "config_providers", &endpoints)
	assert.NoError(t, err)
}

// TODO:
// Tests should exist for the errors that UnmarshalKey can return. For example
// copyAny can return an error if passed a scalar target but the source is
// non-scalar. This can be triggered by calling UnmarshalKey with a struct that
// does not match the same of the data that is being assigned to it. It would be
// great to have all of the possible errors covered by tests.
func TestUnmarshalKeyErrorsExpectingScalar(t *testing.T) {
	t.Skip()
}

func TestUnmarshalKeyErrorsNonStringMap(t *testing.T) {
	t.Skip()
}

func TestUnmarshalKeyConversionErrors(t *testing.T) {
	t.Skip()
}

// TODO:
// We should test that struct tag fields work correctly when the tag has
// specifiers like “omitempty”. This should be tested at 2 levels: 1) directly by
// passing reflect.StructField values to fieldNameToKey and 2) by declaring
// testdata structs that use “omitempty” and using them with UnmarshalKey.
type omitConfig struct {
	Enabled string `yaml:"enabled,omitempty"`
}

// A flag is provided as a struct tag after a field name separated by a comma that
// alters the decoding behavior, eg. struct { Foo string `json:"field1,omitempty"` }
//
// List of common package flags we take into consideration:
// * yaml.v2 flags:  omitempty, flow, inline
// * json flags: omitempty
// * mapstructure flags: squash, remain, omitempty
type squashConfig struct {
	Host     string
	Endpoint Endpoint `mapstructure:",squash"`
}

func TestUnmarshalKeyWithSquash(t *testing.T) {
	confYaml := `
service:
  host: datad0g.com
  name: intake
  apikey: abc1
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("service")
	var svc = squashConfig{}

	t.Run("squash flag errors without option", func(t *testing.T) {
		err := UnmarshalKey(mockConfig, "service", &svc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EnableSquash")
	})

	t.Run("squash flag succeeds with option", func(t *testing.T) {
		err := UnmarshalKey(mockConfig, "service", &svc, EnableSquash)
		assert.NoError(t, err)

		assert.Equal(t, svc.Host, "datad0g.com")
		assert.Equal(t, svc.Endpoint.Name, "intake")
		assert.Equal(t, svc.Endpoint.APIKey, "abc1")
	})
}

func TestFieldNameToKey(t *testing.T) {
	t.Skip()
}

func TestUnmarshalMapstructureFieldNameToKey(t *testing.T) {
	t.Skip()
}

func TestUnmarshalSkipsOmitEmpty(t *testing.T) {
	confYaml := `
feature:
  enabled: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	// If the data from the config is missing, UnmarshalKey is a no-op, does
	// nothing, and returns no error
	var endpoints = []Endpoint{}
	err := UnmarshalKey(mockConfig, "config_providers", &endpoints)
	assert.NoError(t, err)
}

// TODO:
// Test that UnmarshalKey can be used to assign to a map[K]V. Right now only
// structs are assigned to, but map should also work.
func TestUnmarshalKeysToMap(t *testing.T) {
	t.Skip()
}

// TODO:
// The test TestMapGetChildNotFound validates the behavior of a innerMapNodeImpl
// directly, but the other node types don’t have similar tests. For example, we
// should have a test for innerNodeImpl that constructs a node using newNode(someStruct)
// and checks how its methods behave. Same for arrayNodeImpl and leafNodeImpl.
func TestMapGetChildNotFound(t *testing.T) {
	m := map[string]string{"a": "apple", "b": "banana"}
	n, err := newNode(reflect.ValueOf(m))
	assert.NoError(t, err)

	val, err := n.GetChild("a")
	assert.NoError(t, err)
	str, err := val.(leafNode).GetString()
	assert.NoError(t, err)
	assert.Equal(t, str, "apple")

	_, err = n.GetChild("c")
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "not found")

	keys, err := n.ChildrenKeys()
	assert.NoError(t, err)
	assert.Equal(t, keys, []string{"a", "b"})
}
