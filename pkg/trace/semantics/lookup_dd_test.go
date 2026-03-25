// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"math"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDDSpanAccessor(t *testing.T) {
	t.Run("GetString reads from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"span.kind": "server"},
			map[string]float64{},
		)
		assert.Equal(t, "server", a.GetString("span.kind"))
	})

	t.Run("GetString does not read from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"span.kind": 1},
		)
		assert.Equal(t, "", a.GetString("span.kind"))
	})

	t.Run("GetInt64 reads from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"http.status_code": 200},
		)
		v, ok := a.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("GetInt64 does not read from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"http.status_code": "200"},
			map[string]float64{},
		)
		_, ok := a.GetInt64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetFloat64 reads from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"sampling.priority": 1.0},
		)
		v, ok := a.GetFloat64("sampling.priority")
		assert.True(t, ok)
		assert.Equal(t, 1.0, v)
	})

	t.Run("GetFloat64 does not read from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"sampling.priority": "1.0"},
			map[string]float64{},
		)
		_, ok := a.GetFloat64("sampling.priority")
		assert.False(t, ok)
	})

	t.Run("http.status_code in metrics resolves via int64 registry entry", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		// Newer agents store http.status_code in Metrics as float64(200).
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"http.status_code": 200},
		)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		s := LookupString(r, a, ConceptHTTPStatusCode)
		assert.Equal(t, "200", s)
	})

	t.Run("http.status_code in meta resolves via string registry entry", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		// Older agents store http.status_code in Meta as a string.
		a := NewDDSpanAccessor(
			map[string]string{"http.status_code": "404"},
			map[string]float64{},
		)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)

		s := LookupString(r, a, ConceptHTTPStatusCode)
		assert.Equal(t, "404", s)
	})

	t.Run("nil maps return empty/false", func(t *testing.T) {
		a := NewDDSpanAccessor(nil, nil)
		assert.Equal(t, "", a.GetString("key"))
		_, ok := a.GetInt64("key")
		assert.False(t, ok)
		_, ok = a.GetFloat64("key")
		assert.False(t, ok)
	})
}

// newTestSpanV1 builds an InternalSpan and sets attributes via SetAttributeFromString
// (which stores integers as IntValue and strings as StringValueRef) and SetFloat64Attribute.
func newTestSpanV1() *idx.InternalSpan {
	st := idx.NewStringTable()
	s := idx.NewInternalSpan(st, &idx.Span{Attributes: make(map[uint32]*idx.AnyValue)})
	return s
}

func TestDDSpanAccessorV1(t *testing.T) {
	t.Run("GetString returns value for string attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetStringAttribute("http.method", "GET")
		a := NewDDSpanAccessorV1(s)
		assert.Equal(t, "GET", a.GetString("http.method"))
	})

	t.Run("GetString returns empty for int attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetAttributeFromString("http.status_code", "200") // stored as IntValue
		a := NewDDSpanAccessorV1(s)
		assert.Equal(t, "", a.GetString("http.status_code"))
	})

	t.Run("GetString returns empty for float attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("sampling.priority", 1.5)
		a := NewDDSpanAccessorV1(s)
		assert.Equal(t, "", a.GetString("sampling.priority"))
	})

	t.Run("GetString returns empty for missing key", func(t *testing.T) {
		s := newTestSpanV1()
		a := NewDDSpanAccessorV1(s)
		assert.Equal(t, "", a.GetString("missing"))
	})

	t.Run("GetFloat64 returns value for double attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("sampling.priority", 1.5)
		a := NewDDSpanAccessorV1(s)
		v, ok := a.GetFloat64("sampling.priority")
		assert.True(t, ok)
		assert.Equal(t, 1.5, v)
	})

	t.Run("GetFloat64 returns false for int attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetAttributeFromString("http.status_code", "200") // stored as IntValue
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetFloat64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetFloat64 returns false for string attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetStringAttribute("http.method", "GET")
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetFloat64("http.method")
		assert.False(t, ok)
	})

	t.Run("GetInt64 returns value for int attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetAttributeFromString("http.status_code", "200") // stored as IntValue
		a := NewDDSpanAccessorV1(s)
		v, ok := a.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("GetInt64 converts exact DoubleValue", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("http.status_code", 200.0)
		a := NewDDSpanAccessorV1(s)
		v, ok := a.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("GetInt64 rejects fractional DoubleValue", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("http.status_code", 200.5)
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetInt64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetInt64 rejects NaN", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("http.status_code", math.NaN())
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetInt64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetInt64 rejects Inf", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetFloat64Attribute("http.status_code", math.Inf(1))
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetInt64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetInt64 returns false for string attribute", func(t *testing.T) {
		s := newTestSpanV1()
		s.SetStringAttribute("http.method", "GET")
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetInt64("http.method")
		assert.False(t, ok)
	})

	t.Run("GetInt64 returns false for missing key", func(t *testing.T) {
		s := newTestSpanV1()
		a := NewDDSpanAccessorV1(s)
		_, ok := a.GetInt64("missing")
		assert.False(t, ok)
	})

	t.Run("integration: LookupInt64 with IntValue storage", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		s := newTestSpanV1()
		s.SetAttributeFromString("http.status_code", "404") // stored as IntValue
		a := NewDDSpanAccessorV1(s)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)
	})

	t.Run("integration: LookupInt64 with DoubleValue storage", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		s := newTestSpanV1()
		s.SetFloat64Attribute("http.status_code", 503.0)
		a := NewDDSpanAccessorV1(s)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(503), v)
	})
}

