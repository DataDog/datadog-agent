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

func TestEmbeddedRegistryLoads(t *testing.T) {
	r := NewEmbeddedRegistry()
	require.NoError(t, r.LoadError())
	assert.NotEmpty(t, r.Version())
}

func TestGetAttributePrecedence_KnownConcept(t *testing.T) {
	r := NewEmbeddedRegistry()

	tests := []struct {
		name          string
		concept       Concept
		expectedFirst string
		minFallbacks  int
	}{
		{
			name:          "db.query has fallbacks",
			concept:       ConceptDBQuery,
			expectedFirst: "db.query.text",
			minFallbacks:  3,
		},
		{
			name:          "http.status_code has fallbacks",
			concept:       ConceptHTTPStatusCode,
			expectedFirst: "http.status_code",
			minFallbacks:  2,
		},
		{
			name:          "peer.hostname has many fallbacks",
			concept:       ConceptPeerHostname,
			expectedFirst: "peer.hostname",
			minFallbacks:  10,
		},
		{
			name:          "grpc status code has fallbacks",
			concept:       ConceptGRPCStatusCode,
			expectedFirst: "rpc.grpc.status_code",
			minFallbacks:  4,
		},
		{
			name:          "http.method has fallbacks",
			concept:       ConceptHTTPMethod,
			expectedFirst: "http.request.method",
			minFallbacks:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := r.GetAttributePrecedence(tt.concept)
			require.NotNil(t, tags, "expected tags for concept %s", tt.concept)
			require.GreaterOrEqual(t, len(tags), tt.minFallbacks, "expected at least %d fallbacks", tt.minFallbacks)
			assert.Equal(t, tt.expectedFirst, tags[0].Name, "first fallback should be %s", tt.expectedFirst)
		})
	}
}

func TestGetAttributePrecedence_UnknownConcept(t *testing.T) {
	r := NewEmbeddedRegistry()

	tags := r.GetAttributePrecedence(Concept("unknown.concept"))
	assert.Nil(t, tags)
}

func TestGetAttributePrecedence_NoFallbacks(t *testing.T) {
	r := NewEmbeddedRegistry()

	// Concepts with no fallbacks should return the canonical name
	tests := []Concept{
		ConceptPeerService,
		ConceptDDMeasured,
		ConceptMongoDBQuery,
	}

	for _, concept := range tests {
		t.Run(string(concept), func(t *testing.T) {
			tags := r.GetAttributePrecedence(concept)
			require.NotNil(t, tags)
			require.Len(t, tags, 1)
			assert.Equal(t, string(concept), tags[0].Name)
		})
	}
}

func TestGetAllEquivalences(t *testing.T) {
	r := NewEmbeddedRegistry()

	all := r.GetAllEquivalences()
	require.NotNil(t, all)

	// Check that we have a reasonable number of concepts
	assert.GreaterOrEqual(t, len(all), 40, "expected at least 40 concepts")

	// Verify some known concepts exist
	_, ok := all[ConceptDBQuery]
	assert.True(t, ok, "expected db.query concept")

	_, ok = all[ConceptHTTPStatusCode]
	assert.True(t, ok, "expected http.status_code concept")

	_, ok = all[ConceptPeerHostname]
	assert.True(t, ok, "expected peer.hostname concept")
}

func TestVersion(t *testing.T) {
	r := NewEmbeddedRegistry()

	version := r.Version()
	assert.NotEmpty(t, version)
	assert.Equal(t, "0.1.0", version)
}

func TestGetTagNames(t *testing.T) {
	r := NewEmbeddedRegistry()

	names := r.GetTagNames(ConceptDBQuery)
	require.NotNil(t, names)
	require.GreaterOrEqual(t, len(names), 3)

	// Verify the names are strings, not TagInfo
	assert.Equal(t, "db.query.text", names[0])
	assert.Equal(t, "db.statement", names[1])
}

func TestTagInfoMetadata(t *testing.T) {
	r := NewEmbeddedRegistry()

	tags := r.GetAttributePrecedence(ConceptDBQuery)
	require.NotNil(t, tags)

	// First tag should be OTel with version
	assert.Equal(t, "db.query.text", tags[0].Name)
	assert.Equal(t, ProviderOTel, tags[0].Provider)
	assert.Equal(t, "1.26.0", tags[0].Version)

	// Datadog tags should not have version
	found := false
	for _, tag := range tags {
		if tag.Name == "sql.query" {
			found = true
			assert.Equal(t, ProviderDatadog, tag.Provider)
			assert.Empty(t, tag.Version)
		}
	}
	assert.True(t, found, "expected to find sql.query tag")
}

func TestStorageMetadata(t *testing.T) {
	r := NewEmbeddedRegistry()

	// http.status_code has both metrics and meta storage
	tags := r.GetAttributePrecedence(ConceptHTTPStatusCode)
	require.NotNil(t, tags)

	hasMetrics := false
	hasMeta := false
	for _, tag := range tags {
		if tag.Storage == StorageMetrics {
			hasMetrics = true
		}
		if tag.Storage == StorageMeta || tag.Storage == "" {
			hasMeta = true
		}
	}

	assert.True(t, hasMetrics, "expected http.status_code to have metrics storage")
	assert.True(t, hasMeta, "expected http.status_code to have meta storage")
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	require.NotNil(t, r)

	tags := r.GetAttributePrecedence(ConceptDBQuery)
	assert.NotNil(t, tags)

	version := r.Version()
	assert.NotEmpty(t, version)
}

func TestNewRegistryFromJSON(t *testing.T) {
	customJSON := []byte(`{
		"version": "custom-1.0",
		"concepts": {
			"test.concept": {
				"canonical": "test.concept",
				"subsystems": ["test"],
				"fallbacks": [
					{"name": "test.attr1", "provider": "datadog"},
					{"name": "test.attr2", "provider": "otel", "version": "1.0.0"}
				]
			}
		}
	}`)

	r, err := NewRegistryFromJSON(customJSON)
	require.NoError(t, err)
	require.NotNil(t, r)

	assert.Equal(t, "custom-1.0", r.Version())

	tags := r.GetAttributePrecedence(Concept("test.concept"))
	require.NotNil(t, tags)
	require.Len(t, tags, 2)
	assert.Equal(t, "test.attr1", tags[0].Name)
	assert.Equal(t, ProviderDatadog, tags[0].Provider)
	assert.Equal(t, "test.attr2", tags[1].Name)
	assert.Equal(t, ProviderOTel, tags[1].Provider)
	assert.Equal(t, "1.0.0", tags[1].Version)
}

func TestNewRegistryFromJSON_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	r, err := NewRegistryFromJSON(invalidJSON)
	assert.Error(t, err)
	assert.Nil(t, r)
}

func TestConcurrentAccess(t *testing.T) {
	r := NewEmbeddedRegistry()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = r.GetAttributePrecedence(ConceptDBQuery)
				_ = r.GetAllEquivalences()
				_ = r.Version()
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkGetAttributePrecedence(b *testing.B) {
	r := NewEmbeddedRegistry()
	// Warm up
	_ = r.GetAttributePrecedence(ConceptDBQuery)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.GetAttributePrecedence(ConceptDBQuery)
	}
}

func BenchmarkGetAllEquivalences(b *testing.B) {
	r := NewEmbeddedRegistry()
	// Warm up
	_ = r.GetAllEquivalences()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.GetAllEquivalences()
	}
}
