// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// TestResourcesAreValid checks the entries that derived lists and collectors index without checks.
func TestResourcesAreValid(t *testing.T) {
	seen := map[string]struct{}{}
	for _, resource := range Resources() {
		name := resource.Group + "/" + resource.Resource
		assert.NotEmpty(t, resource.Versions, name)
		assert.NotEqual(t, orchestrator.K8sUnsetType, resource.NodeType, name)
		if !resource.IsCustomResource() {
			assert.NotEmpty(t, resource.Resource, name)
			assert.NotEmpty(t, resource.Kind, name)
		}

		_, duplicate := seen[name]
		assert.False(t, duplicate, "duplicate entry %s", name)
		seen[name] = struct{}{}
	}
}

// TestEveryResourceHasFixtures checks that each version of each registry entry has a fixture, which the tests of the
// collectors and of the check use to cover the entry, and that each fixture matches an entry. Custom resources get
// generated fixtures.
func TestEveryResourceHasFixtures(t *testing.T) {
	builtinFixtures := ResourceFixtures()
	customFixtures, err := CustomResourceFixtures()
	require.NoError(t, err)

	entries := map[string]struct{}{}
	for _, resource := range Resources() {
		for _, version := range resource.Versions {
			name := resource.FixtureName(version)
			_, duplicate := entries[name]
			assert.False(t, duplicate, "two entries share the fixture %s", name)
			entries[name] = struct{}{}
			if resource.IsCustomResource() {
				assert.Contains(t, customFixtures, name, "no generated fixture for the custom resource %s", name)
				continue
			}
			assert.Contains(t, builtinFixtures, name, "the new resource %s has no test fixture: add one to ResourceFixtures in registry/testing.go", name)
		}
	}
	for name := range builtinFixtures {
		assert.Contains(t, entries, name, "the fixture %s matches no registry entry: remove it from ResourceFixtures", name)
	}
}
