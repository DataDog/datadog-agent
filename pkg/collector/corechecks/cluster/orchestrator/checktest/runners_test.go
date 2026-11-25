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

// TestClusterCheckRunners checks several instances of the check, each with its own collectors, like the instances
// that the Cluster Agent dispatches to cluster check runners. A runner can run several instances in one process.
func TestClusterCheckRunners(t *testing.T) {
	// One instance collects the built-in resources, and the other the custom resources.
	t.Run("instances_with_other_collectors", func(t *testing.T) {
		builtins := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner), suite.WithoutCollectors("customresourcedefinitions"))
		customResources := builtins.AddCheck(t, suite.WithCRDCollectorsOnly())
		builtinUIDs := except(builtins.UIDs(), append(builtins.CustomResourceUIDs(), "apiextensions.k8s.io_v1_customresourcedefinitions")...)
		customResourceUIDs := builtins.CustomResourceUIDs()

		suite.AssertObjects(t, firstRun(builtinUIDs), builtins.Pop(t, firstRun(builtinUIDs)...))
		suite.AssertObjects(t, suite.Live(customResourceUIDs...), customResources.Pop(t, suite.Live(customResourceUIDs...)...))
		builtins.DeleteAll(t)
		expected := append(deletion(builtinUIDs), "live cluster")
		suite.AssertObjects(t, expected, builtins.Pop(t, expected...))
		suite.AssertObjects(t, deletion(customResourceUIDs), customResources.Pop(t, deletion(customResourceUIDs)...))
	})

	// The instances of a process share the cache of the resource versions sent: the instance that runs first sends an
	// object, and the other skips it until its cache entry expires.
	t.Run("instances_with_the_same_collectors", func(t *testing.T) {
		first := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner))
		second := first.AddCheck(t, suite.WithInstanceTags("instance:second"))
		uids := first.UIDs()

		suite.AssertObjects(t, firstRun(uids), first.Pop(t, firstRun(uids)...))
		suite.AssertObjects(t, nil, second.Pop(t))
		first.ExpireResourceCache()
		suite.AssertObjects(t, firstRun(uids), second.Pop(t, firstRun(uids)...))
		suite.AssertObjects(t, nil, first.Pop(t))
	})
}
