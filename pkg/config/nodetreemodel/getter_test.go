// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
)

func TestGetKnownKeysLowercased(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b.C", "test")
	cfg.SetKnown("d.E.f") //nolint:forbidigo // testing behavior
	cfg.BuildSchema()

	assert.Equal(t,
		map[string]interface{}{
			"a":     struct{}{},
			"b":     struct{}{},
			"b.c":   struct{}{},
			"d":     struct{}{},
			"d.e":   struct{}{},
			"d.e.f": struct{}{},
		},
		cfg.GetKnownKeysLowercased())
}

func TestGet(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.BuildSchema()

	assert.Equal(t, 1234, cfg.Get("a"))

	cfg.Set("a", 9876, model.SourceAgentRuntime)
	assert.Equal(t, 9876, cfg.Get("a"))

	assert.Equal(t, nil, cfg.Get("does_not_exists"))

	// test implicit conversion
	cfg.Set("a", "1111", model.SourceAgentRuntime)
	assert.Equal(t, 1111, cfg.Get("a"))
}

func TestGetDefaultType(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetKnown("a") //nolint:forbidigo // testing behavior
	cfg.SetKnown("b") //nolint:forbidigo // testing behavior
	cfg.BuildSchema()

	cfg.ReadConfig(strings.NewReader(`---
a:
  "url1":
   - apikey2
   - apikey3
  "url2":
   - apikey4
b:
  1:
   - a
   - b
  2:
   - c
`))

	expected := map[string]interface{}{
		"url1": []interface{}{"apikey2", "apikey3"},
		"url2": []interface{}{"apikey4"},
	}
	assert.Equal(t, expected, cfg.Get("a"))

	expected2 := map[interface{}]interface{}{
		1: []interface{}{"a", "b"},
		2: []interface{}{"c"},
	}
	assert.Equal(t, expected2, cfg.Get("b"))
}

func TestGetInnerNode(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a.b.c", 1234)
	cfg.SetDefault("a.e", 1234)
	cfg.BuildSchema()

	assert.Equal(t, 1234, cfg.Get("a.b.c"))
	assert.Equal(t, 1234, cfg.Get("a.e"))
	assert.Equal(t, map[string]interface{}{"c": 1234}, cfg.Get("a.b"))
	assert.Equal(t, map[string]interface{}{"b": map[string]interface{}{"c": 1234}, "e": 1234}, cfg.Get("a"))

	cfg.Set("a.b.c", 9876, model.SourceAgentRuntime)
	assert.Equal(t, 9876, cfg.Get("a.b.c"))
	assert.Equal(t, 1234, cfg.Get("a.e"))
	assert.Equal(t, map[string]interface{}{"c": 9876}, cfg.Get("a.b"))
	assert.Equal(t, map[string]interface{}{"b": map[string]interface{}{"c": 9876}, "e": 1234}, cfg.Get("a"))
}

func TestGetCastToDefault(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", []string{})
	cfg.BuildSchema()

	// This test that we mimic viper's behavior on Get where we convert the value from the config to the same type
	// from the default.

	cfg.Set("a", 9876, model.SourceAgentRuntime)
	assert.Equal(t, []string{"9876"}, cfg.Get("a"))

	cfg.Set("a", "a b c", model.SourceAgentRuntime)
	assert.Equal(t, []string{"a", "b", "c"}, cfg.Get("a"))

	assert.Equal(t, nil, cfg.Get("does_not_exists"))
}

func TestGetString(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "test")
	cfg.BuildSchema()

	assert.Equal(t, "1234", cfg.GetString("a"))
	assert.Equal(t, "test", cfg.GetString("b"))
	assert.Equal(t, "", cfg.GetString("does_not_exists"))
}

func TestGetBool(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", false)
	cfg.SetDefault("b", "true")
	cfg.SetDefault("c", 1)
	cfg.SetDefault("d", 0)
	cfg.BuildSchema()

	assert.Equal(t, false, cfg.GetBool("a"))
	assert.Equal(t, true, cfg.GetBool("b"))
	assert.Equal(t, true, cfg.GetBool("c"))
	assert.Equal(t, false, cfg.GetBool("e"))
	assert.Equal(t, false, cfg.GetBool("does_not_exists"))
}

