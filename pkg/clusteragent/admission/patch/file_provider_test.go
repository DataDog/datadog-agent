// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileProviderProcess(t *testing.T) {
	fpp := newfileProvider("testdata/auto-instru.json", make(chan struct{}), "dev")
	notifs := fpp.subscribe(KindCluster)
	fpp.process(false)
	require.Len(t, notifs, 1)
	pr := <-notifs
	require.Equal(t, "11777398274940883091", pr.ID)
	require.Equal(t, int64(1674236639474287600), pr.Revision)
	require.Equal(t, "v1.0.0", pr.SchemaVersion)
	require.Equal(t, "dev", *pr.LibConfig.Env)
	require.Equal(t, 1, len(pr.K8sTarget.ClusterTargets))
	require.Equal(t, "dev", pr.K8sTarget.ClusterTargets[0].ClusterName)
	require.Equal(t, true, *pr.K8sTarget.ClusterTargets[0].Enabled)
	require.Equal(t, 1, len(*pr.K8sTarget.ClusterTargets[0].EnabledNamespaces))
	require.Equal(t, "ns1", (*pr.K8sTarget.ClusterTargets[0].EnabledNamespaces)[0])
	require.Len(t, notifs, 0)
}
