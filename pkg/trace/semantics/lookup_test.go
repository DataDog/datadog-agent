// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func newTestAccessor(raw map[string]any) PDataMapAccessor {
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

	t.Run("finds int64 value via typed accessor", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("finds int64 value from string map accessor", func(t *testing.T) {
		accessor := NewStringMapAccessor(map[string]string{"http.status_code": "200"})
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

	t.Run("converts int64 to float64 via typed accessor", func(t *testing.T) {
		accessor := newTestAccessor(map[string]any{"http.status_code": int64(200)})
		v, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)
	})

	t.Run("converts int64 to float64 from string map accessor", func(t *testing.T) {
		accessor := NewStringMapAccessor(map[string]string{"http.status_code": "200"})
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

func TestStringMapAccessor(t *testing.T) {
	t.Run("returns value for key", func(t *testing.T) {
		accessor := NewStringMapAccessor(map[string]string{"key": "value", "empty": ""})
		assert.Equal(t, "value", accessor.GetString("key"))
		assert.Equal(t, "", accessor.GetString("empty"))
		assert.Equal(t, "", accessor.GetString("missing"))
	})

	t.Run("nil map returns empty", func(t *testing.T) {
		accessor := NewStringMapAccessor(nil)
		assert.Equal(t, "", accessor.GetString("key"))
	})

	t.Run("numeric accessors always return not-found", func(t *testing.T) {
		// StringMapAccessor is strictly typed: numeric registry entries are skipped
		// so that we never do a string→int→string round-trip (e.g. in LookupString).
		accessor := NewStringMapAccessor(map[string]string{"key": "42"})
		_, ok := accessor.GetInt64("key")
		assert.False(t, ok, "StringMapAccessor.GetInt64 should not parse strings")
		_, ok = accessor.GetFloat64("key")
		assert.False(t, ok, "StringMapAccessor.GetFloat64 should not parse strings")
	})

	t.Run("with semantic lookup", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		accessor := NewStringMapAccessor(map[string]string{"http.status_code": "404"})
		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)

		s := LookupString(r, accessor, ConceptHTTPStatusCode)
		assert.Equal(t, "404", s)
	})
}

// Benchmarks comparing direct attribute access (old approach) vs semantic registry lookup (new approach).

// BenchmarkStringLookup_PData compares direct pdata.Map.Get with semantic LookupString.
func BenchmarkStringLookup_PData(b *testing.B) {
	reg := DefaultRegistry()
	attrs := pcommon.NewMap()
	attrs.PutStr("http.request.method", "GET")
	attrs.PutStr("http.route", "/api/v1/users")
	attrs.PutStr("db.system", "postgresql")
	accessor := NewPDataMapAccessor(attrs)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := attrs.Get("http.request.method"); ok {
				_ = v.AsString()
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPMethod)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewPDataMapAccessor(attrs)
			_ = LookupString(reg, a, ConceptHTTPMethod)
		}
	})
}

// BenchmarkStringLookup_Fallback compares direct multi-key access vs semantic lookup when
// the value is stored under a legacy key that isn't first in precedence order.
// For ConceptHTTPMethod, precedence is: http.request.method (new), http.method (old).
func BenchmarkStringLookup_Fallback(b *testing.B) {
	reg := DefaultRegistry()
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET") // old key, second in precedence
	accessor := NewPDataMapAccessor(attrs)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := attrs.Get("http.request.method"); ok {
				_ = v.AsString()
			} else if v, ok := attrs.Get("http.method"); ok {
				_ = v.AsString()
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPMethod)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewPDataMapAccessor(attrs)
			_ = LookupString(reg, a, ConceptHTTPMethod)
		}
	})
}

