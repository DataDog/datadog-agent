// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package converters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func loadTestData(t *testing.T, filename string) confMap {
	t.Helper()
	path := filepath.Join("td", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read test data file: %s", filename)

	retrieved, err := confmap.NewRetrievedFromYAML(data)
	require.NoError(t, err, "failed to parse YAML from: %s", filename)

	conf, err := retrieved.AsConf()
	require.NoError(t, err, "failed to convert to confmap from: %s", filename)

	return conf.ToStringMap()
}

func newObserverLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(level)
	return zap.New(core), logs
}

func TestGetBasicTypes(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_simple_values.yaml")

	// String
	strVal, ok := Get[string](cm, "string_value")
	require.True(t, ok)
	require.Equal(t, "test-string", strVal)

	// Int
	intVal, ok := Get[int](cm, "int_value")
	require.True(t, ok)
	require.Equal(t, 42, intVal)

	// Bool
	boolVal, ok := Get[bool](cm, "bool_value")
	require.True(t, ok)
	require.Equal(t, true, boolVal)

	// Float
	floatVal, ok := Get[float64](cm, "float_value")
	require.True(t, ok)
	require.Equal(t, 3.14, floatVal)
}

func TestGetNestedValues(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_nested_values.yaml")

	val, ok := Get[string](cm, "level1::level2::level3::deep_string")
	require.True(t, ok)
	require.Equal(t, "deep-value", val)

	numVal, ok := Get[int](cm, "level1::level2::level3::deep_number")
	require.True(t, ok)
	require.Equal(t, 999, numVal)
}

func TestGetMapAndArray(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_maps_and_arrays.yaml")

	// Get map
	mapVal, ok := Get[confMap](cm, "processors::batch")
	require.True(t, ok)
	require.Equal(t, "10s", mapVal["timeout"])

	// Get array
	arrVal, ok := Get[[]any](cm, "list_values")
	require.True(t, ok)
	require.Len(t, arrVal, 3)
	require.Equal(t, "item1", arrVal[0])
}

func TestGetNonExistentPath(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_simple_values.yaml")

	val, ok := Get[string](cm, "non_existent_field")
	require.False(t, ok)
	require.Equal(t, "", val) // Zero value

	// Nested non-existent
	_, ok = Get[string](cm, "level1::level2::missing")
	require.False(t, ok)
}

func TestGetWrongType(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_wrong_types.yaml")

	// Try to get string as int
	_, ok := Get[int](cm, "string_field")
	require.False(t, ok)

	// Try to get number as string
	_, ok = Get[string](cm, "number_field")
	require.False(t, ok)

	// Try to get map as string
	_, ok = Get[string](cm, "map_field")
	require.False(t, ok)
}

func TestGetIntermediateNodeNotMap(t *testing.T) {
	cm := loadTestData(t, "helper_functions/get_inter_nonmap.yaml")

	// Intermediate node is string
	_, ok := Get[string](cm, "processors::batch::timeout")
	require.False(t, ok)

	// Intermediate node is number
	_, ok = Get[string](cm, "receivers::otlp::protocols")
	require.False(t, ok)

	// Intermediate node is array
	_, ok = Get[string](cm, "exporters::otlphttp::headers")
	require.False(t, ok)
}

func TestSetBasicTypes(t *testing.T) {
	cm := confMap{}

	// Set string
	err := Set(cm, "string_value", "test")
	require.NoError(t, err)
	val, ok := Get[string](cm, "string_value")
	require.True(t, ok)
	require.Equal(t, "test", val)

	// Set int
	err = Set(cm, "int_value", 42)
	require.NoError(t, err)
	intVal, ok := Get[int](cm, "int_value")
	require.True(t, ok)
	require.Equal(t, 42, intVal)

	// Set bool
	err = Set(cm, "bool_value", true)
	require.NoError(t, err)
	boolVal, ok := Get[bool](cm, "bool_value")
	require.True(t, ok)
	require.Equal(t, true, boolVal)
}

func TestSetNestedPathCreatesIntermediates(t *testing.T) {
	cm := confMap{}

	err := Set(cm, "level1::level2::level3::value", "deep-value")
	require.NoError(t, err)

	// Verify intermediate maps were created
	_, ok := Get[confMap](cm, "level1")
	require.True(t, ok)
	_, ok = Get[confMap](cm, "level1::level2")
	require.True(t, ok)

	// Verify the value
	val, ok := Get[string](cm, "level1::level2::level3::value")
	require.True(t, ok)
	require.Equal(t, "deep-value", val)
}

