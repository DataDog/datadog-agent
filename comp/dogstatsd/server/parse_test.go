// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifyEvent(t *testing.T) {
	metricSample := []byte("_e{4,5}:title|text|#shell,bash")
	messageType := findMessageType(metricSample)
	assert.Equal(t, eventType, messageType)
}

func TestIdentifyServiceCheck(t *testing.T) {
	metricSample := []byte("_sc|NAME|STATUS|d:TIMESTAMP|h:HOSTNAME|#TAG_KEY_1:TAG_VALUE_1,TAG_2|m:SERVICE_CHECK_MESSAGE")
	messageType := findMessageType(metricSample)
	assert.Equal(t, serviceCheckType, messageType)
}

func TestIdentifyMetricSample(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestIdentifyRandomString(t *testing.T) {
	metricSample := []byte("song.length:240|h|@0.5")
	messageType := findMessageType(metricSample)
	assert.Equal(t, metricSampleType, messageType)
}

func TestParseTags(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	rawTags := []byte("tag:test,mytag,good:boy")
	tags := p.parseTags(rawTags)
	expectedTags := []string{"tag:test", "mytag", "good:boy"}
	assert.ElementsMatch(t, expectedTags, tags)
}

func TestParseTagsEmpty(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	rawTags := []byte("")
	tags := p.parseTags(rawTags)
	assert.Nil(t, tags)
}

func TestUnsafeParseFloat(t *testing.T) {
	rawFloat := "1.1234"

	unsafeFloat, err := parseFloat64([]byte(rawFloat))
	assert.NoError(t, err)
	float, err := strconv.ParseFloat(rawFloat, 64)
	assert.NoError(t, err)

	assert.Equal(t, float, unsafeFloat)
}

func TestUnsafeParseFloatList(t *testing.T) {
	deps := newServerDeps(t)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	p := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	unsafeFloats, err := p.parseFloat64List([]byte("1.1234:21.5:13"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 3)
	assert.Equal(t, []float64{1.1234, 21.5, 13}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 1)
	assert.Equal(t, []float64{1.1234}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234:41:"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte("1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	unsafeFloats, err = p.parseFloat64List([]byte(":1.1234::41"))
	assert.NoError(t, err)
	assert.Len(t, unsafeFloats, 2)
	assert.Equal(t, []float64{1.1234, 41}, unsafeFloats)

	_, err = p.parseFloat64List([]byte(""))
	assert.Error(t, err)
}

func TestUnsafeParseInt(t *testing.T) {
	rawInt := "123"

	unsafeInteger, err := parseInt64([]byte(rawInt))
	assert.NoError(t, err)
	integer, err := strconv.ParseInt(rawInt, 10, 64)
	assert.NoError(t, err)

	assert.Equal(t, integer, unsafeInteger)
}

// FuzzParseFloat64 tests the parseFloat64 function with arbitrary input.
// The function should behave identically to strconv.ParseFloat and never panic.
func FuzzParseFloat64(f *testing.F) {
	// Add seed corpus based on existing tests
	f.Add([]byte("1.1234"))
	f.Add([]byte("0"))
	f.Add([]byte("-1.1234"))
	f.Add([]byte("1e10"))
	f.Add([]byte("1.7976931348623157e+308"))  // Max float64
	f.Add([]byte("-1.7976931348623157e+308")) // Min float64
	f.Add([]byte("2.2250738585072014e-308"))  // Smallest positive normal
	f.Add([]byte("NaN"))
	f.Add([]byte("Inf"))
	f.Add([]byte("-Inf"))
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("invalid"))
	f.Add([]byte("1.234.567"))
	f.Add([]byte("1,234"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that parseFloat64 behaves identically to strconv.ParseFloat
		unsafeResult, unsafeErr := parseFloat64(data)

		// Create a string from the data to use with strconv
		str := string(data)
		stdResult, stdErr := strconv.ParseFloat(str, 64)

		// Both should either succeed or fail together
		if (unsafeErr == nil) != (stdErr == nil) {
			t.Errorf("parseFloat64(%q): error mismatch. unsafe error: %v, std error: %v", data, unsafeErr, stdErr)
		}

		// If both succeeded, results should match
		if unsafeErr == nil && stdErr == nil {
			if unsafeResult != stdResult {
				// Special case for NaN - both should be NaN
				if !(math.IsNaN(unsafeResult) && math.IsNaN(stdResult)) {
					t.Errorf("parseFloat64(%q): result mismatch. unsafe: %v, std: %v", data, unsafeResult, stdResult)
				}
			}
		}
	})
}

// FuzzParseInt64 tests the parseInt64 function with arbitrary input.
// The function should behave identically to strconv.ParseInt and never panic.
func FuzzParseInt64(f *testing.F) {
	// Add seed corpus based on existing tests
	f.Add([]byte("123"))
	f.Add([]byte("0"))
	f.Add([]byte("-123"))
	f.Add([]byte("9223372036854775807"))  // Max int64
	f.Add([]byte("-9223372036854775808")) // Min int64
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("invalid"))
	f.Add([]byte("123.456"))
	f.Add([]byte("0x123"))
	f.Add([]byte("123abc"))
	f.Add([]byte("abc123"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that parseInt64 behaves identically to strconv.ParseInt
		unsafeResult, unsafeErr := parseInt64(data)

		// Create a string from the data to use with strconv
		str := string(data)
		stdResult, stdErr := strconv.ParseInt(str, 10, 64)

		// Both should either succeed or fail together
		if (unsafeErr == nil) != (stdErr == nil) {
			t.Errorf("parseInt64(%q): error mismatch. unsafe error: %v, std error: %v", data, unsafeErr, stdErr)
		}

		// If both succeeded, results should match
		if unsafeErr == nil && stdErr == nil {
			if unsafeResult != stdResult {
				t.Errorf("parseInt64(%q): result mismatch. unsafe: %v, std: %v", data, unsafeResult, stdResult)
			}
		}
	})
}

// FuzzParseInt tests the parseInt function with arbitrary input.
// The function should behave identically to strconv.Atoi and never panic.
func FuzzParseInt(f *testing.F) {
	// Add seed corpus based on existing tests
	f.Add([]byte("123"))
	f.Add([]byte("0"))
	f.Add([]byte("-123"))
	f.Add([]byte("2147483647"))  // Max int32 (common int size)
	f.Add([]byte("-2147483648")) // Min int32
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("invalid"))
	f.Add([]byte("123.456"))
	f.Add([]byte("0x123"))
	f.Add([]byte("123abc"))
	f.Add([]byte("abc123"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that parseInt behaves identically to strconv.Atoi
		unsafeResult, unsafeErr := parseInt(data)

		// Create a string from the data to use with strconv
		str := string(data)
		stdResult, stdErr := strconv.Atoi(str)

		// Both should either succeed or fail together
		if (unsafeErr == nil) != (stdErr == nil) {
			t.Errorf("parseInt(%q): error mismatch. unsafe error: %v, std error: %v", data, unsafeErr, stdErr)
		}

		// If both succeeded, results should match
		if unsafeErr == nil && stdErr == nil {
			if unsafeResult != stdResult {
				t.Errorf("parseInt(%q): result mismatch. unsafe: %v, std: %v", data, unsafeResult, stdResult)
			}
		}
	})
}
