// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package autodiscoveryimpl

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// stubDiscoverer is a discoverer.ConfigDiscoverer used by configmgr tests.
type stubDiscoverer struct {
	mu     sync.Mutex
	called *atomic.Int32
	fn     func(integrationName, serviceJSON string) (string, error)
}

func newStubDiscoverer(fn func(integrationName, serviceJSON string) (string, error)) *stubDiscoverer {
	return &stubDiscoverer{called: atomic.NewInt32(0), fn: fn}
}

func (s *stubDiscoverer) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	s.called.Add(1)
	s.mu.Lock()
	fn := s.fn
	s.mu.Unlock()
	return fn(integrationName, serviceJSON)
}

// TestConfigMgr_DiscoveryTemplate_NoPythonNeverSchedules verifies that a
// discoverer returning PermFail (simulating a no-python build) drops the job
// after a single attempt and never delivers a config to the scheduler.
func TestConfigMgr_DiscoveryTemplate_NoPythonNeverSchedules(t *testing.T) {
	called := atomic.NewInt32(0)
	disco := newStubDiscoverer(func(_, _ string) (string, error) {
		called.Add(1)
		return "", discoverer.PermFail{Err: assert.AnError}
	})
	cm, _ := makeDiscoveryCM(t, "")
	cm.discoveryWorker.Stop()
	cm.discoveryWorker = discoverer.NewWorker(
		disco,
		cmServiceLookup{cm},
		cm.onDiscoveryResult,
		discoverer.Config{MaxAttempts: fastWorkerMaxAttempts, RetryDelay: 10 * time.Millisecond},
		nil,
	)
	cm.discoveryWorker.Start()

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}

	cm.processNewConfig(tpl)
	cm.processNewService(svc)

	require.Eventually(t, func() bool { return called.Load() >= 1 }, time.Second, 50*time.Millisecond)
	assert.Equal(t, int32(1), called.Load(), "PermFail should drop the job after a single attempt")

	select {
	case <-cm.discoveredChanges():
		t.Fatal("no config should be delivered when discovery permanently fails")
	default:
	}
}

// TestConfigMgr_DiscoveryTemplate_RoutesThroughDiscoverer verifies the full
// end-to-end path: the configmgr enqueues a probe instead of resolving the
// template, the worker calls the stub, and the resolved config is delivered
// via discoveredChanges() to be applied by AutoConfig. It covers both a
// container and a process service, which should get distinct
// discovery-specific Source prefixes.
func TestConfigMgr_DiscoveryTemplate_RoutesThroughDiscoverer(t *testing.T) {
	tests := []struct {
		name       string
		svcID      string
		wantSource string
	}{
		{
			name:       "container service",
			svcID:      "docker://k1",
			wantSource: names.ADContainerDiscovery + ":/etc/datadog-agent/conf.d/krakend.d/auto_conf.yaml",
		},
		{
			name:       "process service",
			svcID:      "process://1234",
			wantSource: names.ADProcessDiscovery + ":/etc/datadog-agent/conf.d/krakend.d/auto_conf.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockResolver := MockSecretResolver{}
			disco := newStubDiscoverer(func(_, _ string) (string, error) {
				return `[{"instances":[{"openmetrics_endpoint":"http://%%host%%:8080/metrics"}]}]`, nil
			})
			cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil).(*reconcilingConfigManager)
			cm.start()
			defer cm.stop()

			tpl := integration.Config{
				Name:          "krakend",
				ADIdentifiers: []string{"krakend"},
				Discovery:     &integration.DiscoveryConfig{},
				Source:        "file:/etc/datadog-agent/conf.d/krakend.d/auto_conf.yaml",
				Provider:      names.File,
			}
			svc := &dummyService{
				ID:            tc.svcID,
				ADIdentifiers: []string{"krakend"},
				Hosts:         map[string]string{"main": "10.0.0.1"},
			}

			// processNewConfig + processNewService should NOT schedule synchronously
			// — the template goes through the discovery path instead.
			_, _ = cm.processNewConfig(tpl)
			changes := cm.processNewService(svc)
			assertConfigsMatch(t, changes.Schedule)
			assertConfigsMatch(t, changes.Unschedule)

			// Drain the discovered-changes channel for up to one second.
			ch := cm.discoveredChanges()
			require.NotNil(t, ch)
			select {
			case discovered := <-ch:
				assertConfigsMatch(t, discovered.Schedule, matchName("krakend"))
				require.Len(t, discovered.Schedule, 1)
				// %%host%% in the discovered config should have been resolved
				// through the normal configresolver path against the live service.
				assert.Contains(t, string(discovered.Schedule[0].Instances[0]), "10.0.0.1",
					"discovered instance config should have %%host%% resolved via the configresolver path")
				assert.Equal(t, tc.wantSource, discovered.Schedule[0].Source,
					"discovered config's Source should be tagged with the discovery provider")
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for discovered changes")
			}

			assert.GreaterOrEqual(t, int(disco.called.Load()), 1, "stub discoverer should have been called")
		})
	}
}

