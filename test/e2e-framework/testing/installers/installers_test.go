// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agentStateJSON(t *testing.T, agentVersion, clusterAgentVersion, namespace string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"linuxNodeAgent":    map[string]string{"namespace": namespace, "installVersion": agentVersion},
		"linuxClusterAgent": map[string]string{"namespace": namespace, "installVersion": clusterAgentVersion},
	})
	require.NoError(t, err)
	return raw
}

func TestHelmKubernetesInstallerStatus(t *testing.T) {
	desired := InstallParams{
		AgentVersion:        "7.81",
		ClusterAgentVersion: "latest",
		Namespace:           "datadog",
	}

	cases := []struct {
		name         string
		raw          json.RawMessage
		wantUpToDate bool
	}{
		{"matching config", agentStateJSON(t, "7.81", "latest", "datadog"), true},
		{"agentVersion drifted", agentStateJSON(t, "latest", "latest", "datadog"), false},
		{"clusterAgentVersion drifted", agentStateJSON(t, "7.81", "7.65.0", "datadog"), false},
		{"namespace drifted", agentStateJSON(t, "7.81", "latest", "other-namespace"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := helmKubernetesInstaller{}.status(tc.raw, desired)
			require.NoError(t, err)
			assert.Equal(t, tc.wantUpToDate, got.UpToDate)
			assert.NotEmpty(t, got.Summary)
		})
	}
}

func TestHelmKubernetesInstallerStatusUnparseable(t *testing.T) {
	_, err := helmKubernetesInstaller{}.status(json.RawMessage(`not json`), InstallParams{})
	assert.Error(t, err)
}

func TestStatusNotInstalled(t *testing.T) {
	got, err := Status("helm-k8s", map[string]json.RawMessage{}, InstallParams{})
	require.NoError(t, err)
	assert.Equal(t, InstallStatus{Summary: "not installed"}, got)
}

func TestStatusDelegatesToResolvedInstaller(t *testing.T) {
	entries := map[string]json.RawMessage{
		"kubernetesCluster": json.RawMessage(`{}`),
		"agent":             agentStateJSON(t, "7.81", "latest", "datadog"),
	}
	got, err := Status("", entries, InstallParams{
		AgentVersion:        "7.81",
		ClusterAgentVersion: "latest",
		Namespace:           "datadog",
	})
	require.NoError(t, err)
	assert.True(t, got.UpToDate)
}

func TestStatusUnknownInstaller(t *testing.T) {
	entries := map[string]json.RawMessage{"agent": json.RawMessage(`{}`)}
	_, err := Status("bogus", entries, InstallParams{})
	assert.ErrorContains(t, err, "unknown installer")
}

func TestMergeMaps(t *testing.T) {
	dst := map[string]interface{}{
		"agents": map[string]interface{}{
			"useHostNetwork": true,
			"image":          map[string]interface{}{"tag": "latest"},
		},
		"datadog": map[string]interface{}{"apiKey": "abc"},
	}
	src := map[string]interface{}{
		"agents": map[string]interface{}{
			"useHostNetwork": false, // overrides a scalar nested one level down
		},
		"newTopLevel": "value", // adds a brand-new top-level key
	}

	mergeMaps(dst, src)

	assert.Equal(t, false, dst["agents"].(map[string]interface{})["useHostNetwork"])
	assert.Equal(t, "latest", dst["agents"].(map[string]interface{})["image"].(map[string]interface{})["tag"], "untouched nested key survives the merge")
	assert.Equal(t, "abc", dst["datadog"].(map[string]interface{})["apiKey"], "untouched top-level key survives the merge")
	assert.Equal(t, "value", dst["newTopLevel"])
}

func TestMergeMapsOverridesMapWithNonMap(t *testing.T) {
	dst := map[string]interface{}{"agents": map[string]interface{}{"image": map[string]interface{}{"tag": "latest"}}}
	src := map[string]interface{}{"agents": "disabled"} // src's scalar replaces dst's nested map outright

	mergeMaps(dst, src)

	assert.Equal(t, "disabled", dst["agents"])
}
