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

func TestEmbeddedRegistry(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	assert.NotEmpty(t, r.Version())
}

func TestGetAttributePrecedence(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("known concept returns tags", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptDBStatement)
		require.NotNil(t, tags)
		assert.Greater(t, len(tags), 0)
	})

	t.Run("unknown concept returns nil", func(t *testing.T) {
		tags := r.GetAttributePrecedence(Concept("unknown"))
		assert.Nil(t, tags)
	})
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	require.NotNil(t, r)
	assert.NotNil(t, r.GetAttributePrecedence(ConceptDBStatement))
}

func TestPartialVersionRegistryType(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	tags := r.GetAttributePrecedence(ConceptDDPartialVersion)
	require.Len(t, tags, 1, "ConceptDDPartialVersion should have exactly one fallback")
	assert.Equal(t, ValueTypeFloat64, tags[0].Type, "_dd.partial_version must be float64 to match the Metrics map storage type")
}

func TestDDTagConceptsRegistered(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	concepts := []struct {
		concept Concept
		key     string
	}{
		{ConceptDDEnv, "env"},
		{ConceptDDVersion, "version"},
		{ConceptDDHostname, "_dd.hostname"},
		{ConceptDDGitCommitSHA, "_dd.git.commit.sha"},
		{ConceptDDDecisionMaker, "_dd.p.dm"},
		{ConceptDDAPMMode, "_dd.apm_mode"},
	}

	for _, tc := range concepts {
		t.Run(string(tc.concept), func(t *testing.T) {
			tags := r.GetAttributePrecedence(tc.concept)
			require.Len(t, tags, 1, "concept should have exactly one fallback")
			assert.Equal(t, tc.key, tags[0].Name)
			assert.Equal(t, ProviderDatadog, tags[0].Provider)
			assert.Equal(t, ValueTypeString, tags[0].Type)
		})
	}
}

func TestDDTagConceptLookup(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	meta := map[string]string{
		"env":                "production",
		"version":            "1.2.3",
		"_dd.hostname":       "my-host",
		"_dd.git.commit.sha": "abc123",
		"_dd.p.dm":           "-4",
		"_dd.apm_mode":       "apm",
	}
	accessor := NewStringMapAccessor(meta)

	assert.Equal(t, "production", LookupString(r, accessor, ConceptDDEnv))
	assert.Equal(t, "1.2.3", LookupString(r, accessor, ConceptDDVersion))
	assert.Equal(t, "my-host", LookupString(r, accessor, ConceptDDHostname))
	assert.Equal(t, "abc123", LookupString(r, accessor, ConceptDDGitCommitSHA))
	assert.Equal(t, "-4", LookupString(r, accessor, ConceptDDDecisionMaker))
	assert.Equal(t, "apm", LookupString(r, accessor, ConceptDDAPMMode))
}
