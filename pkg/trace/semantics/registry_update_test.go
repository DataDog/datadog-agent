// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"strings"
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

const fingerprintBaseJSON = `{
  "version":"1.0.0",
  "git_commit":"commit-a",
  "timestamp":"2026-01-01T00:00:00Z",
  "metadata":{"content_hash":"same-declared-hash"},
  "concepts":{
    "alpha":{"canonical":"ignored.alpha","fallbacks":[
      {"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},
      {"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}
    ]},
    "beta":{"canonical":"ignored.beta","fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}
  }
}`

func registryFingerprint(t *testing.T, data string) string {
	t.Helper()
	r, err := NewRegistryFromJSON([]byte(data))
	require.NoError(t, err)
	require.Equal(t, "same-declared-hash", r.ContentHash())
	return r.Fingerprint()
}

func TestFingerprintChangesWithParsedMappings(t *testing.T) {
	base := registryFingerprint(t, fingerprintBaseJSON)
	tests := map[string]string{
		"fallback name": strings.Replace(fingerprintBaseJSON, `"name":"alpha.one"`, `"name":"alpha.changed"`, 1),
		"condition":     strings.Replace(fingerprintBaseJSON, `"eq":"client"`, `"eq":"server"`, 1),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, base, registryFingerprint(t, data))
		})
	}
}

func TestFingerprintChangesWithJSONPresentationAndMetadata(t *testing.T) {
	base := registryFingerprint(t, fingerprintBaseJSON)

	// Presentation-sensitive identity can over-invalidate on cosmetic changes.
	// That is accepted because rebuilding derived state is cheap, while missing a
	// payload change can leave that state stale.
	reformatted := strings.ReplaceAll(fingerprintBaseJSON, "  ", "    ")
	assert.NotEqual(t, base, registryFingerprint(t, reformatted))

	metadataChanged := strings.Replace(fingerprintBaseJSON, `"git_commit":"commit-a"`, `"git_commit":"commit-b"`, 1)
	assert.NotEqual(t, base, registryFingerprint(t, metadataChanged))
}

func TestFingerprintDeterministicAcrossLoads(t *testing.T) {
	want := registryFingerprint(t, fingerprintBaseJSON)
	for i := 0; i < 100; i++ {
		assert.Equal(t, want, registryFingerprint(t, fingerprintBaseJSON))
	}
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

func TestRegistryEqual_IdenticalPayloadBytesRegardlessOfSource(t *testing.T) {
	remote, err := NewRegistryFromJSON(mappingsJSON)
	require.NoError(t, err)
	embedded, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	require.NotEqual(t, remote.Source(), embedded.Source())
	assert.True(t, RegistryEqual(remote, embedded), "registries built from identical payload bytes compare equal regardless of source")
}

func TestRegistryEqual_DifferentPayloadBytesWhenVersionChanges(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "changing the version changes the payload bytes")
}

func TestRegistryEqual_DifferentPayloadBytesWhenMappingsChange(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "changing the mappings changes the payload bytes even when the declared hash is unchanged")
}

func TestRegistryEqual_DifferentPayloadBytesWhenDeclaredHashChanges(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-b"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "changing the producer-declared hash changes the payload bytes even when the mappings are unchanged")
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
