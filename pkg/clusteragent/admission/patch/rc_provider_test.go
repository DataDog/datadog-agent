// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {
	genConfig := func(cluster, kind string) []byte {
		base := `
{
	"id": "17945471932432318983",
	"revision": 1673513604823158800,
	"schema_version": "v1.0.0",
	"action": "enable",
	"lib_config": {
		"library_language": "java",
		"library_version": "latest"
	},
	"k8s_target": {
		"cluster": "%s",
		"kind": "%s",
		"name": "my-java-app",
		"namespace": "default"
	}
}
`
		return []byte(fmt.Sprintf(base, cluster, kind))
	}
	rcp, err := newRemoteConfigProvider(&remote.Client{}, make(chan struct{}), "dev")
	require.NoError(t, err)
	notifs := rcp.subscribe(KindDeployment)
	in := map[string]state.APMTracingConfig{
		"path1": {Config: genConfig("dev", "deployment")},   // valid config
		"path2": {Config: []byte("invalid")},                // invalid json
		"path3": {Config: genConfig("dev", "wrong")},        // kind mismatch
		"path4": {Config: genConfig("wrong", "deployment")}, // cluster mismatch
	}
	rcp.process(in)
	require.Len(t, notifs, 1)
	pr := <-notifs
	require.Equal(t, "17945471932432318983", pr.ID)
	require.Equal(t, int64(1673513604823158800), pr.Revision)
	require.Equal(t, "v1.0.0", pr.SchemaVersion)
	require.Equal(t, "java", pr.LibConfig.Language)
	require.Equal(t, "latest", pr.LibConfig.Version)
	require.Equal(t, "dev", pr.K8sTarget.Cluster)
	require.Equal(t, KindDeployment, pr.K8sTarget.Kind)
	require.Equal(t, "my-java-app", pr.K8sTarget.Name)
	require.Equal(t, "default", pr.K8sTarget.Namespace)
	require.Len(t, notifs, 0)
}