// TestConfigMgr_DiscoveryTemplate_ServiceDeletionCancels confirms that
// deleting the service forgets in-flight probes so the worker stops retrying.
func TestConfigMgr_DiscoveryTemplate_ServiceDeletionCancels(t *testing.T) {
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, _ string) (string, error) {
		// Always fail so the worker keeps retrying — until the service is
		// forgotten.
		return "", assert.AnError
	})
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil).(*reconcilingConfigManager)
	cm.start()
	defer cm.stop()

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}

	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	// Wait until the worker has actually run at least one probe.
	require.Eventually(t, func() bool { return disco.called.Load() >= 1 }, time.Second, 5*time.Millisecond)

	// Delete the service and confirm that probes stop. We can't tell with
	// certainty when an in-flight retry has settled, so we observe the
	// current call count, wait, and assert it didn't grow.
	_ = cm.processDelService(svc)
	settled := disco.called.Load()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, settled, disco.called.Load(),
		"no further probes should fire after the service is removed (started at %d)", settled)
}

// makeDiscoveryCM is a small constructor used by the lifecycle tests below.
// It wires up a reconcilingConfigManager with a stub discoverer that returns
// the supplied JSON for every call, starts the worker, and registers a
// t.Cleanup so the worker is stopped exactly once.
func makeDiscoveryCM(t *testing.T, payload string) (*reconcilingConfigManager, *stubDiscoverer) {
	t.Helper()
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, _ string) (string, error) { return payload, nil })
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil).(*reconcilingConfigManager)
	cm.start()
	t.Cleanup(cm.stop)
	return cm, disco
}

// waitForDiscoveredChange waits for a single ConfigChanges to arrive on the
// discovered-changes channel. Fails the test on timeout.
func waitForDiscoveredChange(t *testing.T, ch <-chan integration.ConfigChanges) integration.ConfigChanges {
	t.Helper()
	select {
	case changes := <-ch:
		return changes
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for discovered changes")
		return integration.ConfigChanges{} // unreachable
	}
}

// drainDiscoveredChanges collects any pending discovered changes for a short
// settling period and returns them. Used by tests that need to assert "no
// further changes arrive".
func drainDiscoveredChanges(ch <-chan integration.ConfigChanges, settle time.Duration) []integration.ConfigChanges {
	var collected []integration.ConfigChanges
	deadline := time.After(settle)
	for {
		select {
		case c := <-ch:
			collected = append(collected, c)
		case <-deadline:
			return collected
		}
	}
}