func TestGetInt(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, 1234, cfg.GetInt("a"))
	assert.Equal(t, 987, cfg.GetInt("b"))
	assert.Equal(t, 0, cfg.GetInt("does_not_exists"))
}

func TestGetInt32(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, int32(1234), cfg.GetInt32("a"))
	assert.Equal(t, int32(987), cfg.GetInt32("b"))
	assert.Equal(t, int32(0), cfg.GetInt32("does_not_exists"))
}

func TestGetInt64(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, int64(1234), cfg.GetInt64("a"))
	assert.Equal(t, int64(987), cfg.GetInt64("b"))
	assert.Equal(t, int64(0), cfg.GetInt64("does_not_exists"))
}

func TestGetFloat64(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234.25)
	cfg.SetDefault("b", "987.25")
	cfg.BuildSchema()

	assert.Equal(t, float64(1234.25), cfg.GetFloat64("a"))
	assert.Equal(t, float64(987.25), cfg.GetFloat64("b"))
	assert.Equal(t, float64(0.0), cfg.GetFloat64("does_not_exists"))
}

func TestGetDuration(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, time.Duration(1234), cfg.GetDuration("a"))
	assert.Equal(t, time.Duration(987), cfg.GetDuration("b"))
	assert.Equal(t, time.Duration(0), cfg.GetDuration("does_not_exists"))
}

func TestGetStringSlice(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", []string{"a", "b", "c"})
	cfg.SetDefault("b", "a b c")
	cfg.BuildSchema()

	assert.Equal(t, []string{"a", "b", "c"}, cfg.GetStringSlice("a"))
	assert.Equal(t, []string{"a", "b", "c"}, cfg.GetStringSlice("b"))
	assert.Equal(t, []string(nil), cfg.GetStringSlice("does_not_exists"))
}

