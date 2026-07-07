// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistryFromJSON_ValidJSON(t *testing.T) {
	r, err := NewRegistryFromJSON(mappingsJSON)
	require.NoError(t, err)
	embedded, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	for concept := range embedded.mappings {
		assert.NotNil(t, r.GetAttributePrecedence(concept), "concept %s should be present", concept)
	}
}

func TestNewRegistryFromJSON_MalformedJSON(t *testing.T) {
	_, err := NewRegistryFromJSON([]byte("not valid json"))
	assert.Error(t, err)
}

func TestNewRegistryFromJSON_EmptyConcepts(t *testing.T) {
	_, err := NewRegistryFromJSON([]byte(`{"version":"0.1.0","metadata":{"content_hash":"hash-a"},"concepts":{}}`))
	assert.Error(t, err)
}

func TestNewRegistryFromJSON_MissingContentHash(t *testing.T) {
	_, err := NewRegistryFromJSON([]byte(`{"version":"0.1.0","concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	assert.Error(t, err)
}

func TestUpdateRegistry_AtomicSwap(t *testing.T) {
	original, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	t.Cleanup(func() { UpdateRegistry(original) })

	customJSON := `{"version":"test-version","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	custom, err := NewRegistryFromJSON([]byte(customJSON))
	require.NoError(t, err)

	UpdateRegistry(custom)
	assert.Equal(t, "test-version", DefaultRegistry().Version())
}

func TestRegistryEqual_SameHashDifferentVersion(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.True(t, RegistryEqual(a, b), "same content_hash means same concepts, regardless of the CI-bumped version string")
}

func TestRegistryEqual_DifferentHashSameVersion(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-b"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "differing content_hash means the concepts changed, even if version happens to match")
}

func TestRegistryEqual_DifferentHash(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"hash-b"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b))
}

func TestRegistryEqual_NilHandling(t *testing.T) {
	assert.True(t, RegistryEqual(nil, nil))
	r, err := NewRegistryFromJSON(mappingsJSON)
	require.NoError(t, err)
	assert.False(t, RegistryEqual(nil, r))
	assert.False(t, RegistryEqual(r, nil))
}

func TestUpdateRegistry_ConcurrentReadWrite(_ *testing.T) {
	const goroutines = 10
	const iterations = 500

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = DefaultRegistry().GetAttributePrecedence(ConceptDBStatement)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < iterations; j++ {
			r, err := NewEmbeddedRegistry()
			if err == nil {
				UpdateRegistry(r)
			}
		}
	}()

	wg.Wait()
}
