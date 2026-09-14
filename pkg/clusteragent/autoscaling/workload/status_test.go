// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// resetStatus clears the package-level status state between subtests.
func resetStatus(t *testing.T) {
	t.Helper()
	statusStore.Lock()
	statusStore.store, statusStore.isLeader, statusStore.rcInstance = nil, nil, ""
	statusStore.Unlock()
	rcTracker.Lock()
	rcTracker.byProduct = map[string]*productStatus{}
	rcTracker.Unlock()
}

func renderText(t *testing.T) string {
	t.Helper()
	b := new(bytes.Buffer)
	require.NoError(t, Provider{}.Text(false, b))
	return b.String()
}

func TestWorkloadAutoscalingStatusDisabled(t *testing.T) {
	resetStatus(t)
	cfg := configmock.New(t)
	cfg.SetInTest("autoscaling.workload.enabled", false)

	out := renderText(t)
	assert.Contains(t, out, "not enabled on the Cluster Agent")
	assert.NotContains(t, out, "DatadogPodAutoscalers:")
}

func TestWorkloadAutoscalingStatusEnabledNotStarted(t *testing.T) {
	resetStatus(t)
	cfg := configmock.New(t)
	cfg.SetInTest("autoscaling.workload.enabled", true)

	out := renderText(t)
	assert.Contains(t, out, "has not started yet")
}

func TestWorkloadAutoscalingStatusNoUpdatesYet(t *testing.T) {
	resetStatus(t)
	cfg := configmock.New(t)
	cfg.SetInTest("autoscaling.workload.enabled", true)

	store := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	InitStatus(store, func() bool { return true }, "cluster-agent:autoscaling")

	out := renderText(t)
	assert.Contains(t, out, "DatadogPodAutoscalers: 0")
	assert.Contains(t, out, "Leader: true")
	assert.Contains(t, out, "Remote Config connection: No update received yet")
	assert.Contains(t, out, "Remote Config client: cluster-agent:autoscaling")
	// Both products must be listed even before anything arrives.
	assert.Contains(t, out, data.ProductContainerAutoscalingSettings)
	assert.Contains(t, out, data.ProductContainerAutoscalingValues)
}

func TestWorkloadAutoscalingStatusWithUpdates(t *testing.T) {
	resetStatus(t)
	cfg := configmock.New(t)
	cfg.SetInTest("autoscaling.workload.enabled", true)

	store := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	for _, id := range []string{"ns/one", "ns/two"} {
		item, _ := store.Get(id)
		item.Upsert(model.PodAutoscalerInternal{}, "test")
		item.Release()
	}
	InitStatus(store, func() bool { return false }, "default")

	now := time.Now().Add(-90 * time.Second)
	// The highest version across the update is what gets reported.
	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, now, map[string]state.RawConfig{
		"cfg-a": {Metadata: state.Metadata{Version: 7}},
		"cfg-b": {Metadata: state.Metadata{Version: 12}},
	})
	recordRemoteConfigError(data.ProductContainerAutoscalingSettings, now, errors.New("bad spec"))

	out := renderText(t)
	assert.Contains(t, out, "DatadogPodAutoscalers: 2")
	assert.Contains(t, out, "Leader: false")
	assert.Contains(t, out, "Remote Config connection: Receiving updates")
	assert.Contains(t, out, "Last config version: 12")
	assert.Contains(t, out, "Configs in last update: 2")
	assert.Contains(t, out, "Updates received: 1")
	assert.Contains(t, out, "Last error: bad spec")
	// Values never delivered anything, so it must still read as pending.
	assert.Contains(t, out, "No update received yet")

	// JSON output must carry the same numbers for programmatic consumers.
	stats := map[string]interface{}{}
	require.NoError(t, Provider{}.JSON(false, stats))
	info := stats["workloadAutoscaling"].(map[string]interface{})
	assert.Equal(t, 2, info["PodAutoscalerCount"])
	assert.Equal(t, true, info["RemoteConfigConnected"])

	// HTML must render without error too.
	h := new(bytes.Buffer)
	require.NoError(t, Provider{}.HTML(false, h))
	assert.Contains(t, h.String(), "DatadogPodAutoscalers: 2")
}
