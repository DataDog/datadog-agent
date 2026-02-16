// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAccessor is a simple SpanAccessor for testing.
type testAccessor struct {
	strings map[string]string
	floats  map[string]float64
	ints    map[string]int64
}

func (a *testAccessor) GetStringAttribute(key string) string {
	if a.strings == nil {
		return ""
	}
	return a.strings[key]
}

func (a *testAccessor) GetFloat64Attribute(key string) (float64, bool) {
	if a.floats == nil {
		return 0, false
	}
	v, ok := a.floats[key]
	return v, ok
}

func (a *testAccessor) GetInt64Attribute(key string) (int64, bool) {
	if a.ints == nil {
		return 0, false
	}
	v, ok := a.ints[key]
	return v, ok
}

func TestLookupString(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds string value", func(t *testing.T) {
		a := &testAccessor{strings: map[string]string{"db.statement": "SELECT 1"}}
		assert.Equal(t, "SELECT 1", LookupString(r, a, ConceptDBStatement))
	})

	t.Run("returns empty for missing", func(t *testing.T) {
		a := &testAccessor{strings: map[string]string{}}
		assert.Equal(t, "", LookupString(r, a, ConceptDBStatement))
	})
}

func TestLookupInt64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds int64 value", func(t *testing.T) {
		a := &testAccessor{ints: map[string]int64{"http.status_code": 200}}
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("returns false for missing", func(t *testing.T) {
		a := &testAccessor{}
		_, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.False(t, ok)
	})
}

func TestLookupFloat64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("converts int64 to float64", func(t *testing.T) {
		a := &testAccessor{ints: map[string]int64{"http.status_code": 200}}
		v, ok := LookupFloat64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)
	})
}

func TestLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("returns result with metadata", func(t *testing.T) {
		a := &testAccessor{strings: map[string]string{"db.statement": "SELECT 1"}}
		result := Lookup(r, a, ConceptDBStatement)
		assert.True(t, result.Found)
		assert.Equal(t, "db.statement", result.TagInfo.Name)
		assert.Equal(t, "SELECT 1", result.StringValue)
	})

	t.Run("unknown concept returns empty", func(t *testing.T) {
		a := &testAccessor{strings: map[string]string{"any": "value"}}
		result := Lookup(r, a, Concept("unknown"))
		assert.False(t, result.Found)
	})
}

func TestCombinedAccessor(t *testing.T) {
	primary := &testAccessor{strings: map[string]string{"key": "primary"}}
	secondary := &testAccessor{strings: map[string]string{"key": "secondary", "other": "value"}}
	combined := NewCombinedAccessor(primary, secondary)

	assert.Equal(t, "primary", combined.GetStringAttribute("key"))
	assert.Equal(t, "value", combined.GetStringAttribute("other"))
	assert.Equal(t, "", combined.GetStringAttribute("missing"))
}
