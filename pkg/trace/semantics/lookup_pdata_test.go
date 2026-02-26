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

func TestPDataMapAccessor(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")
	attrs.PutStr("db.statement", "SELECT * FROM users")
	attrs.PutInt("http.status_code", 200)
	attrs.PutDouble("custom.float", 3.14)

	accessor := NewPDataMapAccessor(attrs)

	t.Run("GetString", func(t *testing.T) {
		assert.Equal(t, "GET", accessor.GetString("http.method"))
		assert.Equal(t, "SELECT * FROM users", accessor.GetString("db.statement"))
		// GetString is strict: returns "" for non-Str pdata types; use GetInt64/GetFloat64 for numerics.
		assert.Equal(t, "", accessor.GetString("http.status_code"))
		assert.Equal(t, "", accessor.GetString("custom.float"))
		assert.Equal(t, "", accessor.GetString("nonexistent"))
	})

	t.Run("GetInt64", func(t *testing.T) {
		v, ok := accessor.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		_, ok = accessor.GetInt64("http.method")
		assert.False(t, ok)

		_, ok = accessor.GetInt64("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetInt64 from exact float", func(t *testing.T) {
		m := pcommon.NewMap()
		m.PutDouble("code", 200.0)
		a := NewPDataMapAccessor(m)
		v, ok := a.GetInt64("code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("GetInt64 rejects non-integer float", func(t *testing.T) {
		m := pcommon.NewMap()
		m.PutDouble("code", 3.14)
		a := NewPDataMapAccessor(m)
		_, ok := a.GetInt64("code")
		assert.False(t, ok)
	})

	t.Run("GetFloat64", func(t *testing.T) {
		v, ok := accessor.GetFloat64("custom.float")
		assert.True(t, ok)
		assert.InDelta(t, 3.14, v, 0.001)

		v, ok = accessor.GetFloat64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)

		_, ok = accessor.GetFloat64("http.method")
		assert.False(t, ok)

		_, ok = accessor.GetFloat64("nonexistent")
		assert.False(t, ok)
	})
}

func TestNewOTelSpanAccessor(t *testing.T) {
	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("http.method", "POST")
	spanAttrs.PutInt("http.status_code", 500)

	resAttrs := pcommon.NewMap()
	resAttrs.PutStr("http.method", "GET")
	resAttrs.PutStr("service.name", "my-service")
	resAttrs.PutInt("http.status_code", 200)

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	t.Run("GetString precedence", func(t *testing.T) {
		assert.Equal(t, "POST", accessor.GetString("http.method"))
		assert.Equal(t, "my-service", accessor.GetString("service.name"))
		assert.Equal(t, "", accessor.GetString("nonexistent"))
	})

	t.Run("GetInt64 precedence", func(t *testing.T) {
		v, ok := accessor.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(500), v)
	})

	t.Run("GetFloat64 precedence", func(t *testing.T) {
		v, ok := accessor.GetFloat64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, float64(500), v)
	})
}

func TestPDataAccessorWithRegistry(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("db.statement", "SELECT * FROM users")

	resAttrs := pcommon.NewMap()

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
	result := LookupString(r, accessor, ConceptDBStatement)
	assert.Equal(t, "SELECT * FROM users", result)
}

func TestPDataAccessorTypedLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	spanAttrs := pcommon.NewMap()
	spanAttrs.PutInt("http.status_code", 404)

	resAttrs := pcommon.NewMap()

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	t.Run("LookupInt64 uses typed path", func(t *testing.T) {
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)
	})

	t.Run("LookupFloat64 uses typed path", func(t *testing.T) {
		v, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(404), v)
	})
}
