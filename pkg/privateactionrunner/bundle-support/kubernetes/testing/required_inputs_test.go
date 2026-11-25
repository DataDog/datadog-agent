// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	testhelpers "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/test"
	apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	discovery "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/discovery"
	networking "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/networking"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func generatedActionTask(inputs map[string]interface{}) *types.Task {
	return testhelpers.NewTestTask("test", "test", &types.Attributes{Inputs: inputs})
}

func TestGeneratedActionsRejectMissingInputs(t *testing.T) {
	tests := []struct {
		name      string
		bundle    types.Bundle
		resources []generatedActionResource
	}{
		{
			name:   "apps",
			bundle: apps.NewKubernetesApps(),
			resources: []generatedActionResource{
				{name: "ControllerRevision", namespaced: true},
				{name: "DaemonSet", namespaced: true},
				{name: "Deployment", namespaced: true},
				{name: "ReplicaSet", namespaced: true},
				{name: "StatefulSet", namespaced: true},
			},
		},
		{
			name:   "batch",
			bundle: batch.NewKubernetesBatch(),
			resources: []generatedActionResource{
				{name: "CronJob", namespaced: true},
				{name: "Job", namespaced: true},
			},
		},
		{
			name:   "core",
			bundle: core.NewKubernetesCore(),
			resources: []generatedActionResource{
				{name: "ConfigMap", namespaced: true},
				{name: "Endpoints", namespaced: true},
				{name: "Event", namespaced: true},
				{name: "LimitRange", namespaced: true},
				{name: "Namespace"},
				{name: "Node"},
				{name: "PersistentVolume"},
				{name: "PersistentVolumeClaim", namespaced: true},
				{name: "Pod", namespaced: true},
				{name: "PodTemplate", namespaced: true},
				{name: "ReplicationController", namespaced: true},
				{name: "ResourceQuota", namespaced: true},
				{name: "Service", namespaced: true},
				{name: "ServiceAccount", namespaced: true},
			},
		},
		{
			name:      "discovery",
			bundle:    discovery.NewKubernetesDiscovery(),
			resources: []generatedActionResource{{name: "EndpointSlice", namespaced: true}},
		},
		{
			name:   "networking",
			bundle: networking.NewKubernetesNetworking(),
			resources: []generatedActionResource{
				{name: "IPAddress"},
				{name: "Ingress", namespaced: true},
				{name: "IngressClass"},
				{name: "NetworkPolicy", namespaced: true},
				{name: "ServiceCIDR"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGeneratedActionsRejectMissingInputs(t, tt.bundle, tt.resources)
		})
	}
}

type generatedActionResource struct {
	name       string
	namespaced bool
}

func assertGeneratedActionsRejectMissingInputs(t *testing.T, bundle types.Bundle, resources []generatedActionResource) {
	for _, resource := range resources {
		for _, verb := range []string{"get", "delete", "patch"} {
			for _, inputs := range []map[string]interface{}{nil, {}, {"namespace": "test"}, {"name": ""}, {"dryRun": "All"}} {
				t.Run(verb+resource.name, func(t *testing.T) {
					action := bundle.GetAction(verb + resource.name)
					require.NotNil(t, action, "action must be registered")
					_, err := action.Run(context.Background(), generatedActionTask(inputs), nil)
					require.EqualError(t, err, "name is required")
				})
			}
		}
		for _, verb := range []string{"create", "update", "patch"} {
			t.Run(verb+resource.name+"/missing body", func(t *testing.T) {
				action := bundle.GetAction(verb + resource.name)
				require.NotNil(t, action, "action must be registered")
				_, err := action.Run(context.Background(), generatedActionTask(map[string]interface{}{"namespace": "test", "name": "test"}), nil)
				require.EqualError(t, err, "body is required")
			})
		}
		if resource.namespaced {
			for _, verb := range []string{"create", "get", "update", "delete", "patch"} {
				t.Run(verb+resource.name+"/missing namespace", func(t *testing.T) {
					action := bundle.GetAction(verb + resource.name)
					require.NotNil(t, action, "action must be registered")
					_, err := action.Run(context.Background(), generatedActionTask(map[string]interface{}{"name": "test"}), nil)
					require.EqualError(t, err, "namespace is required")
				})
			}
		}
	}
}
