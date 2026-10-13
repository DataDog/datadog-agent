// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/clock"

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
	statusStore.store, statusStore.isLeader = nil, nil
	statusStore.settings, statusStore.values = productStatus{}, productStatus{}
	statusStore.Unlock()
}

func renderText(t *testing.T) string {
	t.Helper()
	b := new(bytes.Buffer)
	require.NoError(t, Provider{}.Text(false, b))
	return b.String()
}

// productBlock returns the rendered lines belonging to one product, so a test can
// assert an update landed under the right heading rather than merely somewhere.
//
// Anchored on the rendered heading rather than the bare product name: a real
// processing error embeds the Remote Configuration config key, which itself
// contains the product name, so a bare match can land inside the other product's
// error line.
func productBlock(t *testing.T, out, product string) string {
	t.Helper()
	heading := "\n  " + product + "\n"
	start := strings.Index(out, heading)
	require.GreaterOrEqual(t, start, 0, "product %s not rendered in:\n%s", product, out)
	rest := out[start+len(heading):]
	if next := strings.Index(rest, "\n  CONTAINER_AUTOSCALING_"); next != -1 {
		return rest[:next]
	}
	return rest
}

func renderHTML(t *testing.T) string {
	t.Helper()
	b := new(bytes.Buffer)
	require.NoError(t, Provider{}.HTML(false, b))
	return b.String()
}

// enabledConfig sets autoscaling.workload.enabled
func enabledConfig(t *testing.T, enabled bool) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("autoscaling.workload.enabled", enabled)
}

func TestWorkloadAutoscalingStatusDisabled(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, false)

	// Registered, so only the setting keeps this out of the live branch
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	out := renderText(t)
	assert.Contains(t, out, "not enabled on the Cluster Agent")
	assert.NotContains(t, out, "DatadogPodAutoscalers:")
	assert.Contains(t, renderHTML(t), "not enabled on the Cluster Agent")
}

func TestWorkloadAutoscalingStatusEnabledNotStarted(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)

	out := renderText(t)
	assert.Contains(t, out, "has not started yet")
	assert.NotContains(t, out, "DatadogPodAutoscalers:")
	assert.Contains(t, renderHTML(t), "has not started yet")
}

func TestWorkloadAutoscalingStatusStarted(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)

	store := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	for _, id := range []string{"ns/one", "ns/two"} {
		item, _ := store.Get(id)
		item.Upsert(model.PodAutoscalerInternal{}, "test")
		item.Release()
	}
	InitStatus(store, func() bool { return true })

	out := renderText(t)
	assert.Contains(t, out, "DatadogPodAutoscalers: 2")
	assert.Contains(t, out, "Leader: true")
	// Both products are listed even before anything arrives
	assert.Contains(t, out, data.ProductContainerAutoscalingSettings)
	assert.Contains(t, out, data.ProductContainerAutoscalingValues)
	assert.Contains(t, out, "No update received yet")
	assert.Contains(t, renderHTML(t), "DatadogPodAutoscalers: 2")
}

func TestWorkloadAutoscalingStatusFollower(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return false })

	assert.Contains(t, renderText(t), "Leader: false")
}

func TestWorkloadAutoscalingStatusReceivingUpdates(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, map[string]state.RawConfig{
		"a": {Metadata: state.Metadata{Version: 7}},
		"b": {Metadata: state.Metadata{Version: 12}},
	})

	out := renderText(t)
	// The highest version across the update is what gets reported
	assert.Contains(t, out, "Last config version: 12")
	// A healthy product must not grow either failure line
	assert.NotContains(t, out, "Configs failed:")
	assert.NotContains(t, out, "Last error:")
	assert.Contains(t, out, "Configs in last update: 2")
	assert.Contains(t, out, "Updates received: 1")
	assert.NotContains(t, out, "no update received yet")
}

// TestConfigRetrieverRecordsRemoteConfigUpdate covers the subscription wiring,
// which the tests above bypass by calling recordRemoteConfigUpdate directly.
// Swapping the two product constants in NewConfigRetriever must fail here. The payload is
// malformed on purpose: an update whose configs all fail to apply must still
// count as received.
func TestConfigRetrieverRecordsRemoteConfigUpdate(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)

	store := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	InitStatus(store, func() bool { return true })
	_, rcClient := newMockConfigRetriever(t, func() bool { return true }, store, clock.RealClock{})

	require.Contains(t, renderText(t), "No update received yet")

	var applied []state.ApplyStatus
	rcClient.triggerUpdate(
		data.ProductContainerAutoscalingSettings,
		map[string]state.RawConfig{
			"foo": buildRawConfig(t, data.ProductContainerAutoscalingSettings, 1, []byte("{")),
		},
		func(_ string, status state.ApplyStatus) { applied = append(applied, status) },
	)

	// Assert the apply really did fail, so this pins "a failed update still
	// counts as received" rather than just "some update arrived".
	require.Len(t, applied, 1)
	require.Equal(t, state.ApplyStateError, applied[0].State)

	// Asserted per product: "Updates received: 1" appearing anywhere would also
	// hold if the subscription routed this update to the other product.
	out := renderText(t)
	settings := productBlock(t, out, data.ProductContainerAutoscalingSettings)
	assert.Contains(t, settings, "Updates received: 1")
	assert.Contains(t, settings, "Last error:")
	assert.Contains(t, productBlock(t, out, data.ProductContainerAutoscalingValues), "No update received yet")
}

