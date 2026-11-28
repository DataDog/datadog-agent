// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && kubeapiserver

package selfident

import "testing"

// SetClusterIDLookup makes the SelfIdents resolve the Kubernetes cluster ID with lookup, on every Agent flavor, until
// the end of the test: for instance a slow one, standing in for an unreachable Cluster Agent. It changes package
// state without synchronization, so it can't be used from a parallel test, nor while a lookup started by an earlier
// SelfIdent may still be running.
func SetClusterIDLookup(t testing.TB, lookup func() (string, error)) {
	t.Setenv("DD_TEST_SELFIDENT_CLUSTER_ID_LOOKUP", "set") // panics in a parallel test
	previousNodeAgent, previousClusterAgent := nodeAgentClusterIDFunc, clusterAgentClusterIDFunc
	nodeAgentClusterIDFunc, clusterAgentClusterIDFunc = lookup, lookup
	t.Cleanup(func() {
		nodeAgentClusterIDFunc, clusterAgentClusterIDFunc = previousNodeAgent, previousClusterAgent
	})
}
