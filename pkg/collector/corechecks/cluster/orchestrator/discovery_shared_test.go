// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSharedDiscovery checks that both bundles share one API discovery per check run, and that each bundle sees the
// resources that a refresh of the other bundle added.
func TestSharedDiscovery(t *testing.T) {
	fetches := 0
	resources := []*metav1.APIResourceList{{GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "pods"}}}}
	shared := newSharedDiscovery(func() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
		fetches++
		return nil, resources, nil
	})
	metadata := &CollectorBundle{collectorDiscovery: shared.collector, sharedDiscovery: shared}
	manifests := &CollectorBundle{collectorDiscovery: shared.collector, sharedDiscovery: shared}

	updated, current, err := metadata.refreshDiscovery()
	require.NoError(t, err)
	assert.True(t, updated)
	assert.True(t, current)
	updated, _, _ = manifests.refreshDiscovery()
	assert.True(t, updated, "the refresh of the other bundle serves new resources")
	assert.Equal(t, 1, fetches, "the bundles of a check run share one refresh")

	// The metadata bundle refreshes alone after a CRD change. The manifest bundle still sees the new resources.
	shared.nextRun()
	resources = append(resources, &metav1.APIResourceList{GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{{Name: "widgets"}}})
	updated, _, _ = metadata.refreshDiscovery()
	assert.True(t, updated)
	shared.nextRun()
	updated, _, _ = manifests.refreshDiscovery()
	assert.True(t, updated, "a later refresh serves the resources that the other bundle already added")
	assert.Equal(t, 3, fetches)

	// A CRD change after the refresh of the check run is not included in it.
	manifests.flagCRDChange()
	_, current, _ = metadata.refreshDiscovery()
	assert.False(t, current)
	assert.Equal(t, 3, fetches)
}
