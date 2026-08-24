// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// unstructuredForGroup returns the ExpectedType a custom resource factory would
// report for the given API group, mirroring customresourcestate's ExpectedType.
func unstructuredForGroup(group, version, kind string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
	return u
}

// TestCustomResourceClientKeyIsGroupAware ensures two CRDs that share the same
// Kind/plural but differ by API group produce distinct client keys, so they do
// not collide onto a single client (CONS-8512).
func TestCustomResourceClientKeyIsGroupAware(t *testing.T) {
	const (
		resource = "projects"
		version  = "v1alpha1"
		kind     = "Project"
	)

	artifactory := unstructuredForGroup("artifactory.example.com", version, kind)
	sonarqube := unstructuredForGroup("sonarqube.example.com", version, kind)

	artifactoryKey := CustomResourceClientKey(resource, artifactory)
	sonarqubeKey := CustomResourceClientKey(resource, sonarqube)

	assert.NotEqual(t, artifactoryKey, sonarqubeKey,
		"same-plural CRDs from different groups must not share a client key")
	assert.Contains(t, artifactoryKey, "artifactory.example.com")
	assert.Contains(t, sonarqubeKey, "sonarqube.example.com")
}

// TestGetCustomResourceClientDoesNotCollide verifies that when two same-plural
// custom resources from different groups are registered, each store resolves
// its own client rather than sharing one (the root cause of the "Unexpected
// watch event object gvk" errors in CONS-8512).
func TestGetCustomResourceClientDoesNotCollide(t *testing.T) {
	const (
		resource = "projects"
		version  = "v1alpha1"
		kind     = "Project"
	)

	artifactory := unstructuredForGroup("artifactory.example.com", version, kind)
	sonarqube := unstructuredForGroup("sonarqube.example.com", version, kind)

	// Sentinel clients so we can assert identity without a real API client.
	artifactoryClient := "artifactory-client"
	sonarqubeClient := "sonarqube-client"

	b := New()
	b.customResourceClients = map[string]interface{}{
		CustomResourceClientKey(resource, artifactory): artifactoryClient,
		CustomResourceClientKey(resource, sonarqube):   sonarqubeClient,
	}

	// Both entries survive population (no overwrite) ...
	assert.Len(t, b.customResourceClients, 2)

	// ... and each store's lookup, driven by its own ExpectedType, resolves the
	// matching client instead of whichever one was inserted last.
	assert.Equal(t, artifactoryClient, b.getCustomResourceClient(resource, artifactory))
	assert.Equal(t, sonarqubeClient, b.getCustomResourceClient(resource, sonarqube))
}