// BenchmarkInt64Lookup_PData compares direct typed pdata access vs semantic LookupInt64.
func BenchmarkInt64Lookup_PData(b *testing.B) {
	reg := DefaultRegistry()
	attrs := pcommon.NewMap()
	attrs.PutInt("http.status_code", 200)
	accessor := NewPDataMapAccessor(attrs)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := attrs.Get("http.status_code"); ok {
				_ = v.Int()
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_, _ = LookupInt64(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewPDataMapAccessor(attrs)
			_, _ = LookupInt64(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkStringLookup_StringMap compares direct map access vs semantic lookup on map[string]string.
func BenchmarkStringLookup_StringMap(b *testing.B) {
	reg := DefaultRegistry()
	m := map[string]string{
		"http.status_code": "200",
		"http.method":      "GET",
		"http.route":       "/api/v1/users",
	}
	accessor := NewStringMapAccessor(m)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			_ = m["http.status_code"]
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewStringMapAccessor(m)
			_ = LookupString(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkInt64Lookup_StringMap compares direct map access + parse vs semantic LookupInt64
// on a map[string]string (string parsing fallback path).
func BenchmarkInt64Lookup_StringMap(b *testing.B) {
	reg := DefaultRegistry()
	m := map[string]string{"http.status_code": "200"}
	accessor := NewStringMapAccessor(m)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := m["http.status_code"]; ok {
				_, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_, _ = LookupInt64(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewStringMapAccessor(m)
			_, _ = LookupInt64(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkDualMapLookup simulates the common OTel pattern of checking span attrs then
// resource attrs. Compares direct two-map access vs semantic lookup with OTelSpanAccessor.
func BenchmarkDualMapLookup(b *testing.B) {
	reg := DefaultRegistry()
	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("http.request.method", "GET")
	spanAttrs.PutStr("http.route", "/api/v1/users")
	spanAttrs.PutInt("http.status_code", 200)

	resAttrs := pcommon.NewMap()
	resAttrs.PutStr("service.name", "my-service")
	resAttrs.PutStr("deployment.environment", "production")

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := spanAttrs.Get("http.request.method"); ok {
				_ = v.AsString()
			} else if v, ok := spanAttrs.Get("http.method"); ok {
				_ = v.AsString()
			} else if v, ok := resAttrs.Get("http.request.method"); ok {
				_ = v.AsString()
			} else if v, ok := resAttrs.Get("http.method"); ok {
				_ = v.AsString()
			}

			if v, ok := spanAttrs.Get("http.route"); ok {
				_ = v.AsString()
			} else if v, ok := resAttrs.Get("http.route"); ok {
				_ = v.AsString()
			}

			if v, ok := spanAttrs.Get("http.status_code"); ok {
				_ = v.Int()
			} else if v, ok := resAttrs.Get("http.status_code"); ok {
				_ = v.Int()
			}

			if v, ok := spanAttrs.Get("deployment.environment"); ok {
				_ = v.AsString()
			} else if v, ok := spanAttrs.Get("deployment.environment.name"); ok {
				_ = v.AsString()
			} else if v, ok := resAttrs.Get("deployment.environment"); ok {
				_ = v.AsString()
			} else if v, ok := resAttrs.Get("deployment.environment.name"); ok {
				_ = v.AsString()
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPMethod)
			_ = LookupString(reg, accessor, ConceptHTTPRoute)
			_, _ = LookupInt64(reg, accessor, ConceptHTTPStatusCode)
			_ = LookupString(reg, accessor, ConceptDeploymentEnv)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewOTelSpanAccessor(spanAttrs, resAttrs)
			_ = LookupString(reg, a, ConceptHTTPMethod)
			_ = LookupString(reg, a, ConceptHTTPRoute)
			_, _ = LookupInt64(reg, a, ConceptHTTPStatusCode)
			_ = LookupString(reg, a, ConceptDeploymentEnv)
		}
	})
}

// BenchmarkLookup_Miss compares the cost of looking up a concept that has no matching
// attribute in the map (worst case: all fallback keys checked, none found).
func BenchmarkLookup_Miss(b *testing.B) {
	reg := DefaultRegistry()
	attrs := pcommon.NewMap()
	attrs.PutStr("unrelated.attr", "value")
	accessor := NewPDataMapAccessor(attrs)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			if v, ok := attrs.Get("http.request.method"); ok {
				_ = v.AsString()
			} else if v, ok := attrs.Get("http.method"); ok {
				_ = v.AsString()
			}
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPMethod)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewPDataMapAccessor(attrs)
			_ = LookupString(reg, a, ConceptHTTPMethod)
		}
	})
}