// TestConfigMgr_Lifecycle_ServiceArrivesBeforeTemplate is the symmetric flow
// to the happy-path test: when the service is registered first and the
// Discovery template arrives later, the probe still fires once both are
// known and the resolved config is delivered.
func TestConfigMgr_Lifecycle_ServiceArrivesBeforeTemplate(t *testing.T) {
	cm, disco := makeDiscoveryCM(t, `[{"instances":[{"openmetrics_endpoint":"http://%%host%%:8080/metrics"}]}]`)

	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}

	// Service first.
	changes := cm.processNewService(svc)
	assertConfigsMatch(t, changes.Schedule)
	assertConfigsMatch(t, changes.Unschedule)
	// No probes have been queued because there's no Discovery template yet.
	time.Sleep(20 * time.Millisecond)
	assert.Zero(t, disco.called.Load())

	// Template arrives — synchronous returned changes stay empty (deferred),
	// but the worker now has work to do.
	changes, _ = cm.processNewConfig(tpl)
	assertConfigsMatch(t, changes.Schedule)
	assertConfigsMatch(t, changes.Unschedule)

	got := waitForDiscoveredChange(t, cm.discoveredChanges())
	assertConfigsMatch(t, got.Schedule, matchName("krakend"))
	require.Len(t, got.Schedule, 1)
	assert.Contains(t, string(got.Schedule[0].Instances[0]), "10.0.0.1")
	assert.GreaterOrEqual(t, int(disco.called.Load()), 1)
}

// TestConfigMgr_Lifecycle_ServiceRemovedAfterDiscovery: once a discovered
// config is scheduled, deleting the service unschedules it (so the
// downstream scheduler tears the check down).
func TestConfigMgr_Lifecycle_ServiceRemovedAfterDiscovery(t *testing.T) {
	cm, _ := makeDiscoveryCM(t, `[{"instances":[{"port":8080}]}]`)
	ch := cm.discoveredChanges()

	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	// First arrival: scheduled by the worker.
	got := waitForDiscoveredChange(t, ch)
	require.Len(t, got.Schedule, 1)
	scheduledDigest := got.Schedule[0].Digest()

	// Service deletion should unschedule the resolved config that the
	// worker had previously scheduled.
	changes := cm.processDelService(svc)
	assertConfigsMatch(t, changes.Schedule)
	assertConfigsMatch(t, changes.Unschedule, matchDigest(scheduledDigest))
}

// TestConfigMgr_Lifecycle_TemplateRemovedAfterDiscovery: once a discovered
// config is scheduled, removing the underlying Discovery template
// unschedules the resolved config.
func TestConfigMgr_Lifecycle_TemplateRemovedAfterDiscovery(t *testing.T) {
	cm, _ := makeDiscoveryCM(t, `[{"instances":[{"port":8080}]}]`)
	ch := cm.discoveredChanges()

	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	got := waitForDiscoveredChange(t, ch)
	require.Len(t, got.Schedule, 1)
	scheduledDigest := got.Schedule[0].Digest()

	// Removing the template must unschedule the resolved config.
	changes := cm.processDelConfigs([]integration.Config{tpl})
	assertConfigsMatch(t, changes.Schedule)
	assertConfigsMatch(t, changes.Unschedule, matchDigest(scheduledDigest))
}

// TestConfigMgr_Lifecycle_EmptyDiscoveryResult: a probe that always returns
// an empty array eventually gives up and never schedules anything. (This is
// the integration's way of saying "I cannot configure for this service" —
// e.g. the wrong port set was exposed.)
func TestConfigMgr_Lifecycle_EmptyDiscoveryResult(t *testing.T) {
	cm, disco := makeDiscoveryCM(t, `[]`)
	// Replace the default worker (10s retry × 5 attempts ≈ 50s) with one
	// tuned for fast tests: 5ms × maxAttempts. The configmgr keeps the
	// same callbacks and lookup so behaviour is otherwise unchanged.
	cm.discoveryWorker.Stop()
	cm.discoveryWorker = discoverer.NewWorker(
		disco,
		cmServiceLookup{cm},
		cm.onDiscoveryResult,
		discoverer.Config{MaxAttempts: fastWorkerMaxAttempts, RetryDelay: 5 * time.Millisecond},
		nil,
	)
	cm.discoveryWorker.Start()

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	// Wait long enough for all retry attempts to be exhausted.
	require.Eventually(t, func() bool { return disco.called.Load() >= int32(fastWorkerMaxAttempts) },
		2*time.Second, 5*time.Millisecond)

	// Nothing should have shown up on the discovered-changes channel.
	remaining := drainDiscoveredChanges(cm.discoveredChanges(), 50*time.Millisecond)
	assert.Empty(t, remaining, "no configs should be scheduled when the integration only returns empty arrays")
	assert.Equal(t, int32(fastWorkerMaxAttempts), disco.called.Load(),
		"worker should run exactly maxAttempts probes before giving up")
}

