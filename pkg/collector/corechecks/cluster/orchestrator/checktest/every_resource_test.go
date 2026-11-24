// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// TestEveryResource runs an object of every resource through its lifecycle in each configuration of the check, which
// sends every object except the ones that the configuration lists. Some configurations also compare testdata/payloads.
func TestEveryResource(t *testing.T) {
	configurations := []struct {
		name    string
		options []suite.Option
		// checkContent compares the payload content with the payload files. The other configurations only change which
		// objects the check selects.
		checkContent bool
		// notCollected are the objects of the cluster that the check does not collect.
		notCollected []string
	}{
		{name: "leader", checkContent: true},
		{name: "cluster_check_runner", options: []suite.Option{suite.WithMode(suite.ClusterCheckRunner)}, checkContent: true},
		// Each payload holds one resource, so the check splits its results in as many payloads, and fills and flushes
		// the manifest buffer at each one.
		{name: "one_resource_per_message", options: []suite.Option{suite.WithSetting("orchestrator_explorer.max_per_message", 1)}, checkContent: true},
		{
			// Without a collector selection, the check collects the stable resources that discovery reports.
			name:    "default_collectors",
			options: []suite.Option{suite.WithInstance("")},
			// ConfigMaps and Secrets have unstable collectors, which a collector selection must name. Helm releases are
			// collected with the ConfigMap collector only.
			notCollected: []string{"v1_configmaps", "v1_secrets", "v1_helm-releases-configmaps"},
		},
		{
			// Without collector discovery, the check selects the stable collectors of its inventory, in their default
			// versions.
			name:    "collector_discovery_disabled",
			options: []suite.Option{suite.WithInstance(""), suite.WithSetting("orchestrator_explorer.collector_discovery.enabled", false)},
			// Unlike with discovery, the inventory selects the Helm release collector without the ConfigMap collector,
			// as the setting of the suite makes it stable.
			notCollected: []string{"v1_configmaps", "v1_secrets"},
		},
		{
			// The cluster serves the older version of the resources that have several.
			name:    "older_versions",
			options: []suite.Option{suite.WithInstance(""), suite.WithOlderVersions()},
			notCollected: []string{
				"v1_configmaps", "v1_secrets", "v1_helm-releases-configmaps",
				// The check selects one version for each API group. ray.io serves raycronjobs in v1 only, so the check
				// selects v1, which these resources do not serve in this cluster.
				"ray.io_v1alpha1_rayclusters", "ray.io_v1alpha1_rayjobs", "ray.io_v1alpha1_rayservices",
			},
		},
	}

	for _, c := range configurations {
		t.Run(c.name, func(t *testing.T) {
			env := suite.Setup(t, c.options...)
			collected := except(env.UIDs(), c.notCollected...)

			// The check sends the cluster when the nodes change.
			expected := append(suite.Live(collected...), "live cluster")
			actual := env.Pop(t, expected...)
			suite.AssertObjects(t, expected, actual)
			if c.checkContent {
				suite.AssertPayloadFiles(t, actual, contentObjects(t)...)
			}

			suite.AssertObjects(t, nil, env.Pop(t))

			env.UpdateAll(t)
			expected = suite.Live(collected...)
			actual = env.Pop(t, expected...)
			suite.AssertObjects(t, expected, actual)
			suite.AssertUpdated(t, actual)

			env.DeleteAll(t)
			expected = append(deletion(collected), "live cluster")
			actual = env.Pop(t, expected...)
			suite.AssertObjects(t, expected, actual)
			suite.AssertDeleted(t, actual)
			if c.checkContent {
				// A deleted pod differs from a live one beyond its deletion timestamp: its status and its version.
				suite.AssertPayloadFiles(t, actual, "deleted v1_pods")
			}
		})
	}
}
