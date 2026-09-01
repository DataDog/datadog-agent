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
		"fallback name":   `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.changed","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"provider":        `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"otel","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"version":         `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"2.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"type":            `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"int64"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"fallback order":  `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]},{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"when attribute":  `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"operation","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"when present":    `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":false,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"when equality":   `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"server"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]}}}`,
		"added concept":   `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]},"beta":{"fallbacks":[{"name":"beta.one","provider":"datadog","type":"string"}]},"gamma":{"fallbacks":[{"name":"gamma.one","provider":"otel","type":"string"}]}}}`,
		"removed concept": `{"version":"1","metadata":{"content_hash":"same-declared-hash"},"concepts":{"alpha":{"fallbacks":[{"name":"alpha.one","provider":"datadog","version":"1.0","type":"string"},{"name":"alpha.two","provider":"otel","type":"float64","when":[{"attribute":"span.kind","present":true,"eq":"client"}]}]}}}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, base, registryFingerprint(t, data))
		})
	}
}

func TestFingerprintIgnoresJSONPresentationAndMetadata(t *testing.T) {
	reformatted := `{"timestamp":"2030-12-31T23:59:59Z","concepts":{"beta":{"fallbacks":[{"type":"string","provider":"datadog","name":"beta.one"}],"canonical":"changed.but.ignored"},"alpha":{"fallbacks":[{"type":"string","version":"1.0","provider":"datadog","name":"alpha.one"},{"when":[{"eq":"client","present":true,"attribute":"span.kind"}],"type":"float64","name":"alpha.two","provider":"otel"}],"canonical":"also.changed"}},"metadata":{"content_hash":"same-declared-hash"},"git_commit":"commit-b","version":"9.9.9"}`

	assert.Equal(t, registryFingerprint(t, fingerprintBaseJSON), registryFingerprint(t, reformatted))
}

func TestFingerprintFieldBoundariesAreUnambiguous(t *testing.T) {
	withinFallbacksA := map[Concept][]TagInfo{"concept": {{Name: "ab"}, {Name: ""}}}
	withinFallbacksB := map[Concept][]TagInfo{"concept": {{Name: "a"}, {Name: "b"}}}
	assert.NotEqual(t, fingerprint(withinFallbacksA), fingerprint(withinFallbacksB))

	acrossConceptAndFieldA := map[Concept][]TagInfo{"ab": {{Name: "c"}}}
	acrossConceptAndFieldB := map[Concept][]TagInfo{"a": {{Name: "bc"}}}
	assert.NotEqual(t, fingerprint(acrossConceptAndFieldA), fingerprint(acrossConceptAndFieldB))
}

func TestFingerprintDistinguishesUnsetConditionFields(t *testing.T) {
	t.Run("present absent versus false", func(t *testing.T) {
		presentFalse := false
		absent := map[Concept][]TagInfo{"concept": {{Name: "tag", When: []Condition{{Attribute: "attribute"}}}}}
		set := map[Concept][]TagInfo{"concept": {{Name: "tag", When: []Condition{{Attribute: "attribute", Present: &presentFalse}}}}}

		assert.NotEqual(t, fingerprint(absent), fingerprint(set))
	})

	t.Run("eq absent versus empty", func(t *testing.T) {
		empty := ""
		absent := map[Concept][]TagInfo{"concept": {{Name: "tag", When: []Condition{{Attribute: "attribute"}}}}}
		set := map[Concept][]TagInfo{"concept": {{Name: "tag", When: []Condition{{Attribute: "attribute", Eq: &empty}}}}}

		assert.NotEqual(t, fingerprint(absent), fingerprint(set))
	})
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

func TestRegistryEqual_SameMappingsDifferentVersion(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.True(t, RegistryEqual(a, b), "metadata changes do not change parsed concept mappings")
}

func TestRegistryEqual_SameHashDifferentMappings(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.False(t, RegistryEqual(a, b), "the declared hash does not override changed parsed mappings")
}

func TestRegistryEqual_DifferentHashSameMappings(t *testing.T) {
	a, err := NewRegistryFromJSON([]byte(`{"version":"1.0.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	b, err := NewRegistryFromJSON([]byte(`{"version":"2.0.0","metadata":{"content_hash":"hash-b"},"concepts":{"db.statement":{"canonical":"different.canonical","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`))
	require.NoError(t, err)
	assert.True(t, RegistryEqual(a, b))
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
