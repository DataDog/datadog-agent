// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package clusteridresolverimpl

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clusteridresolver "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

const testID = "226430c6-5e57-11ea-91d5-42010a8400c6"

func startedResolver(t *testing.T, lookup lookupFunc, gateHealth bool) (*resolver, *compdef.TestLifecycle) {
	t.Helper()
	lc := compdef.NewTestLifecycle(t)
	r := newResolver(logmock.New(t))
	r.backoff = &backoff.ZeroBackOff{}
	r.start(lc, lookup, gateHealth)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, lc.Stop(ctx))
	})
	return r, lc
}

func newComponent(t *testing.T, agentFlavor string, cfg config.Component) clusteridresolver.Component {
	t.Helper()
	flavor.SetTestFlavor(t, agentFlavor)
	return NewComponent(Requires{Lifecycle: compdef.NewTestLifecycle(t), Config: cfg, Log: logmock.New(t)}).Comp
}

func assertHealthBlocked(t *testing.T, blocked bool) {
	t.Helper()
	for _, status := range []health.Status{health.GetStartup(), health.GetLive(), health.GetReady()} {
		if blocked {
			assert.Contains(t, status.Unhealthy, healthCheckName)
		} else {
			assert.NotContains(t, status.Unhealthy, healthCheckName)
		}
	}
}

func assertSameLookup(t *testing.T, expected, actual lookupFunc) {
	t.Helper()
	assert.Equal(t, reflect.ValueOf(expected).Pointer(), reflect.ValueOf(actual).Pointer())
}

func TestResolutionRetriesAndSharesResult(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	r, lc := startedResolver(t, func(ctx context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "", errors.New("temporary failure")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return testID, nil
		}
	}, true)
	assertHealthBlocked(t, true)
	_, err := r.GetID()
	require.ErrorIs(t, err, clusteridresolver.ErrNotResolved)

	require.NoError(t, lc.Start(t.Context()))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			id, err := r.WaitForID(t.Context())
			assert.NoError(t, err)
			assert.Equal(t, testID, id)
		})
	}
	close(release)
	wg.Wait()
	assertHealthBlocked(t, false)

	// A read after success must not send a new request.
	id, err := r.GetID()
	require.NoError(t, err)
	assert.Equal(t, testID, id)
	assert.EqualValues(t, 2, calls.Load())
	// Legacy callers get the same ID.
	legacyID, err := clustername.GetClusterID()
	require.NoError(t, err)
	assert.Equal(t, testID, legacyID)
}

func TestInvalidIDIsRetried(t *testing.T) {
	var calls atomic.Int32
	r, lc := startedResolver(t, func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "invalid", nil
		}
		return testID, nil
	}, false)
	require.NoError(t, lc.Start(t.Context()))
	id, err := r.WaitForID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, testID, id)
	assert.EqualValues(t, 2, calls.Load())
}

func TestWaitCancellationDoesNotCancelResolution(t *testing.T) {
	release := make(chan struct{})
	r, lc := startedResolver(t, func(ctx context.Context) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return testID, nil
		}
	}, true)
	require.NoError(t, lc.Start(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := r.WaitForID(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assertHealthBlocked(t, true)

	close(release)
	id, err := r.WaitForID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, testID, id)
}

func TestShutdownCancelsResolutionAndReleasesWaiters(t *testing.T) {
	entered := make(chan struct{})
	r, lc := startedResolver(t, func(ctx context.Context) (string, error) {
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}, true)
	require.NoError(t, lc.Start(t.Context()))
	<-entered
	require.NoError(t, lc.Stop(t.Context()))
	_, err := r.WaitForID(t.Context())
	require.ErrorIs(t, err, context.Canceled)
	assertHealthBlocked(t, false)
}

func TestResolutionPreservesOtherReadinessChecks(t *testing.T) {
	other := health.RegisterReadiness("other-component")
	t.Cleanup(func() { require.NoError(t, other.Deregister()) })
	r, lc := startedResolver(t, func(context.Context) (string, error) { return testID, nil }, true)
	require.NoError(t, lc.Start(t.Context()))
	_, err := r.WaitForID(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, health.GetReady().Unhealthy, healthCheckName)
	assert.Contains(t, health.GetReady().Unhealthy, "other-component")
}

func TestOverrideHasPriority(t *testing.T) {
	for _, agentFlavor := range []string{flavor.DefaultAgent, flavor.ClusterAgent} {
		t.Run(agentFlavor, func(t *testing.T) {
			t.Setenv(clustername.ClusterIDEnv, testID)
			r := newComponent(t, agentFlavor, config.NewMock(t))
			// The ID is available before the component starts.
			id, err := r.GetID()
			require.NoError(t, err)
			assert.Equal(t, testID, id)
			assertHealthBlocked(t, false)
		})
	}
}

func TestInvalidOverrideIsIgnored(t *testing.T) {
	env.SetFeatures(t)
	for _, override := range []string{"", "invalid"} {
		t.Run(override, func(t *testing.T) {
			t.Setenv(clustername.ClusterIDEnv, override)
			_, err := newComponent(t, flavor.DefaultAgent, config.NewMock(t)).WaitForID(t.Context())
			require.ErrorIs(t, err, clusteridresolver.ErrDisabled)
		})
	}
}

func TestDisabledResolutionDoesNotGateHealth(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("cloud_foundry", true)
	_, err := newComponent(t, flavor.ClusterAgent, cfg).WaitForID(t.Context())
	require.ErrorIs(t, err, clusteridresolver.ErrDisabled)
	assertHealthBlocked(t, false)
}

func TestClusterAgentUsesKubernetes(t *testing.T) {
	assertSameLookup(t, resolveFromKubernetes, selectLookup(config.NewMock(t), true))

	cfg := config.NewMock(t)
	cfg.SetInTest("cloud_foundry", true)
	assert.Nil(t, selectLookup(cfg, true))
}

func TestNodeAgentUsesClusterAgentWhenReachable(t *testing.T) {
	tests := []struct {
		name     string
		features []env.Feature
		settings map[string]any
		expected lookupFunc
	}{
		{name: "kubernetes", features: []env.Feature{env.Kubernetes}, expected: resolveFromClusterAgent},
		{name: "eks fargate", features: []env.Feature{env.EKSFargate}, expected: resolveFromClusterAgent},
		{name: "clc runner", settings: map[string]any{"clc_runner_enabled": true}, expected: resolveFromClusterAgent},
		{name: "explicit url", settings: map[string]any{"cluster_agent.url": "https://cluster-agent:5005"}, expected: resolveFromClusterAgent},
		{name: "no cluster agent endpoint"},
		{name: "cluster agent disabled", features: []env.Feature{env.Kubernetes}, settings: map[string]any{"cluster_agent.enabled": false}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env.SetFeatures(t, test.features...)
			cfg := config.NewMock(t)
			cfg.SetInTest("cluster_agent.enabled", true)
			for key, value := range test.settings {
				cfg.SetInTest(key, value)
			}
			assertSameLookup(t, test.expected, selectLookup(cfg, false))
		})
	}
}
