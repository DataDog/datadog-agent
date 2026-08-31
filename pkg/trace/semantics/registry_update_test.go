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

const dbStatementContentHash = "sha256:a055d6c59d7d4d260cb9d0c99a29b5d894f05b8e48cf8f2ccaaa86ed956e7e73"

func TestComputeContentHash_PythonProducerFixture(t *testing.T) {
	data := []byte(`{"version":"1.0.0","metadata":{"content_hash":"ignored"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`)

	hash, err := computeContentHash(data)

	require.NoError(t, err)
	assert.Equal(t, dbStatementContentHash, hash)
}

func TestComputeContentHash_PythonStringEncoding(t *testing.T) {
	data := []byte(`{"concepts":{"test.concept":{"canonical":"test.concept","fallbacks":[{"name":"café😀<>&\u007f","present":true,"provider":"datadog","type":"string"}]}}}`)

	hash, err := computeContentHash(data)

	require.NoError(t, err)
	assert.Equal(t, "sha256:208924fbfc6e72e944566069bcf77f744af72bbea6a5f4e0158048a670297d3b", hash)
}

func TestNewRegistryFromJSON_RejectsChangedContentWithOldHash(t *testing.T) {
	data := []byte(`{"version":"1.0.0","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"changed.statement","provider":"datadog","type":"string"}]}}}`)

	_, err := NewRegistryFromJSON(data)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "content hash mismatch")
}

func TestNewRegistryFromJSON_AcceptsReorderedKeysAndWhitespace(t *testing.T) {
	data := []byte(`{
  "concepts": {
    "db.statement": {
      "fallbacks": [{"type": "string", "provider": "datadog", "name": "db.statement"}],
      "canonical": "db.statement"
    }
  },
  "metadata": {"content_hash": "` + dbStatementContentHash + `"},
  "version": "1.0.0"
}`)

	r, err := NewRegistryFromJSON(data)

	require.NoError(t, err)
	assert.Equal(t, dbStatementContentHash, r.ContentHash())
}

func TestComputeContentHash_IgnoresVersionAndMetadata(t *testing.T) {
	a := []byte(`{"version":"1.0.0","metadata":{"content_hash":"old","source":"one"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`)
	b := []byte(`{"version":"2.0.0","metadata":{"content_hash":"new","source":"two"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`)

	hashA, err := computeContentHash(a)
	require.NoError(t, err)
	hashB, err := computeContentHash(b)
	require.NoError(t, err)

	assert.Equal(t, dbStatementContentHash, hashA)
	assert.Equal(t, dbStatementContentHash, hashB)
}

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

	customJSON := `{"version":"test-version","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	custom, err := NewRegistryFromJSON([]byte(customJSON))
	require.NoError(t, err)

	UpdateRegistry(custom)
	assert.Equal(t, "test-version", DefaultRegistry().Version())
}

func TestRegistryEqual_SameHashDifferentVersion(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.True(t, RegistryEqual(a, b), "same content_hash means same concepts, regardless of the CI-bumped version string")
}

func TestRegistryEqual_DifferentHashSameVersion(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"sha256:de824759184887dc3a653771c8ff8c7366525dcfe3f62674cdf8747c86030ca2"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "differing content_hash means the concepts changed, even if version happens to match")
}

func TestRegistryEqual_DifferentHash(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"` + dbStatementContentHash + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"sha256:de824759184887dc3a653771c8ff8c7366525dcfe3f62674cdf8747c86030ca2"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`))
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