// TestWorkloadAutoscalingStatusIgnoresUnknownProduct documents the silent drop: a
// product with no bucket must not panic and must not land in either of them.
func TestWorkloadAutoscalingStatusIgnoresUnknownProduct(t *testing.T) {
	resetStatus(t)

	assert.NotPanics(t, func() {
		recordRemoteConfigUpdate("SOME_OTHER_PRODUCT", map[string]state.RawConfig{
			"a": {Metadata: state.Metadata{Version: 1}},
		})
		recordRemoteConfigError("SOME_OTHER_PRODUCT", errors.New("boom"))
	})

	assert.True(t, statusStore.settings.lastUpdate.IsZero())
	assert.True(t, statusStore.values.lastUpdate.IsZero())
}

// TestWorkloadAutoscalingStatusProductsAreIndependent covers the bucket routing:
// making productStatusFor return the same bucket for both products must fail
// here. It drives recordRemoteConfigUpdate directly, so it says nothing about the
// subscriptions -- TestConfigRetrieverRecordsRemoteConfigUpdate covers those.
func TestWorkloadAutoscalingStatusProductsAreIndependent(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	recordRemoteConfigUpdate(data.ProductContainerAutoscalingValues, map[string]state.RawConfig{
		"a": {Metadata: state.Metadata{Version: 41}},
	})

	out := renderText(t)
	assert.Contains(t, productBlock(t, out, data.ProductContainerAutoscalingSettings), "No update received yet")
	assert.Contains(t, productBlock(t, out, data.ProductContainerAutoscalingValues), "Last config version: 41")
}

// TestWorkloadAutoscalingStatusReportsOnlyTheLatestUpdate covers the reset: an
// update carries the product's whole config set, so a stale version or a
// recovered error must not be reported as current.
func TestWorkloadAutoscalingStatusReportsOnlyTheLatestUpdate(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, map[string]state.RawConfig{
		"a": {Metadata: state.Metadata{Version: 99}},
	})
	recordRemoteConfigError(data.ProductContainerAutoscalingSettings, errors.New("bad spec"))
	require.Contains(t, renderText(t), "Last error: bad spec")
	require.Contains(t, renderText(t), "Configs failed: 1 of 1")

	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, map[string]state.RawConfig{
		"a": {Metadata: state.Metadata{Version: 4}},
	})

	out := renderText(t)
	assert.Contains(t, out, "Last config version: 4")
	assert.NotContains(t, out, "Last config version: 99")
	assert.NotContains(t, out, "Last error:")
	// errorCount is the third entry in the reset list, so the easiest to miss
	assert.NotContains(t, out, "Configs failed:")
	// The running total keeps accumulating across updates
	assert.Contains(t, out, "Updates received: 2")
}

// TestWorkloadAutoscalingStatusErrorStaysOnOneLine covers a config carrying
// several bad entries: processItem returns errors.Join over them, which is
// newline-separated, and the section renders one field per line.
func TestWorkloadAutoscalingStatusErrorStaysOnOneLine(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, map[string]state.RawConfig{
		"a": {Metadata: state.Metadata{Version: 1}},
	})
	recordRemoteConfigError(data.ProductContainerAutoscalingSettings,
		errors.Join(errors.New("first entry bad"), errors.New("second entry bad")))

	for _, line := range strings.Split(renderText(t), "\n") {
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, " "),
			"every rendered line must stay indented, got %q in:\n%s", line, renderText(t))
	}
}

// TestWorkloadAutoscalingStatusCountsFailingConfigs is the point of the count:
// without it, one failing config and most of them failing render identically.
func TestWorkloadAutoscalingStatusCountsFailingConfigs(t *testing.T) {
	resetStatus(t)
	enabledConfig(t, true)
	InitStatus(autoscalingstore.NewStore[model.PodAutoscalerInternal](), func() bool { return true })

	update := map[string]state.RawConfig{}
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		update[key] = state.RawConfig{Metadata: state.Metadata{Version: 1}}
	}
	recordRemoteConfigUpdate(data.ProductContainerAutoscalingSettings, update)

	// processorCallback calls this once per failing config.
	for _, key := range []string{"a", "b", "c"} {
		recordRemoteConfigError(data.ProductContainerAutoscalingSettings, errors.New("bad "+key))
	}

	assert.Contains(t, productBlock(t, renderText(t), data.ProductContainerAutoscalingSettings),
		"Configs failed: 3 of 5")
}
