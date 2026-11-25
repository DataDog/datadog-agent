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

// StubNodeClusterNameLabelForTest replaces the cluster name lookup from node labels with the stub until the test ends.
func StubNodeClusterNameLabelForTest(t testing.TB, stub func(context.Context, string) (string, error)) {
	previous := nodeClusterNameLabel
	t.Cleanup(func() { nodeClusterNameLabel = previous })
	nodeClusterNameLabel = stub
}

// DisableNodeLabelLookupForTest stops the cluster name lookup from reading node labels until the test ends. Without
// it, the lookup waits for the API server that the host kubeconfig selects, which makes tests depend on the host.
func DisableNodeLabelLookupForTest(t testing.TB) {
	StubNodeClusterNameLabelForTest(t, func(_ context.Context, clusterName string) (string, error) {
		return clusterName, nil
	})
}