func TestGetStringMap(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", map[string]interface{}{"a": 1, "b": "b", "c": nil})
	cfg.SetDefault("b", "{\"a\": 1234}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string]interface{}{"a": 1, "b": "b", "c": nil}, cfg.GetStringMap("a"))
	assert.Equal(t, map[string]interface{}{"a": 1234.0}, cfg.GetStringMap("b"))
	assert.Equal(t, map[string]interface{}{}, cfg.GetStringMap("does_not_exists"))
}

func TestGetStringMapString(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", map[string]interface{}{"a": 123, "b": "b", "c": ""})
	cfg.SetDefault("b", "{\"a\": \"test\"}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string]string{"a": "123", "b": "b", "c": ""}, cfg.GetStringMapString("a"))
	assert.Equal(t, map[string]string{"a": "test"}, cfg.GetStringMapString("b"))
	assert.Equal(t, map[string]string{}, cfg.GetStringMapString("does_not_exists"))
}

func TestGetStringMapStringSlice(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", map[string][]interface{}{"a": {1, 2}, "b": {"b", "bb"}, "c": nil})
	cfg.SetDefault("b", "{\"a\": [\"test\", \"test2\"]}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string][]string{"a": {"1", "2"}, "b": {"b", "bb"}, "c": nil}, cfg.GetStringMapStringSlice("a"))
	assert.Equal(t, map[string][]string{"a": {"test", "test2"}}, cfg.GetStringMapStringSlice("b"))
	assert.Equal(t, map[string][]string{}, cfg.GetStringMapStringSlice("does_not_exists"))
}

func TestGetSizeInBytes(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("a", "123")
	cfg.SetDefault("b", "1kb")
	cfg.SetDefault("c", "1Mb")
	cfg.SetDefault("d", "1 GB")
	cfg.SetDefault("e", "-1")
	cfg.SetDefault("f", "invalid")
	cfg.BuildSchema()

	assert.Equal(t, uint(123), cfg.GetSizeInBytes("a"))
	assert.Equal(t, uint(1024), cfg.GetSizeInBytes("b"))
	assert.Equal(t, uint(1024*1024), cfg.GetSizeInBytes("c"))
	assert.Equal(t, uint(1024*1024*1024), cfg.GetSizeInBytes("d"))
	assert.Equal(t, uint(0), cfg.GetSizeInBytes("e"))
	assert.Equal(t, uint(0), cfg.GetSizeInBytes("f"))
}

func TestGetFloat64Slice(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("float_list", []float64{})
	cfg.SetDefault("string_list", []string{})
	cfg.BuildSchema()
	cfg.ReadConfig(strings.NewReader(`---
float_list:
  - 1.1
  - "2.2"
  - 3
string_list:
  - 1.1
  - "2.2"
  - 3
`))

	assert.Equal(t, []float64{1.1, 2.2, 3.0}, cfg.GetFloat64Slice("float_list"))
	assert.Equal(t, []float64{1.1, 2.2, 3.0}, cfg.GetFloat64Slice("string_list"))
}

func TestGetFloat64SliceError(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "", nil)
	cfg.SetDefault("float_list", []float64{})
	cfg.BuildSchema()
	cfg.ReadConfig(strings.NewReader(`---
float_list:
  - a
  - 2.2
  - 3.3
`))

	assert.Nil(t, cfg.GetFloat64Slice("float_list"))
}

func TestGetFloat64SliceStringFromEnv(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST", nil)
	cfg.BindEnvAndSetDefault("float_list", []string{})
	t.Setenv("TEST_FLOAT_LIST", "1.1 2.2 3.3")
	cfg.BuildSchema()

	assert.Equal(t, []float64{1.1, 2.2, 3.3}, cfg.GetFloat64Slice("float_list"))
}

func TestGetAllSources(t *testing.T) {
	t.Setenv("TEST_A", "4")

	cfg := NewNodeTreeConfig("test", "TEST", nil)
	cfg.BindEnvAndSetDefault("a", 0)
	cfg.BuildSchema()

	cfg.Set("a", 1, model.SourceUnknown)
	cfg.Set("a", 2, model.SourceInfraMode)
	cfg.Set("a", 3, model.SourceFile)
	cfg.Set("a", 5, model.SourceFleetPolicies)
	cfg.Set("a", 6, model.SourceAgentRuntime)
	cfg.Set("a", 7, model.SourceLocalConfigProcess)
	cfg.Set("a", 8, model.SourceRC)
	cfg.Set("a", 9, model.SourceCLI)

	res := cfg.GetAllSources("a")
	assert.Equal(t,
		[]model.ValueWithSource{
			{Source: model.SourceDefault, Value: 0},
			{Source: model.SourceUnknown, Value: 1},
			{Source: model.SourceInfraMode, Value: 2},
			{Source: model.SourceFile, Value: 3},
			{Source: model.SourceEnvVar, Value: "4"},
			{Source: model.SourceFleetPolicies, Value: 5},
			{Source: model.SourceAgentRuntime, Value: 6},
			{Source: model.SourceLocalConfigProcess, Value: 7},
			{Source: model.SourceRC, Value: 8},
			{Source: model.SourceCLI, Value: 9},
		},
		res,
	)
}

func TestGetEnvVars(t *testing.T) {
	cfg := NewNodeTreeConfig("test", "TEST", nil)

	cfg.BindEnvAndSetDefault("d", 0, "D")
	cfg.BindEnvAndSetDefault("a", 0, "ABC")
	cfg.BindEnvAndSetDefault("b", 0, "ABC", "DEF")
	cfg.BindEnvAndSetDefault("c", 0, "DEF")
	cfg.BindEnvAndSetDefault("x", 0)

	// testing that duplicate are removed and result is sorted
	assert.Equal(t, []string{"ABC", "D", "DEF", "TEST_X"}, cfg.GetEnvVars())
}
