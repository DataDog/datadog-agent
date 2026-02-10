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

func TestDDSpanAccessor(t *testing.T) {
	t.Run("GetStringAttribute", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"http.status_code": "200",
				"db.statement":     "SELECT * FROM users",
			},
		}

		assert.Equal(t, "200", accessor.GetStringAttribute("http.status_code"))
		assert.Equal(t, "SELECT * FROM users", accessor.GetStringAttribute("db.statement"))
		assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
	})

	t.Run("GetStringAttribute nil map", func(t *testing.T) {
		accessor := &DDSpanAccessor{}
		assert.Equal(t, "", accessor.GetStringAttribute("any.key"))
	})

	t.Run("GetFloat64Attribute", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code":    200,
				"_sampling_priority":  1,
				"custom.float.metric": 3.14,
			},
		}

		v, ok := accessor.GetFloat64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)

		v, ok = accessor.GetFloat64Attribute("custom.float.metric")
		assert.True(t, ok)
		assert.Equal(t, 3.14, v)

		_, ok = accessor.GetFloat64Attribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetFloat64Attribute nil map", func(t *testing.T) {
		accessor := &DDSpanAccessor{}
		_, ok := accessor.GetFloat64Attribute("any.key")
		assert.False(t, ok)
	})

	t.Run("GetInt64Attribute", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}

		v, ok := accessor.GetInt64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		_, ok = accessor.GetInt64Attribute("nonexistent")
		assert.False(t, ok)
	})
}

func TestLookupString(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("db.query with db.statement", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT * FROM users",
			},
		}

		result := LookupString(r, accessor, ConceptDBQuery)
		assert.Equal(t, "SELECT * FROM users", result)
	})

	t.Run("db.query with db.query.text (higher precedence)", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.query.text": "SELECT * FROM orders",
				"db.statement":  "SELECT * FROM users",
			},
		}

		result := LookupString(r, accessor, ConceptDBQuery)
		assert.Equal(t, "SELECT * FROM orders", result)
	})

	t.Run("db.query with sql.query", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"sql.query": "SELECT * FROM products",
			},
		}

		result := LookupString(r, accessor, ConceptDBQuery)
		assert.Equal(t, "SELECT * FROM products", result)
	})

	t.Run("unknown concept returns empty", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"some.key": "value",
			},
		}

		result := LookupString(r, accessor, Concept("unknown.concept"))
		assert.Equal(t, "", result)
	})

	t.Run("no matching attribute returns empty", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"unrelated.key": "value",
			},
		}

		result := LookupString(r, accessor, ConceptDBQuery)
		assert.Equal(t, "", result)
	})
}

func TestLookupFloat64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("http.status_code from metrics", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}

		v, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)
	})

	t.Run("http.status_code from meta (string)", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"http.status_code": "404",
			},
		}

		v, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(404), v)
	})

	t.Run("grpc.status_code from metrics", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"rpc.grpc.status_code": 2,
			},
		}

		v, ok := LookupFloat64(r, accessor, ConceptGRPCStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(2), v)
	})

	t.Run("not found returns false", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{},
		}

		_, ok := LookupFloat64(r, accessor, ConceptHTTPStatusCode)
		assert.False(t, ok)
	})
}

func TestLookupInt64(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("http.status_code from metrics", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}

		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("http.status_code from meta (string)", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"http.status_code": "500",
			},
		}

		v, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(500), v)
	})

	t.Run("not found returns false", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{},
		}

		_, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
		assert.False(t, ok)
	})
}

func TestLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("returns detailed result", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT * FROM users",
			},
		}

		result := Lookup(r, accessor, ConceptDBQuery)
		assert.True(t, result.Found)
		assert.Equal(t, "db.statement", result.Key)
		assert.Equal(t, "SELECT * FROM users", result.StringValue)
		assert.Equal(t, ProviderOTel, result.TagInfo.Provider)
		assert.Equal(t, ValueTypeString, result.TagInfo.Type)
	})

	t.Run("returns first matching in precedence order", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.query.text": "SELECT 1",
				"db.statement":  "SELECT 2",
				"sql.query":     "SELECT 3",
			},
		}

		result := Lookup(r, accessor, ConceptDBQuery)
		assert.True(t, result.Found)
		assert.Equal(t, "db.query.text", result.Key)
		assert.Equal(t, "SELECT 1", result.StringValue)
	})

	t.Run("not found returns empty result", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{},
		}

		result := Lookup(r, accessor, ConceptDBQuery)
		assert.False(t, result.Found)
		assert.Equal(t, "", result.Key)
		assert.Equal(t, "", result.StringValue)
	})

	t.Run("unknown concept returns empty result", func(t *testing.T) {
		accessor := &DDSpanAccessor{
			Meta: map[string]string{
				"some.key": "value",
			},
		}

		result := Lookup(r, accessor, Concept("unknown.concept"))
		assert.False(t, result.Found)
	})
}