func TestSetOverwritesExistingValue(t *testing.T) {
	cm := loadTestData(t, "helper_functions/set_overwrites.yaml")

	// Get original value
	origVal, ok := Get[string](cm, "processors::batch::timeout")
	require.True(t, ok)
	require.Equal(t, "10s", origVal)

	// Overwrite
	err := Set(cm, "processors::batch::timeout", "20s")
	require.NoError(t, err)

	// Verify new value
	newVal, ok := Get[string](cm, "processors::batch::timeout")
	require.True(t, ok)
	require.Equal(t, "20s", newVal)
}

func TestSetMapAndArray(t *testing.T) {
	cm := confMap{}

	// Set map
	newMap := confMap{"key1": "value1", "key2": 42}
	err := Set(cm, "nested::map", newMap)
	require.NoError(t, err)

	val, ok := Get[confMap](cm, "nested::map")
	require.True(t, ok)
	require.Equal(t, "value1", val["key1"])

	// Set array
	arr := []any{"item1", "item2"}
	err = Set(cm, "list::items", arr)
	require.NoError(t, err)

	arrVal, ok := Get[[]any](cm, "list::items")
	require.True(t, ok)
	require.Len(t, arrVal, 2)
}

func TestSetIntermediateNodeNotMap(t *testing.T) {
	cm := loadTestData(t, "helper_functions/set_inter_nonmap.yaml")

	// Intermediate node is string - should error
	err := Set(cm, "processors::batch::timeout", "10s")
	require.Error(t, err)
	require.Contains(t, err.Error(), "processors")

	// Intermediate node is number - should error
	err = Set(cm, "receivers::otlp::protocols", confMap{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "otlp")
}

func TestEnsureCreatesZeroValues(t *testing.T) {
	cm := confMap{}

	// String zero value
	strVal, err := Ensure[string](cm, "string_field")
	require.NoError(t, err)
	require.Equal(t, "", strVal)

	// Int zero value
	intVal, err := Ensure[int](cm, "int_field")
	require.NoError(t, err)
	require.Equal(t, 0, intVal)

	// Bool zero value
	boolVal, err := Ensure[bool](cm, "bool_field")
	require.NoError(t, err)
	require.Equal(t, false, boolVal)
}

func TestEnsureCreatesEmptyMapForMapTypes(t *testing.T) {
	cm := confMap{}

	mapVal, err := Ensure[confMap](cm, "processors")
	require.NoError(t, err)
	require.NotNil(t, mapVal)
	require.Empty(t, mapVal)

	// Verify it was set in the config
	retrieved, ok := Get[confMap](cm, "processors")
	require.True(t, ok)
	require.NotNil(t, retrieved)
}

func TestEnsureReturnsExistingValue(t *testing.T) {
	cm := confMap{
		"field": "existing-value",
	}

	val, err := Ensure[string](cm, "field")
	require.NoError(t, err)
	require.Equal(t, "existing-value", val)
}

func TestEnsureCreatesNestedPath(t *testing.T) {
	cm := confMap{}

	val, err := Ensure[int](cm, "a::b::c::d")
	require.NoError(t, err)
	require.Equal(t, 0, val)

	// Verify intermediate maps were created
	_, ok := Get[confMap](cm, "a::b::c")
	require.True(t, ok)
}

func TestEnsureErrorWhenIntermediateNotMap(t *testing.T) {
	cm := confMap{
		"processors": "not-a-map",
	}

	_, err := Ensure[bool](cm, "processors::batch::enabled")
	require.Error(t, err)
	require.Contains(t, err.Error(), "processors")
}

func TestConverterWithoutAgentLogsViaOTelLogger(t *testing.T) {
	logger, logs := newObserverLogger(zap.WarnLevel)

	conv := newConverterWithoutAgent(confmap.ConverterSettings{Logger: logger})
	conf := confmap.NewFromStringMap(loadTestData(t, "no_agent/symbol-up-disabled/in.yaml"))

	err := conv.Convert(context.Background(), conf)
	require.NoError(t, err)
	require.GreaterOrEqual(t, logs.Len(), 1, "expected at least one log from the converter")

	const expectedMsg = "Added minimal resourcedetection processor to user configuration"
	found := false
	for _, entry := range logs.All() {
		if entry.Level == zap.WarnLevel && entry.Message == expectedMsg {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about adding resourcedetection processor, got: %v", logs.All())
}

func TestConverterWithoutAgentLogsHostArchWarning(t *testing.T) {
	logger, logs := newObserverLogger(zap.DebugLevel)

	conv := newConverterWithoutAgent(confmap.ConverterSettings{Logger: logger})
	conf := confmap.NewFromStringMap(loadTestData(t, "no_agent/preserve-host-arch/in.yaml"))

	err := conv.Convert(context.Background(), conf)
	require.NoError(t, err)

	const expectedMsg = "host.arch is required but is disabled by user configuration; preserving user value. Profiles for compiled languages will be missing symbols."
	found := false
	for _, entry := range logs.All() {
		if entry.Message == expectedMsg {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about host.arch being disabled, got logs: %v", logs.All())
}
