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
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b.C", "test")
	cfg.SetKnown("d.E.f")
	cfg.BuildSchema()

	assert.Equal(t,
		map[string]interface{}{
			"a":     struct{}{},
			"b.c":   struct{}{},
			"d.e.f": struct{}{},
		},
		cfg.GetKnownKeysLowercased())
}

func TestGet(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.BuildSchema()

	assert.Equal(t, 1234, cfg.Get("a"))

	cfg.Set("a", "test", model.SourceAgentRuntime)
	assert.Equal(t, "test", cfg.Get("a"))

	assert.Equal(t, nil, cfg.Get("does_not_exists"))
}

func TestGetString(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "test")
	cfg.BuildSchema()

	assert.Equal(t, "1234", cfg.GetString("a"))
	assert.Equal(t, "test", cfg.GetString("b"))
	assert.Equal(t, "", cfg.GetString("does_not_exists"))
}

func TestGetBool(t *testing.T) {
	cfg := NewConfig("test", "", nil)
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
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, 1234, cfg.GetInt("a"))
	assert.Equal(t, 987, cfg.GetInt("b"))
	assert.Equal(t, 0, cfg.GetInt("does_not_exists"))
}

func TestGetInt32(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, int32(1234), cfg.GetInt32("a"))
	assert.Equal(t, int32(987), cfg.GetInt32("b"))
	assert.Equal(t, int32(0), cfg.GetInt32("does_not_exists"))
}

func TestGetInt64(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, int64(1234), cfg.GetInt64("a"))
	assert.Equal(t, int64(987), cfg.GetInt64("b"))
	assert.Equal(t, int64(0), cfg.GetInt64("does_not_exists"))
}

func TestGetFloat64(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234.25)
	cfg.SetDefault("b", "987.25")
	cfg.BuildSchema()

	assert.Equal(t, float64(1234.25), cfg.GetFloat64("a"))
	assert.Equal(t, float64(987.25), cfg.GetFloat64("b"))
	assert.Equal(t, float64(0.0), cfg.GetFloat64("does_not_exists"))
}

func TestGetDuration(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 1234)
	cfg.SetDefault("b", "987")
	cfg.BuildSchema()

	assert.Equal(t, time.Duration(1234), cfg.GetDuration("a"))
	assert.Equal(t, time.Duration(987), cfg.GetDuration("b"))
	assert.Equal(t, time.Duration(0), cfg.GetDuration("does_not_exists"))
}

func TestGetStringSlice(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", []string{"a", "b", "c"})
	cfg.SetDefault("b", "a b c")
	cfg.BuildSchema()

	assert.Equal(t, []string{"a", "b", "c"}, cfg.GetStringSlice("a"))
	assert.Equal(t, []string{"a", "b", "c"}, cfg.GetStringSlice("b"))
	assert.Equal(t, []string(nil), cfg.GetStringSlice("does_not_exists"))
}

func TestGetStringMap(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", map[string]interface{}{"a": 1, "b": "b", "c": nil})
	cfg.SetDefault("b", "{\"a\": 1234}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string]interface{}{"a": 1, "b": "b", "c": nil}, cfg.GetStringMap("a"))
	assert.Equal(t, map[string]interface{}{"a": 1234.0}, cfg.GetStringMap("b"))
	assert.Equal(t, map[string]interface{}{}, cfg.GetStringMap("does_not_exists"))
}

func TestGetStringMapString(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", map[string]interface{}{"a": 123, "b": "b", "c": ""})
	cfg.SetDefault("b", "{\"a\": \"test\"}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string]string{"a": "123", "b": "b", "c": ""}, cfg.GetStringMapString("a"))
	assert.Equal(t, map[string]string{"a": "test"}, cfg.GetStringMapString("b"))
	assert.Equal(t, map[string]string{}, cfg.GetStringMapString("does_not_exists"))
}

func TestGetStringMapStringSlice(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", map[string][]interface{}{"a": {1, 2}, "b": {"b", "bb"}, "c": nil})
	cfg.SetDefault("b", "{\"a\": [\"test\", \"test2\"]}") // viper handles JSON string implicitly so we have to reproduce this behavior
	cfg.BuildSchema()

	assert.Equal(t, map[string][]string{"a": {"1", "2"}, "b": {"b", "bb"}, "c": nil}, cfg.GetStringMapStringSlice("a"))
	assert.Equal(t, map[string][]string{"a": {"test", "test2"}}, cfg.GetStringMapStringSlice("b"))
	assert.Equal(t, map[string][]string{}, cfg.GetStringMapStringSlice("does_not_exists"))
}

func TestGetSizeInBytes(t *testing.T) {
	cfg := NewConfig("test", "", nil)
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
	cfg := NewConfig("test", "", nil)
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
	cfg := NewConfig("test", "", nil)
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
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("float_list", []string{})
	cfg.BuildSchema()
	cfg.Set("float_list", "1.1 2.2 3.3", model.SourceEnvVar)

	assert.Equal(t, []float64{1.1, 2.2, 3.3}, cfg.GetFloat64Slice("float_list"))
}

func TestGetAllSources(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.SetDefault("a", 0)
	cfg.BuildSchema()

	cfg.Set("a", 1, model.SourceUnknown)
	cfg.Set("a", 2, model.SourceFile)
	cfg.Set("a", 3, model.SourceEnvVar)
	cfg.Set("a", 4, model.SourceFleetPolicies)
	cfg.Set("a", 5, model.SourceAgentRuntime)
	cfg.Set("a", 6, model.SourceLocalConfigProcess)
	cfg.Set("a", 7, model.SourceRC)
	cfg.Set("a", 8, model.SourceCLI)

	res := cfg.GetAllSources("a")
	assert.Equal(t,
		[]model.ValueWithSource{
			{Source: model.SourceDefault, Value: 0},
			{Source: model.SourceUnknown, Value: 1},
			{Source: model.SourceFile, Value: 2},
			{Source: model.SourceEnvVar, Value: 3},
			{Source: model.SourceFleetPolicies, Value: 4},
			{Source: model.SourceAgentRuntime, Value: 5},
			{Source: model.SourceLocalConfigProcess, Value: 6},
			{Source: model.SourceRC, Value: 7},
			{Source: model.SourceCLI, Value: 8},
		},
		res,
	)
}
