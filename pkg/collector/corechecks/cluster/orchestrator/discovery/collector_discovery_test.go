// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package discovery

import (
	"testing"

	"go.uber.org/fx"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestWalkAPIResources(t *testing.T) {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	inventory := inventory.NewCollectorInventory(cfg, mockStore, fakeTagger)
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
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{
					Name: "deployments",
				},
			},
		},
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
	}

	provider.walkAPIResources(inventory, preferredResources)
	assert.EqualValues(t, map[string]struct{}{"cronjobs": {}}, provider.seen)

	provider.walkAPIResources(inventory, allResources)
	assert.EqualValues(t, map[string]struct{}{"cronjobs": {}, "deployments": {}}, provider.seen)

	require.Len(t, provider.result, 2)
	assert.True(t, provider.result[0].Metadata().FullName() == "batch/v1/cronjobs")
	assert.True(t, provider.result[1].Metadata().FullName() == "apps/v1/deployments")
}

func TestIdentifyResources(t *testing.T) {
	groups := []*v1.APIGroup{
		{
			Name: "apps",
			Versions: []v1.GroupVersionForDiscovery{
				{
					GroupVersion: "apps/v1",
				},
			},
			PreferredVersion: v1.GroupVersionForDiscovery{
				GroupVersion: "apps/v1",
				Version:      "v1",
			},
		},
		{
			Name: "batch",
			Versions: []v1.GroupVersionForDiscovery{
				{
					GroupVersion: "batch/v1",
					Version:      "v1",
				},
				{
					GroupVersion: "batch/v1beta1",
					Version:      "v1beta1",
				},
			},
			PreferredVersion: v1.GroupVersionForDiscovery{
				GroupVersion: "batch/v1",
				Version:      "v1",
			},
		},
	}

	resources := []*v1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{
					Name: "deployments",
				},
			},
		},
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
	}

	expectedPreferredResources := []*v1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []v1.APIResource{
				{
					Name: "deployments",
				},
			},
		},
		{
			GroupVersion: "batch/v1",
			APIResources: []v1.APIResource{
				{
					Name: "cronjobs",
				},
			},
		},
	}

	expectedOtherResources := []*v1.APIResourceList{
		{
			GroupVersion: "batch/v1beta1",
			APIResources: []v1.APIResource{
				{
					Name: "cronjobs",
				},
			},
		},
	}

	actualPreferredResources, actualOtherResources := identifyResources(groups, resources)

	assert.EqualValues(t, expectedPreferredResources, actualPreferredResources)
	assert.EqualValues(t, expectedOtherResources, actualOtherResources)
}
