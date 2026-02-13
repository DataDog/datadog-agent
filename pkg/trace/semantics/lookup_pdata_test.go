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
	t.Run("GetStringAttribute", func(t *testing.T) {
		attrs := pcommon.NewMap()
		attrs.PutStr("http.method", "GET")
		attrs.PutStr("db.statement", "SELECT * FROM users")
		attrs.PutInt("http.status_code", 200)

		accessor := NewPDataMapAccessor(attrs)

		assert.Equal(t, "GET", accessor.GetStringAttribute("http.method"))
		assert.Equal(t, "SELECT * FROM users", accessor.GetStringAttribute("db.statement"))
		// Int value should be converted to string via AsString()
		assert.Equal(t, "200", accessor.GetStringAttribute("http.status_code"))
		assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
	})

	t.Run("GetFloat64Attribute", func(t *testing.T) {
		attrs := pcommon.NewMap()
		attrs.PutDouble("custom.float", 3.14)
		attrs.PutInt("http.status_code", 200)
		attrs.PutStr("string.value", "not a number")

		accessor := NewPDataMapAccessor(attrs)

		v, ok := accessor.GetFloat64Attribute("custom.float")
		assert.True(t, ok)
		assert.Equal(t, 3.14, v)

		v, ok = accessor.GetFloat64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)

		_, ok = accessor.GetFloat64Attribute("string.value")
		assert.False(t, ok)

		_, ok = accessor.GetFloat64Attribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetInt64Attribute", func(t *testing.T) {
		attrs := pcommon.NewMap()
		attrs.PutInt("http.status_code", 200)
		attrs.PutDouble("custom.float", 3.14)
		attrs.PutStr("string.value", "not a number")

		accessor := NewPDataMapAccessor(attrs)

		v, ok := accessor.GetInt64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		v, ok = accessor.GetInt64Attribute("custom.float")
		assert.True(t, ok)
		assert.Equal(t, int64(3), v)

		_, ok = accessor.GetInt64Attribute("string.value")
		assert.False(t, ok)

		_, ok = accessor.GetInt64Attribute("nonexistent")
		assert.False(t, ok)
	})
}

func TestNewOTelSpanAccessor(t *testing.T) {
	t.Run("span attributes take precedence", func(t *testing.T) {
		spanAttrs := pcommon.NewMap()
		spanAttrs.PutStr("http.method", "POST")
		spanAttrs.PutInt("http.status_code", 201)

		resAttrs := pcommon.NewMap()
		resAttrs.PutStr("http.method", "GET")
		resAttrs.PutStr("service.name", "my-service")
		resAttrs.PutInt("http.status_code", 200)

		accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

		// Span attributes should take precedence
		assert.Equal(t, "POST", accessor.GetStringAttribute("http.method"))

		v, ok := accessor.GetInt64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(201), v)

		// Resource-only attributes should be accessible
		assert.Equal(t, "my-service", accessor.GetStringAttribute("service.name"))
	})

	t.Run("falls back to resource when span doesn't have attribute", func(t *testing.T) {
		spanAttrs := pcommon.NewMap()
		spanAttrs.PutStr("http.method", "GET")

		resAttrs := pcommon.NewMap()
		resAttrs.PutStr("service.name", "my-service")
		resAttrs.PutStr("deployment.environment", "production")

		accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

		assert.Equal(t, "GET", accessor.GetStringAttribute("http.method"))
		assert.Equal(t, "my-service", accessor.GetStringAttribute("service.name"))
		assert.Equal(t, "production", accessor.GetStringAttribute("deployment.environment"))
		assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
	})
}

func TestPDataAccessorWithRegistry(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("lookup db.query from OTel span", func(t *testing.T) {
		spanAttrs := pcommon.NewMap()
		spanAttrs.PutStr("db.statement", "SELECT * FROM users")

		resAttrs := pcommon.NewMap()

		accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
		result := LookupString(r, accessor, ConceptDBQuery)
		assert.Equal(t, "SELECT * FROM users", result)
	})

	t.Run("lookup http.status_code from OTel span", func(t *testing.T) {
		spanAttrs := pcommon.NewMap()
		spanAttrs.PutInt("http.status_code", 200)

		resAttrs := pcommon.NewMap()

		accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("lookup peer.hostname with fallbacks", func(t *testing.T) {
		tests := []struct {
			name     string
			spanKey  string
			spanVal  string
			expected string
		}{
			{
				name:     "peer.hostname direct",
				spanKey:  "peer.hostname",
				spanVal:  "host1",
				expected: "host1",
			},
			{
				name:     "net.peer.name fallback",
				spanKey:  "net.peer.name",
				spanVal:  "host2",
				expected: "host2",
			},
			{
				name:     "server.address fallback",
				spanKey:  "server.address",
				spanVal:  "host3",
				expected: "host3",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				spanAttrs := pcommon.NewMap()
				spanAttrs.PutStr(tt.spanKey, tt.spanVal)

				resAttrs := pcommon.NewMap()

				accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
				result := LookupString(r, accessor, ConceptPeerHostname)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("lookup grpc status code", func(t *testing.T) {
		spanAttrs := pcommon.NewMap()
		spanAttrs.PutInt("rpc.grpc.status_code", 2)

		resAttrs := pcommon.NewMap()

		accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
		v, ok := LookupInt64(r, accessor, ConceptGRPCStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(2), v)
	})
}

func BenchmarkPDataMapAccessor(b *testing.B) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")
	attrs.PutStr("db.statement", "SELECT * FROM users")
	attrs.PutInt("http.status_code", 200)

	accessor := NewPDataMapAccessor(attrs)

	b.Run("GetStringAttribute", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = accessor.GetStringAttribute("http.method")
		}
	})

	b.Run("GetInt64Attribute", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = accessor.GetInt64Attribute("http.status_code")
		}
	})
}

func BenchmarkOTelSpanAccessorWithRegistry(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("db.statement", "SELECT * FROM users")
	spanAttrs.PutInt("http.status_code", 200)

	resAttrs := pcommon.NewMap()
	resAttrs.PutStr("service.name", "my-service")

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	b.Run("LookupString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = LookupString(r, accessor, ConceptDBQuery)
		}
	})

	b.Run("LookupInt64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = LookupInt64(r, accessor, ConceptHTTPStatusCode)
		}
	})
}
