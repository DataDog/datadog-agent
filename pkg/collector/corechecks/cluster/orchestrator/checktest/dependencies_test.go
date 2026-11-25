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

// TestCollectorDependencies checks the collectors that depend on another one, or share its informer. The cluster has
// an object of every resource, so the objects sent show the collectors that run.
func TestCollectorDependencies(t *testing.T) {
	// The nodes and cluster collectors share the node informer, which the cluster collector reads to build the
	// cluster.
	nodeCases := []struct {
		name       string
		collectors []string
		first      []string
		deleted    []string
	}{
		{name: "nodes_only", collectors: []string{"nodes"}, first: []string{"live v1_nodes"}, deleted: []string{"deleted v1_nodes"}},
		{name: "clusters_only", collectors: []string{"clusters"}, first: []string{"live cluster"}, deleted: []string{"live cluster"}},
		{name: "nodes_then_clusters", collectors: []string{"nodes", "clusters"}, first: []string{"live cluster", "live v1_nodes"}, deleted: []string{"deleted v1_nodes", "live cluster"}},
		// Known defect: an informer only calls the delete handler of the collector that starts it. When the cluster
		// collector starts the node informer, deleted nodes are not reported.
		{name: "clusters_then_nodes", collectors: []string{"clusters", "nodes"}, first: []string{"live cluster", "live v1_nodes"}, deleted: []string{"live cluster"}},
	}
	for _, c := range nodeCases {
		t.Run(c.name, func(t *testing.T) {
			env := suite.Setup(t, suite.WithCollectors(c.collectors...))

			suite.AssertObjects(t, c.first, env.Pop(t, c.first...))
			env.DeleteUIDs(t, "v1_nodes")
			suite.AssertObjects(t, c.deleted, env.Pop(t, c.deleted...))
		})
	}

	// The check adds the terminated pods collector to the pods collector, which ignores deletions.
	t.Run("pods_without_terminated_pods", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("pods"))

		suite.AssertObjects(t, []string{"live v1_pods"}, env.Pop(t, "live v1_pods"))
		env.DeleteUIDs(t, "v1_pods")
		suite.AssertObjects(t, []string{"deleted v1_pods"}, env.Pop(t, "deleted v1_pods"))
	})

	t.Run("terminated_pods_without_pods", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("terminated-pods"))

		suite.AssertObjects(t, nil, env.Pop(t))
		env.DeleteUIDs(t, "v1_pods")
		suite.AssertObjects(t, []string{"deleted v1_pods"}, env.Pop(t, "deleted v1_pods"))
	})

	// The check adds the Helm release collector to the ConfigMap collector only, and collects the built-in custom
	// resources with the CRD collector only.
	t.Run("collectors_without_their_dependency", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("deployments"))

		suite.AssertObjects(t, []string{"live apps_v1_deployments"}, env.Pop(t, "live apps_v1_deployments"))
	})
}