func TestLookupFromMaps(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("string lookup", func(t *testing.T) {
		meta := map[string]string{
			"db.statement": "SELECT * FROM users",
		}
		metrics := map[string]float64{}

		result := LookupFromMaps(r, meta, metrics, ConceptDBQuery)
		assert.True(t, result.Found)
		assert.Equal(t, "SELECT * FROM users", result.StringValue)
	})

	t.Run("numeric lookup from metrics", func(t *testing.T) {
		meta := map[string]string{}
		metrics := map[string]float64{
			"http.status_code": 200,
		}

		result := LookupFromMaps(r, meta, metrics, ConceptHTTPStatusCode)
		assert.True(t, result.Found)
		assert.Equal(t, int64(200), result.Int64Value)
	})

	t.Run("convenience functions", func(t *testing.T) {
		meta := map[string]string{
			"db.statement": "SELECT 1",
		}
		metrics := map[string]float64{
			"http.status_code": 200,
		}

		str := LookupStringFromMaps(r, meta, metrics, ConceptDBQuery)
		assert.Equal(t, "SELECT 1", str)

		f, ok := LookupFloat64FromMaps(r, meta, metrics, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, float64(200), f)

		i, ok := LookupInt64FromMaps(r, meta, metrics, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), i)
	})
}

func TestGetAttributeKeys(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("returns all keys for concept", func(t *testing.T) {
		keys := GetAttributeKeys(r, ConceptDBQuery)
		require.NotNil(t, keys)
		assert.Contains(t, keys, "db.query.text")
		assert.Contains(t, keys, "db.statement")
		assert.Contains(t, keys, "sql.query")
		assert.Contains(t, keys, "mongodb.query")
	})

	t.Run("unknown concept returns nil", func(t *testing.T) {
		keys := GetAttributeKeys(r, Concept("unknown.concept"))
		assert.Nil(t, keys)
	})
}

func TestGetAttributeKeysForType(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("string type keys", func(t *testing.T) {
		keys := GetAttributeKeysForType(r, ConceptHTTPStatusCode, ValueTypeString)
		require.NotNil(t, keys)
		// Should include string-typed status code keys
		assert.Contains(t, keys, "http.response.status_code")
	})

	t.Run("int64 type keys", func(t *testing.T) {
		keys := GetAttributeKeysForType(r, ConceptHTTPStatusCode, ValueTypeInt64)
		require.NotNil(t, keys)
		// Should include int64-typed status code keys
		assert.Contains(t, keys, "http.status_code")
	})

	t.Run("unknown concept returns nil", func(t *testing.T) {
		keys := GetAttributeKeysForType(r, Concept("unknown.concept"), ValueTypeString)
		assert.Nil(t, keys)
	})
}

func TestOTelMapAccessor(t *testing.T) {
	t.Run("GetStringAttribute", func(t *testing.T) {
		accessor := &OTelMapAccessor{
			Attributes: map[string]interface{}{
				"http.method":     "GET",
				"http.status_code": 200, // int, not string
			},
		}

		assert.Equal(t, "GET", accessor.GetStringAttribute("http.method"))
		assert.Equal(t, "", accessor.GetStringAttribute("http.status_code")) // int doesn't convert
		assert.Equal(t, "", accessor.GetStringAttribute("nonexistent"))
	})

	t.Run("GetStringAttribute nil map", func(t *testing.T) {
		accessor := &OTelMapAccessor{}
		assert.Equal(t, "", accessor.GetStringAttribute("any.key"))
	})

	t.Run("GetFloat64Attribute", func(t *testing.T) {
		accessor := &OTelMapAccessor{
			Attributes: map[string]interface{}{
				"float64_val": float64(3.14),
				"float32_val": float32(2.71),
				"int_val":     42,
				"int64_val":   int64(100),
				"int32_val":   int32(50),
				"string_val":  "not a number",
			},
		}

		v, ok := accessor.GetFloat64Attribute("float64_val")
		assert.True(t, ok)
		assert.Equal(t, 3.14, v)

		v, ok = accessor.GetFloat64Attribute("float32_val")
		assert.True(t, ok)
		assert.InDelta(t, 2.71, v, 0.01)

		v, ok = accessor.GetFloat64Attribute("int_val")
		assert.True(t, ok)
		assert.Equal(t, float64(42), v)

		v, ok = accessor.GetFloat64Attribute("int64_val")
		assert.True(t, ok)
		assert.Equal(t, float64(100), v)

		_, ok = accessor.GetFloat64Attribute("string_val")
		assert.False(t, ok)

		_, ok = accessor.GetFloat64Attribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetInt64Attribute", func(t *testing.T) {
		accessor := &OTelMapAccessor{
			Attributes: map[string]interface{}{
				"int64_val":   int64(100),
				"int_val":     42,
				"int32_val":   int32(50),
				"float64_val": float64(3.14),
				"string_val":  "not a number",
			},
		}

		v, ok := accessor.GetInt64Attribute("int64_val")
		assert.True(t, ok)
		assert.Equal(t, int64(100), v)

		v, ok = accessor.GetInt64Attribute("int_val")
		assert.True(t, ok)
		assert.Equal(t, int64(42), v)

		v, ok = accessor.GetInt64Attribute("float64_val")
		assert.True(t, ok)
		assert.Equal(t, int64(3), v)

		_, ok = accessor.GetInt64Attribute("string_val")
		assert.False(t, ok)
	})
}