// TestConfigMgr_Lifecycle_MultipleInstances: a discovered config with two
// instances in the payload schedules a single config carrying both
// instances. (Matches the PoC convention: only the first discoveredConfig
// in the JSON array is consumed, but it may carry many instances.)
func TestConfigMgr_Lifecycle_MultipleInstances(t *testing.T) {
	cm, _ := makeDiscoveryCM(t, `[{"instances":[{"port":8080},{"port":9090}]}]`)
	ch := cm.discoveredChanges()

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	got := waitForDiscoveredChange(t, ch)
	require.Len(t, got.Schedule, 1, "exactly one Config should be scheduled even with multiple instances")
	assert.Len(t, got.Schedule[0].Instances, 2)
}

// TestConfigMgr_Lifecycle_HostPortsPassedToDiscoverer captures the most
// important contract of the discovery payload: the integration receives the
// resolved host and the *complete* port list (number + name) from the live
// service, in the order GetPorts() returns them.
func TestConfigMgr_Lifecycle_HostPortsPassedToDiscoverer(t *testing.T) {
	var capturedJSON atomic.Value
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, serviceJSON string) (string, error) {
		capturedJSON.Store(serviceJSON)
		return `[{"instances":[{"port":8080}]}]`, nil
	})
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil).(*reconcilingConfigManager)
	cm.start()
	t.Cleanup(cm.stop)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	// Single network: matches %%host%%'s single-network fallback rule.
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
		Ports: []workloadmeta.ContainerPort{
			{Name: "http", Port: 8080},
			{Name: "metrics", Port: 9090},
		},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	_ = waitForDiscoveredChange(t, cm.discoveredChanges())

	raw, ok := capturedJSON.Load().(string)
	require.True(t, ok, "stub discoverer must have been called")
	type port struct {
		Number int    `json:"number"`
		Name   string `json:"name"`
	}
	var payload struct {
		ID    string `json:"id"`
		Host  string `json:"host"`
		Ports []port `json:"ports"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	assert.Equal(t, "docker://k1", payload.ID)
	assert.Equal(t, "10.0.0.1", payload.Host)
	require.Len(t, payload.Ports, 2)
	assert.Equal(t, port{Number: 8080, Name: "http"}, payload.Ports[0])
	assert.Equal(t, port{Number: 9090, Name: "metrics"}, payload.Ports[1])
}

// TestConfigMgr_Lifecycle_HostMultiNetworkBridge: a service exposing both
// "bridge" and a custom network resolves to the bridge IP — same behaviour
// as %%host%%.
func TestConfigMgr_Lifecycle_HostMultiNetworkBridge(t *testing.T) {
	var capturedJSON atomic.Value
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, serviceJSON string) (string, error) {
		capturedJSON.Store(serviceJSON)
		return `[{"instances":[{"port":8080}]}]`, nil
	})
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil).(*reconcilingConfigManager)
	cm.start()
	t.Cleanup(cm.stop)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"custom": "1.2.3.4", "bridge": "10.0.0.1"},
	}
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	_ = waitForDiscoveredChange(t, cm.discoveredChanges())

	var payload struct {
		Host string `json:"host"`
	}
	require.NoError(t, json.Unmarshal([]byte(capturedJSON.Load().(string)), &payload))
	assert.Equal(t, "10.0.0.1", payload.Host, "discovery must pick the bridge IP when multiple networks exist")
}

// fastWorkerMaxAttempts is the retry budget used by the
// EmptyDiscoveryResult lifecycle test — small so the test finishes quickly.
const fastWorkerMaxAttempts = 3
