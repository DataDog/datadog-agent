// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {
	genConfig := func(cluster string) []byte {
		base := `
{
	"id": "17945471932432318983",
	"revision": 1673513604823158800,
	"schema_version": "v1.0.0",
	"action": "enable",
	"lib_config": {
		"env": "staging"
	},
	"k8s_target_v2": {
		"cluster_targets": [
			{
				"cluster_name": "%s",
				"enabled": true,
				"enabled_namespaces": ["ns1"]
			}
		]
	}
}
`
		return []byte(fmt.Sprintf(base, cluster))
	}
	rcp, err := newRemoteConfigProvider(&rcclient.Client{}, make(chan struct{}), telemetry.NewNoopCollector(), "dev")
	require.NoError(t, err)
	notifs := rcp.subscribe(KindCluster)
	in := map[string]state.RawConfig{
		"path1": {Config: genConfig("dev")}, // valid config
		//"path2": {Config: []byte("invalid")},  // invalid json
		//"path3": {Config: genConfig("dev")},   // kind mismatch
		//"path4": {Config: genConfig("wrong")}, // cluster mismatch
	}
	rcp.process(in, nil)
	require.Len(t, notifs, 1)
	pr := <-notifs
	require.Equal(t, "17945471932432318983", pr.ID)
	require.Equal(t, int64(1673513604823158800), pr.Revision)
	require.Equal(t, "v1.0.0", pr.SchemaVersion)
	require.Equal(t, "staging", *pr.LibConfig.Env)
	require.Equal(t, 1, len(pr.K8sTarget.ClusterTargets))
	require.Equal(t, "dev", pr.K8sTarget.ClusterTargets[0].ClusterName)
	require.Equal(t, true, *pr.K8sTarget.ClusterTargets[0].Enabled)
	require.Equal(t, &([]string{"ns1"}), pr.K8sTarget.ClusterTargets[0].EnabledNamespaces)
	require.Len(t, notifs, 0)
}
