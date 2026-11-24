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

// TestFailures checks that a failure of one collector does not stop the others.
func TestFailures(t *testing.T) {
	// The check skips a built-in custom resource that its RBAC permissions do not let it list.
	t.Run("forbidden_custom_resource", func(t *testing.T) {
		env := suite.Setup(t, suite.WithForbiddenResources("argoproj.io/v1alpha1/rollouts"))
		expected := firstRun(except(env.UIDs(), "argoproj.io_v1alpha1_rollouts"))

		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})

	// The check skips a collector whose informer does not sync within kube_cache_sync_timeout_seconds and
	// extra_sync_timeout_seconds.
	t.Run("informer_that_never_syncs", func(t *testing.T) {
		env := suite.Setup(t,
			suite.WithFailingLists("apps/v1/deployments"),
			suite.WithSetting("kube_cache_sync_timeout_seconds", 1),
			suite.WithExtraSyncTimeout(1))
		expected := firstRun(except(env.UIDs(), "apps_v1_deployments"))

		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})

	// A custom resource collector fails its runs when the resource has more objects than custom_resources.max_count.
	t.Run("too_many_custom_resources", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("orchestrator_explorer.custom_resources.max_count", 1))
		env.Add(t, suite.Named(t, env.Object(t, "argoproj.io_v1alpha1_rollouts"), "second-rollout"))
		expected := firstRun(except(env.UIDs(), "argoproj.io_v1alpha1_rollouts", "second-rollout"))

		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})

	// The cluster collector reads the creation time of the kube-system namespace, and fails without it.
	t.Run("cluster_without_kube_system", func(t *testing.T) {
		env := suite.Setup(t)
		env.DeleteUIDs(t, "v1_namespaces")
		expected := suite.Live(env.UIDs()...)

		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})
}
