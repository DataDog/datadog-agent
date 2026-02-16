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

	t.Run("GetStringAttribute", func(t *testing.T) {
		assert.Equal(t, "GET", accessor.GetStringAttribute("http.method"))
		// Only returns value for actual string types, not conversions
		assert.Equal(t, "", accessor.GetStringAttribute("http.status_code"))
		assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
	})

	t.Run("GetFloat64Attribute", func(t *testing.T) {
		v, ok := accessor.GetFloat64Attribute("custom.float")
		assert.True(t, ok)
		assert.Equal(t, 3.14, v)

		// Only returns value for actual double types, not conversions
		_, ok = accessor.GetFloat64Attribute("http.status_code")
		assert.False(t, ok)

		_, ok = accessor.GetFloat64Attribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetInt64Attribute", func(t *testing.T) {
		v, ok := accessor.GetInt64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		_, ok = accessor.GetInt64Attribute("nonexistent")
		assert.False(t, ok)
	})
}

func TestNewOTelSpanAccessor(t *testing.T) {
	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("http.method", "POST")

	resAttrs := pcommon.NewMap()
	resAttrs.PutStr("http.method", "GET")
	resAttrs.PutStr("service.name", "my-service")

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	// Span takes precedence
	assert.Equal(t, "POST", accessor.GetStringAttribute("http.method"))
	// Falls back to resource
	assert.Equal(t, "my-service", accessor.GetStringAttribute("service.name"))
	// Missing returns empty
	assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
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
