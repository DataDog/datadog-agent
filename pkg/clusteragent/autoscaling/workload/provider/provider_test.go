// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestIsArgoRolloutsAvailable(t *testing.T) {
	t.Run("returns true when rollouts resource exists", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*v1.APIResourceList{
			{
				GroupVersion: "argoproj.io/v1alpha1",
				APIResources: []v1.APIResource{
					{Kind: "Rollout", Name: "rollouts"},
					{Kind: "AnalysisTemplate", Name: "analysistemplates"},
				},
			},
		}

		assert.True(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when group does not exist", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*v1.APIResourceList{
			{
				GroupVersion: "apps/v1",
				APIResources: []v1.APIResource{
					{Kind: "Deployment", Name: "deployments"},
				},
			},
		}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when group exists but rollouts resource is missing", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*v1.APIResourceList{
			{
				GroupVersion: "argoproj.io/v1alpha1",
				APIResources: []v1.APIResource{
					{Kind: "AnalysisTemplate", Name: "analysistemplates"},
				},
			},
		}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when no resources at all", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*v1.APIResourceList{}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})
}
