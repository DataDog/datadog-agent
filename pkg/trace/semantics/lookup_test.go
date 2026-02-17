// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// newTestAccessor creates a PDataMapAccessor from a raw map for testing.
func newTestAccessor(raw map[string]any) *PDataMapAccessor {
	m := pcommon.NewMap()
	_ = m.FromRaw(raw)
	return NewPDataMapAccessor(m)
}

func TestLookupString(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds string value", func(t *testing.T) {
		a := newTestAccessor(map[string]any{"db.statement": "SELECT 1"})
		assert.Equal(t, "SELECT 1", LookupString(r, a, ConceptDBStatement))
	})

	t.Run("returns empty for missing", func(t *testing.T) {
		a := newTestAccessor(map[string]any{})
		assert.Equal(t, "", LookupString(r, a, ConceptDBStatement))
	})
}

func TestLookupInt64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds int64 value", func(t *testing.T) {
		a := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("returns false for missing", func(t *testing.T) {
		a := newTestAccessor(map[string]any{})
		_, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.False(t, ok)
	})
}

func TestLookupFloat64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("converts int64 to float64", func(t *testing.T) {
		// http.status_code is defined as int64 in mappings.json
		// LookupFloat64 handles the int64 -> float64 conversion
		a := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupFloat64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)
	})
}

func TestLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("returns result with metadata", func(t *testing.T) {
		a := newTestAccessor(map[string]any{"db.statement": "SELECT 1"})
		result, ok := Lookup(r, a, ConceptDBStatement)
		assert.True(t, ok)
		assert.Equal(t, "db.statement", result.TagInfo.Name)
		assert.Equal(t, "SELECT 1", result.StringValue)
	})

	t.Run("unknown concept returns false", func(t *testing.T) {
		a := newTestAccessor(map[string]any{"any": "value"})
		_, ok := Lookup(r, a, Concept("unknown"))
		assert.False(t, ok)
	})
}

func TestCombinedAccessor(t *testing.T) {
	primary := newTestAccessor(map[string]any{"key": "primary"})
	secondary := newTestAccessor(map[string]any{"key": "secondary", "other": "value"})
	combined := NewCombinedAccessor(primary, secondary)

	assert.Equal(t, "primary", combined.GetStringAttribute("key"))
	assert.Equal(t, "value", combined.GetStringAttribute("other"))
	assert.Equal(t, "", combined.GetStringAttribute("missing"))
}

func TestStringMapAccessor(t *testing.T) {
	t.Run("GetStringAttribute", func(t *testing.T) {
		a := NewStringMapAccessor(map[string]string{
			"key":   "value",
			"empty": "",
		})
		assert.Equal(t, "value", a.GetStringAttribute("key"))
		assert.Equal(t, "", a.GetStringAttribute("empty"))
		assert.Equal(t, "", a.GetStringAttribute("missing"))
	})

	t.Run("GetStringAttribute nil map", func(t *testing.T) {
		a := NewStringMapAccessor(nil)
		assert.Equal(t, "", a.GetStringAttribute("key"))
	})

	t.Run("GetInt64Attribute", func(t *testing.T) {
		a := NewStringMapAccessor(map[string]string{
			"int":     "200",
			"invalid": "not-a-number",
			"empty":   "",
		})
		v, ok := a.GetInt64Attribute("int")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		_, ok = a.GetInt64Attribute("invalid")
		assert.False(t, ok)

		_, ok = a.GetInt64Attribute("empty")
		assert.False(t, ok)

		_, ok = a.GetInt64Attribute("missing")
		assert.False(t, ok)
	})

	t.Run("GetFloat64Attribute", func(t *testing.T) {
		a := NewStringMapAccessor(map[string]string{
			"float":   "3.14",
			"int":     "200",
			"invalid": "not-a-number",
		})
		v, ok := a.GetFloat64Attribute("float")
		assert.True(t, ok)
		assert.Equal(t, 3.14, v)

		v, ok = a.GetFloat64Attribute("int")
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)

		_, ok = a.GetFloat64Attribute("invalid")
		assert.False(t, ok)
	})

	t.Run("with semantic lookup", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		a := NewStringMapAccessor(map[string]string{
			"http.status_code": "404",
		})
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)

		// Also works with string lookup
		s := LookupString(r, a, ConceptHTTPStatusCode)
		assert.Equal(t, "404", s)
	})
}
