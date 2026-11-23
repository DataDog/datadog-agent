// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package clustername

import (
	"context"
	"testing"
)

// DisableNodeLabelLookupForTest stops the cluster name lookup from reading node labels until the test ends. Without
// it, the lookup waits for the API server that the host kubeconfig selects, which makes tests depend on the host.
func DisableNodeLabelLookupForTest(t testing.TB) {
	stubNodeClusterNameLabel(t, func(_ context.Context, clusterName string) (string, error) {
		return clusterName, nil
	})
}

// stubNodeClusterNameLabel replaces the node-label discovery with stub for the
// duration of the test.
func stubNodeClusterNameLabel(t testing.TB, stub func(context.Context, string) (string, error)) {
	t.Helper()
	previous := nodeClusterNameLabel
	t.Cleanup(func() { nodeClusterNameLabel = previous })
	nodeClusterNameLabel = stub
}
