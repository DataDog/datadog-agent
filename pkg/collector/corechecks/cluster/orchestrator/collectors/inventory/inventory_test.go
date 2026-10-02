// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// genericMetadataByNameVersion flattens the default generic collectors into a
// (name, version) -> metadata map.
func genericMetadataByNameVersion(t *testing.T) map[[2]string]*collectors.CollectorMetadata {
	t.Helper()
	out := map[[2]string]*collectors.CollectorMetadata{}
	for _, cv := range getGenericCollectorVersions() {
		for _, collector := range cv.Collectors {
			m := collector.Metadata()
			out[[2]string{m.Name, m.Version}] = m
		}
	}
	return out
}

// TestDRAGenericResourcesRegistered guards the WS3 change: DRA's built-in
// resource.k8s.io types ride the generic manifest channel (NodeType: K8sCR,
// IsManifestProducer: true) rather than first-class backend types. If either
// entry is dropped, ResourceClaim/ResourceSlice stop appearing in Kubernetes
// Explorer with no error.
func TestDRAGenericResourcesRegistered(t *testing.T) {
	byNameVersion := genericMetadataByNameVersion(t)

	for _, name := range []string{"resourceclaims", "resourceslices"} {
		m, ok := byNameVersion[[2]string{name, "v1"}]
		require.True(t, ok, "generic collector %q is not registered in defaultGenericResource", name)

		assert.Equal(t, "resource.k8s.io", m.Group, "%s must use the resource.k8s.io group", name)
		assert.Equal(t, pkgorchestratormodel.NodeType(pkgorchestratormodel.K8sCR), m.NodeType, "%s must ride the generic manifest channel (K8sCR)", name)
		assert.True(t, m.IsManifestProducer, "%s must be a manifest producer", name)
		assert.True(t, m.IsGenericCollector, "%s must be a generic collector", name)
	}
}

// TestDRAGenericResourcesCoverBetaVersions pins version coverage. Discovery
// matches a collector on its exact GroupVersion and skips a miss without
// logging, so registering only v1 -- which exists from Kubernetes 1.34 -- means
// collecting nothing at all, silently, on every cluster serving a beta version.
func TestDRAGenericResourcesCoverBetaVersions(t *testing.T) {
	byNameVersion := genericMetadataByNameVersion(t)

	for _, name := range []string{"resourceclaims", "resourceslices"} {
		for _, version := range []string{"v1", "v1beta2", "v1beta1"} {
			m, ok := byNameVersion[[2]string{name, version}]
			require.True(t, ok, "%s must be registered for %s", name, version)
			assert.Equal(t, "resource.k8s.io/"+version, m.GroupVersion())
		}
	}
}

// TestGenericResourcesHaveExactlyOneDefaultVersion pins the invariant that
// makes multi-version registration safe. Discovery dedupes by name, but the
// fallback used when discovery fails selects on IsStable && IsDefaultVersion --
// so a second default version there would start an informer that can only 404.
func TestGenericResourcesHaveExactlyOneDefaultVersion(t *testing.T) {
	defaults := map[string]int{}
	names := map[string]struct{}{}
	for _, cv := range getGenericCollectorVersions() {
		for _, collector := range cv.Collectors {
			m := collector.Metadata()
			names[m.Name] = struct{}{}
			if m.IsDefaultVersion {
				defaults[m.Name]++
			}
		}
	}

	for name := range names {
		assert.Equal(t, 1, defaults[name], "generic resource %q must have exactly one default version", name)
	}
}

// TestDRAGenericResourcesAreOptIn pins the activation model. Marking these
// stable makes APIServerDiscoveryProvider.walkAPIResources activate them on
// every cluster serving resource.k8s.io -- including those whose Cluster Agent
// has no RBAC for the group, where the LIST 403s and Initialize stalls for the
// full cache-sync plus extra-sync timeout before skipping them. Unstable keeps
// discovery off them while addCollectorFromConfig still honours an explicit
// collectors list, so the feature stays reachable without that cost.
func TestDRAGenericResourcesAreOptIn(t *testing.T) {
	byNameVersion := genericMetadataByNameVersion(t)

	for _, name := range []string{"resourceclaims", "resourceslices"} {
		for _, version := range []string{"v1", "v1beta2", "v1beta1"} {
			m, ok := byNameVersion[[2]string{name, version}]
			require.True(t, ok, "%s must be registered for %s", name, version)
			assert.False(t, m.IsStable, "%s/%s must stay opt-in until the charts ship resource.k8s.io RBAC", name, version)
		}
	}
}