// BenchmarkStringLookup_DDSpan compares direct meta map access vs semantic LookupString
// on a DD V0 span.
func BenchmarkStringLookup_DDSpan(b *testing.B) {
	reg := DefaultRegistry()
	meta := map[string]string{"http.status_code": "200"}
	metrics := map[string]float64{}
	accessor := NewDDSpanAccessor(meta, metrics)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			_ = meta["http.status_code"]
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewDDSpanAccessor(meta, metrics)
			_ = LookupString(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkInt64Lookup_DDSpan compares direct metrics map access vs semantic LookupInt64
// on a DD V0 span.
func BenchmarkInt64Lookup_DDSpan(b *testing.B) {
	reg := DefaultRegistry()
	meta := map[string]string{}
	metrics := map[string]float64{"http.status_code": 200}
	accessor := NewDDSpanAccessor(meta, metrics)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			_ = metrics["http.status_code"]
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_, _ = LookupInt64(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewDDSpanAccessor(meta, metrics)
			_, _ = LookupInt64(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkStringLookup_DDSpanV1 compares direct InternalSpan attribute access vs semantic
// LookupString on a DD V1 span (string attribute path).
func BenchmarkStringLookup_DDSpanV1(b *testing.B) {
	reg := DefaultRegistry()
	s := newTestSpanV1()
	s.SetStringAttribute("http.status_code", "200")
	accessor := NewDDSpanAccessorV1(s)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			_, _ = s.GetAttributeAsString("http.status_code")
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_ = LookupString(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewDDSpanAccessorV1(s)
			_ = LookupString(reg, a, ConceptHTTPStatusCode)
		}
	})
}

// BenchmarkInt64Lookup_DDSpanV1 compares direct InternalSpan attribute access vs semantic
// LookupInt64 on a DD V1 span.
func BenchmarkInt64Lookup_DDSpanV1(b *testing.B) {
	reg := DefaultRegistry()
	s := newTestSpanV1()
	s.SetAttributeFromString("http.status_code", "200")
	accessor := NewDDSpanAccessorV1(s)

	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			_, _ = s.GetAttributeAsFloat64("http.status_code")
		}
	})

	b.Run("Semantic", func(b *testing.B) {
		for b.Loop() {
			_, _ = LookupInt64(reg, accessor, ConceptHTTPStatusCode)
		}
	})

	b.Run("SemanticWithAccessor", func(b *testing.B) {
		for b.Loop() {
			a := NewDDSpanAccessorV1(s)
			_, _ = LookupInt64(reg, a, ConceptHTTPStatusCode)
		}
	})
}