func TestCombinedAccessor(t *testing.T) {
	t.Run("GetStringAttribute returns first match", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"http.method": "POST",
			},
		}
		resAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"http.method": "GET",
				"service.name": "my-service",
			},
		}

		combined := NewCombinedAccessor(spanAccessor, resAccessor)

		// Span accessor has http.method, should return that
		assert.Equal(t, "POST", combined.GetStringAttribute("http.method"))
		// Only resource accessor has service.name
		assert.Equal(t, "my-service", combined.GetStringAttribute("service.name"))
		// Neither has this
		assert.Equal(t, "", combined.GetStringAttribute("nonexistent"))
	})

	t.Run("GetFloat64Attribute returns first match", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}
		resAccessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 500,
				"custom.metric":    42,
			},
		}

		combined := NewCombinedAccessor(spanAccessor, resAccessor)

		v, ok := combined.GetFloat64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, float64(200), v)

		v, ok = combined.GetFloat64Attribute("custom.metric")
		assert.True(t, ok)
		assert.Equal(t, float64(42), v)

		_, ok = combined.GetFloat64Attribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetInt64Attribute returns first match", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}
		resAccessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 500,
			},
		}

		combined := NewCombinedAccessor(spanAccessor, resAccessor)

		v, ok := combined.GetInt64Attribute("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})
}

func TestLookupWithFallback(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("span takes precedence over resource", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT * FROM span",
			},
		}
		resAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT * FROM resource",
			},
		}

		result := LookupWithFallback(r, ConceptDBQuery, spanAccessor, resAccessor)
		assert.True(t, result.Found)
		assert.Equal(t, "SELECT * FROM span", result.StringValue)
	})

	t.Run("falls back to resource when span doesn't have it", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Meta: map[string]string{},
		}
		resAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT * FROM resource",
			},
		}

		result := LookupWithFallback(r, ConceptDBQuery, spanAccessor, resAccessor)
		assert.True(t, result.Found)
		assert.Equal(t, "SELECT * FROM resource", result.StringValue)
	})

	t.Run("convenience functions", func(t *testing.T) {
		spanAccessor := &DDSpanAccessor{
			Meta: map[string]string{
				"db.statement": "SELECT 1",
			},
		}
		resAccessor := &DDSpanAccessor{
			Metrics: map[string]float64{
				"http.status_code": 200,
			},
		}

		str := LookupStringWithFallback(r, ConceptDBQuery, spanAccessor, resAccessor)
		assert.Equal(t, "SELECT 1", str)

		f, ok := LookupFloat64WithFallback(r, ConceptHTTPStatusCode, spanAccessor, resAccessor)
		assert.True(t, ok)
		assert.Equal(t, float64(200), f)

		i, ok := LookupInt64WithFallback(r, ConceptHTTPStatusCode, spanAccessor, resAccessor)
		assert.True(t, ok)
		assert.Equal(t, int64(200), i)
	})
}

func TestPeerTagsLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("peer.hostname with multiple fallbacks", func(t *testing.T) {
		tests := []struct {
			name     string
			meta     map[string]string
			expected string
		}{
			{
				name:     "peer.hostname direct",
				meta:     map[string]string{"peer.hostname": "host1"},
				expected: "host1",
			},
			{
				name:     "hostname fallback",
				meta:     map[string]string{"hostname": "host2"},
				expected: "host2",
			},
			{
				name:     "net.peer.name fallback",
				meta:     map[string]string{"net.peer.name": "host3"},
				expected: "host3",
			},
			{
				name:     "db.hostname fallback",
				meta:     map[string]string{"db.hostname": "host4"},
				expected: "host4",
			},
			{
				name:     "out.host fallback",
				meta:     map[string]string{"out.host": "host5"},
				expected: "host5",
			},
			{
				name:     "precedence: peer.hostname over hostname",
				meta:     map[string]string{"peer.hostname": "host1", "hostname": "host2"},
				expected: "host1",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				accessor := &DDSpanAccessor{Meta: tt.meta}
				result := LookupString(r, accessor, ConceptPeerHostname)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("peer.db.name with multiple fallbacks", func(t *testing.T) {
		tests := []struct {
			name     string
			meta     map[string]string
			expected string
		}{
			{
				name:     "db.name direct",
				meta:     map[string]string{"db.name": "mydb"},
				expected: "mydb",
			},
			{
				name:     "mongodb.db fallback",
				meta:     map[string]string{"mongodb.db": "mongodb"},
				expected: "mongodb",
			},
			{
				name:     "db.instance fallback",
				meta:     map[string]string{"db.instance": "instance1"},
				expected: "instance1",
			},
			{
				name:     "cassandra.keyspace fallback",
				meta:     map[string]string{"cassandra.keyspace": "keyspace1"},
				expected: "keyspace1",
			},
			{
				name:     "db.namespace fallback",
				meta:     map[string]string{"db.namespace": "namespace1"},
				expected: "namespace1",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				accessor := &DDSpanAccessor{Meta: tt.meta}
				result := LookupString(r, accessor, ConceptPeerDBName)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestHTTPStatusCodeLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("precedence order", func(t *testing.T) {
		tests := []struct {
			name     string
			meta     map[string]string
			metrics  map[string]float64
			expected int64
		}{
			{
				name:     "http.status_code in metrics (int64 type)",
				metrics:  map[string]float64{"http.status_code": 200},
				expected: 200,
			},
			{
				name:     "http.status_code in meta (string type)",
				meta:     map[string]string{"http.status_code": "404"},
				expected: 404,
			},
			{
				name:     "http.response.status_code in meta",
				meta:     map[string]string{"http.response.status_code": "500"},
				expected: 500,
			},
			{
				name:     "metrics int64 takes precedence over meta string",
				meta:     map[string]string{"http.status_code": "404"},
				metrics:  map[string]float64{"http.status_code": 200},
				expected: 200,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				accessor := &DDSpanAccessor{Meta: tt.meta, Metrics: tt.metrics}
				result, ok := LookupInt64(r, accessor, ConceptHTTPStatusCode)
				assert.True(t, ok)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestGRPCStatusCodeLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("precedence order", func(t *testing.T) {
		tests := []struct {
			name     string
			meta     map[string]string
			metrics  map[string]float64
			expected int64
		}{
			{
				name:     "rpc.grpc.status_code in metrics",
				metrics:  map[string]float64{"rpc.grpc.status_code": 2},
				expected: 2,
			},
			{
				name:     "grpc.code in metrics",
				metrics:  map[string]float64{"grpc.code": 3},
				expected: 3,
			},
			{
				name:     "rpc.grpc.status.code in metrics",
				metrics:  map[string]float64{"rpc.grpc.status.code": 4},
				expected: 4,
			},
			{
				name:     "grpc.status.code in metrics",
				metrics:  map[string]float64{"grpc.status.code": 5},
				expected: 5,
			},
			{
				name:     "rpc.grpc.status_code in meta (string)",
				meta:     map[string]string{"rpc.grpc.status_code": "6"},
				expected: 6,
			},
			{
				name:     "grpc.code in meta (string)",
				meta:     map[string]string{"grpc.code": "7"},
				expected: 7,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				accessor := &DDSpanAccessor{Meta: tt.meta, Metrics: tt.metrics}
				result, ok := LookupInt64(r, accessor, ConceptGRPCStatusCode)
				assert.True(t, ok)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

// Benchmark tests
func BenchmarkLookupString(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	accessor := &DDSpanAccessor{
		Meta: map[string]string{
			"db.statement": "SELECT * FROM users",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LookupString(r, accessor, ConceptDBQuery)
	}
}

func BenchmarkLookupInt64(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	accessor := &DDSpanAccessor{
		Metrics: map[string]float64{
			"http.status_code": 200,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LookupInt64(r, accessor, ConceptHTTPStatusCode)
	}
}

func BenchmarkLookupFromMaps(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	meta := map[string]string{
		"db.statement": "SELECT * FROM users",
	}
	metrics := map[string]float64{
		"http.status_code": 200,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LookupFromMaps(r, meta, metrics, ConceptDBQuery)
	}
}

func BenchmarkLookupWithFallback(b *testing.B) {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		b.Fatal(err)
	}

	spanAccessor := &DDSpanAccessor{
		Meta: map[string]string{},
	}
	resAccessor := &DDSpanAccessor{
		Meta: map[string]string{
			"db.statement": "SELECT * FROM users",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LookupWithFallback(r, ConceptDBQuery, spanAccessor, resAccessor)
	}
}

