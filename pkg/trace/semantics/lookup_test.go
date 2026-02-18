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

func newTestAccessor(raw map[string]any) AttrGetter {
	m := pcommon.NewMap()
	_ = m.FromRaw(raw)
	return NewPDataMapAccessor(m)
}

func TestLookupString(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds string value", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"db.statement": "SELECT 1"})
		assert.Equal(t, "SELECT 1", LookupString(r, accessor, ConceptDBStatement))
	})

	t.Run("returns empty for missing", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{})
		assert.Equal(t, "", LookupString(r, accessor, ConceptDBStatement))
	})
}

func TestLookupInt64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("finds int64 value", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("returns false for missing", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{})
		_, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.False(t, ok)
	})
}

func TestLookupFloat64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("converts int64 to float64", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)
	})
}

func TestLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("returns result with metadata", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"db.statement": "SELECT 1"})
		result, ok := Lookup(r, accessor, ConceptDBStatement)
		assert.True(t, ok)
		assert.Equal(t, "db.statement", result.TagInfo.Name)
		assert.Equal(t, "SELECT 1", result.StringValue)
	})

	t.Run("unknown concept returns false", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"any": "value"})
		_, ok := Lookup(r, accessor, Concept("unknown"))
		assert.False(t, ok)
	})
}

func TestNewCombinedAccessor(t *testing.T) {
	primary := newTestAccessor(map[string]any{"key": "primary"})
	secondary := newTestAccessor(map[string]any{"key": "secondary", "other": "value"})
	combined := NewCombinedAccessor(primary, secondary)

	assert.Equal(t, "primary", combined("key"))
	assert.Equal(t, "value", combined("other"))
	assert.Equal(t, "", combined("missing"))
}

func TestStringMapAccessor(t *testing.T) {
	t.Run("returns value for key", func(t *testing.T) {
		accessor := NewStringMapAccessor(map[string]string{"key": "value", "empty": ""})
		assert.Equal(t, "value", accessor("key"))
		assert.Equal(t, "", accessor("empty"))
		assert.Equal(t, "", accessor("missing"))
	})

	t.Run("nil map returns empty", func(t *testing.T) {
		accessor := NewStringMapAccessor(nil)
		assert.Equal(t, "", accessor("key"))
	})

	t.Run("with semantic lookup", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		accessor := NewStringMapAccessor(map[string]string{"http.status_code": "404"})
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)

		// Also works with string lookup
		s := LookupString(r, accessor, ConceptHTTPStatusCode)
		assert.Equal(t, "404", s)
	})
}
