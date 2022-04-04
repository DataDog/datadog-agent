// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package orchestrator

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
)

func TestWalkAPIResources(t *testing.T) {
	inventory := inventory.NewCollectorInventory()
	provider := NewAPIServerDiscoveryProvider()

	preferredResources := []*v1.APIResourceList{
		{
			GroupVersion: "batch/v1",
			APIResources: []v1.APIResource{
				{
					Name: "cronjobs",
				},
			},
		},
	}
	allResources := []*v1.APIResourceList{
		{
			GroupVersion: "batch/v1",
			APIResources: []v1.APIResource{
				{
					Name: "cronjobs",
				},
			},
		},
		{
			GroupVersion: "batch/v1beta1",
			APIResources: []v1.APIResource{
				{
					Name: "cronjobs",
				},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{
					Name: "deployments",
				},
			},
		},
	}

	provider.walkAPIResources(inventory, preferredResources)
	assert.EqualValues(t, map[string]struct{}{"cronjobs": {}}, provider.seen)

	provider.walkAPIResources(inventory, allResources)
	assert.EqualValues(t, map[string]struct{}{"cronjobs": {}, "deployments": {}}, provider.seen)

	require.Len(t, provider.result, 2)
	assert.True(t, provider.result[0].Metadata().FullName() == "batch/v1/cronjobs")
	assert.True(t, provider.result[1].Metadata().FullName() == "apps/v1/deployments")
}
