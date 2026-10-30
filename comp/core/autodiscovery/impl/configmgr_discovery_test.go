// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package autodiscoveryimpl

import (
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
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
	cm, _, published := makeDiscoveryCM(t, "")
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

	cm.stop()
	assert.Empty(t, published.snapshot(), "discovery must not publish after permanent failure")
}

// TestConfigMgr_DiscoveryTemplate_RoutesThroughDiscoverer verifies the full
// end-to-end path: the configmgr enqueues a probe instead of resolving the
// template, the worker calls the stub, and the resolved config is delivered
// through the publication callback. It covers both a
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
			published := &configChangeRecorder{}
			cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
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

			// Wait for the worker to publish the resolved config.
			discovered := published.wait(t)
			assertConfigsMatch(t, discovered.Schedule, matchName("krakend"))
			require.Len(t, discovered.Schedule, 1)
			// %%host%% in the discovered config should have been resolved
			// through the normal configresolver path against the live service.
			assert.Contains(t, string(discovered.Schedule[0].Instances[0]), "10.0.0.1",
				"discovered instance config should have %%host%% resolved via the configresolver path")
			assert.Equal(t, tc.wantSource, discovered.Schedule[0].Source,
				"discovered config's Source should be tagged with the discovery provider")
			assert.Contains(t, string(discovered.Schedule[0].Instances[0]), configDiscoveryTag,
				"discovered instance config should carry the configuration-discovery marker tag")

			assert.GreaterOrEqual(t, int(disco.called.Load()), 1, "stub discoverer should have been called")
		})
	}
}

// countTag returns how many times tag appears in tags.
func countTag(tags []string, tag string) int {
	n := 0
	for _, t := range tags {
		if t == tag {
			n++
		}
	}
	return n
}

// instanceTags unmarshals the `tags` field of a resolved instance for
// precise assertions (exact membership/count), rather than substring
// matching on the raw YAML.
func instanceTags(t *testing.T, instance integration.Data) []string {
	t.Helper()
	var parsed struct {
		Tags []string `yaml:"tags"`
	}
	require.NoError(t, yaml.Unmarshal(instance, &parsed))
	return parsed.Tags
}

// TestConfigMgr_DiscoveryTemplate_TagsAllInstances verifies that the
// configuration-discovery marker tag is added exactly once to every instance of
// a discovered config, is merged alongside any tags the discovered instance
// already carries rather than replacing them, and is still added even when the
// discovered config opts out of ordinary autodiscovery service tags via
// ignore_autodiscovery_tags (which must still suppress the service's own tags,
// proving the marker tag is applied independently of that mechanism).
func TestConfigMgr_DiscoveryTemplate_TagsAllInstances(t *testing.T) {
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, _ string) (string, error) {
		return `[{
			"instances": [
				{"openmetrics_endpoint": "http://%%host%%:8080/metrics", "tags": ["custom:tag"]},
				{"openmetrics_endpoint": "http://%%host%%:8081/metrics"}
			],
			"ignore_autodiscovery_tags": true
		}]`, nil
	})
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
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
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
		Tags:          []string{"service:tag"},
	}

	_, _ = cm.processNewConfig(tpl)
	changes := cm.processNewService(svc)
	assertConfigsMatch(t, changes.Schedule)
	assertConfigsMatch(t, changes.Unschedule)

	discovered := published.wait(t)
	require.Len(t, discovered.Schedule, 1)
	require.Len(t, discovered.Schedule[0].Instances, 2)

	firstTags := instanceTags(t, discovered.Schedule[0].Instances[0])
	assert.Equal(t, 1, countTag(firstTags, configDiscoveryTag),
		"marker tag should appear exactly once on the first instance, got %v", firstTags)
	assert.Contains(t, firstTags, "custom:tag",
		"first instance's own tag should be preserved alongside the marker tag")
	assert.NotContains(t, firstTags, "service:tag",
		"ignore_autodiscovery_tags should still suppress ordinary service tags")

	secondTags := instanceTags(t, discovered.Schedule[0].Instances[1])
	assert.Equal(t, 1, countTag(secondTags, configDiscoveryTag),
		"marker tag should appear exactly once on the second instance, got %v", secondTags)
}

// TestConfigMgr_DiscoveryTemplate_TagPersistsAcrossRediscovery verifies that
// a second discovery result for the same service+template (e.g. the
// integration's discover_config returning updated instance data on a later
// probe) still carries exactly one configDiscoveryTag on the replacement
// config, rather than accumulating duplicates or losing the tag across
// re-resolution.
func TestConfigMgr_DiscoveryTemplate_TagPersistsAcrossRediscovery(t *testing.T) {
	mockResolver := MockSecretResolver{}
	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
		Source:        "file:/etc/datadog-agent/conf.d/krakend.d/auto_conf.yaml",
		Provider:      names.File,
	}
	svc := &dummyService{
		ID:            "docker://k1",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}

	// The discovery worker is never started: this test drives
	// applyDiscoveredConfigsLocked directly (as the worker's callback would),
	// so no discoverer or running goroutine is needed.
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)
	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	tplDigest := tpl.Digest()
	firstConfigs := []integration.Config{{
		Instances: []integration.Data{integration.Data(`openmetrics_endpoint: http://%%host%%:8080/metrics`)},
	}}
	cm.m.Lock()
	changes := cm.applyDiscoveredConfigsLocked(svc.ID, tplDigest, firstConfigs)
	cm.m.Unlock()
	require.Len(t, changes.Schedule, 1)
	firstTags := instanceTags(t, changes.Schedule[0].Instances[0])
	require.Equal(t, 1, countTag(firstTags, configDiscoveryTag))

	secondConfigs := []integration.Config{{
		Instances: []integration.Data{integration.Data(`openmetrics_endpoint: http://%%host%%:9090/metrics`)},
	}}
	cm.m.Lock()
	changes = cm.applyDiscoveredConfigsLocked(svc.ID, tplDigest, secondConfigs)
	cm.m.Unlock()
	require.Len(t, changes.Schedule, 1, "rediscovery should schedule the replacement config")
	require.Len(t, changes.Unschedule, 1, "rediscovery should unschedule the previous config")
	secondTags := instanceTags(t, changes.Schedule[0].Instances[0])
	assert.Equal(t, 1, countTag(secondTags, configDiscoveryTag),
		"marker tag should appear exactly once on the re-discovered config, got %v", secondTags)
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
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
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

// midFlightDiscoverySuppressionHarness sets up a configmgr with a "krakend"
// discovery template + service whose probe is blocked on a channel, so tests
// can deterministically inject a conflicting config while the probe is in
// flight and then verify the (stale) result gets suppressed once released.
// Used by the DSCVR-626 regression tests below.
type midFlightDiscoverySuppressionHarness struct {
	cm        *reconcilingConfigManager
	release   chan struct{}
	processed chan struct{}
	published *configChangeRecorder
}

// newMidFlightDiscoverySuppressionHarness builds the harness, installs
// filterTemplates on the service (it should mirror whichever
// filterTemplatesDiscovery rule the test wants to exercise), and blocks until
// the probe has actually started.
func newMidFlightDiscoverySuppressionHarness(t *testing.T, staticConfigIndex *listeners.StaticConfigIndex, filterTemplates func(map[string]integration.Config)) *midFlightDiscoverySuppressionHarness {
	t.Helper()
	mockResolver := MockSecretResolver{}

	probeStarted := make(chan struct{})
	release := make(chan struct{})
	disco := newStubDiscoverer(func(_, _ string) (string, error) {
		close(probeStarted)
		<-release
		return `[{"instances":[{"openmetrics_endpoint":"http://%%host%%:8080/metrics"}]}]`, nil
	})
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, staticConfigIndex, disco, nil, published.record).(*reconcilingConfigManager)
	cm.start()
	t.Cleanup(cm.stop)

	// Wrap the worker callback so the test can wait deterministically for the
	// released probe result to be fully processed (scheduled or dropped)
	// before checking published changes — a fixed sleep could pass even if
	// the callback hadn't run yet under a slow/stalled scheduler.
	processed := make(chan struct{})
	cm.discoveryWorker.Stop()
	cm.discoveryWorker = discoverer.NewWorker(
		disco,
		cmServiceLookup{cm},
		func(svcID, tplDigest string, configs []integration.Config) {
			cm.onDiscoveryResult(svcID, tplDigest, configs)
			close(processed)
		},
		discoverer.Config{},
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
	svc.filterTemplates = filterTemplates

	_, _ = cm.processNewConfig(tpl)
	_ = cm.processNewService(svc)

	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("probe never started")
	}

	return &midFlightDiscoverySuppressionHarness{cm: cm, release: release, processed: processed, published: published}
}

// releaseAndAssertSuppressed unblocks the in-flight probe and asserts that
// its result is never published. It waits deterministically
// for the wrapped callback to finish (see newMidFlightDiscoverySuppressionHarness)
// before checking publication, rather than sleeping a fixed duration.
func (h *midFlightDiscoverySuppressionHarness) releaseAndAssertSuppressed(t *testing.T) {
	t.Helper()
	before := h.published.snapshot()
	close(h.release)
	select {
	case <-h.processed:
	case <-time.After(2 * time.Second):
		t.Fatal("discovery callback never processed the released probe result")
	}

	assert.Equal(t, before, h.published.snapshot(), "the suppressed probe must not publish a config")
}

// TestConfigMgr_Discovery_StaleProbeResultSuppressedByMidFlightConflict
// reproduces the DSCVR-626 race: FilterTemplates only runs synchronously
// inside reconcileService, so it can't affect a probe that's already in
// flight. Without a re-check immediately before applying the probe result, a
// conflicting generic-integration config that arrives for the same service
// or host *after* the probe was enqueued but *before* it completes would
// still get scheduled on top of it, defeating the point of the filtering.
//
// Both subtests below end up dropping the discovery digest for the same
// (test-local, faked) reason — that part is deliberately trivial. What
// actually differs, and is the point of having two cases, is *how* the
// conflicting config reaches the config manager, which exercises two
// different branches of processNewConfig:
//
//   - "sibling template arrives": the conflict has AD identifiers, so it IS
//     a template. This DOES trigger a synchronous reconcileService for the
//     affected service — but that alone changes nothing, because a
//     discovery digest is never recorded in existingResolutions while its
//     probe is pending, so reconcileService has nothing to unschedule. This
//     proves the apply-time re-check matters even when reconcile does run.
//   - "static config arrives host-wide": the conflict has no AD
//     identifiers, so it's non-template. This NEVER triggers
//     reconcileService for any already-active service at all (see the TODO
//     in processNewConfig) — it only updates the shared StaticConfigIndex.
//     This proves the apply-time re-check is the *only* thing that can catch
//     this case; there's no reconcile to even race against.
//
// In both cases, applyDiscoveredConfigsLocked must re-validate against live
// filtering state before scheduling the (stale) probe result.
func TestConfigMgr_Discovery_StaleProbeResultSuppressedByMidFlightConflict(t *testing.T) {
	t.Run("sibling template arrives for the same service", func(t *testing.T) {
		// Mirrors filterTemplatesDiscovery's generic-integration-sibling rule:
		// drop every discovery template while an openmetrics/prometheus
		// sibling is present, regardless of Name.
		filterTemplates := func(configs map[string]integration.Config) {
			hasGenericSibling := false
			for _, cfg := range configs {
				if cfg.Discovery == nil && len(cfg.Instances) > 0 && (cfg.Name == "openmetrics" || cfg.Name == "prometheus") {
					hasGenericSibling = true
				}
			}
			if !hasGenericSibling {
				return
			}
			for digest, cfg := range configs {
				if cfg.Discovery != nil {
					delete(configs, digest)
				}
			}
		}
		h := newMidFlightDiscoverySuppressionHarness(t, nil, filterTemplates)

		// While the probe is in flight, a sibling openmetrics config for the
		// same service arrives. Having AD identifiers makes this a template,
		// so it triggers a real (but ultimately inconsequential)
		// reconcileService for the service.
		sibling := integration.Config{
			Name:          "openmetrics",
			ADIdentifiers: []string{"krakend"},
			Instances:     []integration.Data{[]byte("openmetrics_endpoint: http://10.0.0.1:9091/metrics")},
		}
		changes, _ := h.cm.processNewConfig(sibling)
		assertConfigsMatch(t, changes.Schedule, matchName("openmetrics"))

		h.releaseAndAssertSuppressed(t)
	})

	t.Run("static config arrives host-wide", func(t *testing.T) {
		idx := listeners.NewStaticConfigIndex()
		// Mirrors filterTemplatesDiscovery's static-index rule: drop every
		// discovery template while a host-wide generic-integration static
		// config is present in the shared index.
		filterTemplates := func(configs map[string]integration.Config) {
			if !idx.Has("openmetrics") && !idx.Has("prometheus") {
				return
			}
			for digest, cfg := range configs {
				if cfg.Discovery != nil {
					delete(configs, digest)
				}
			}
		}
		h := newMidFlightDiscoverySuppressionHarness(t, idx, filterTemplates)

		// A static openmetrics config arrives while the probe is in flight.
		// It has no AD identifiers, so it's non-template and never touches
		// reconcileService — only the shared index.
		staticOpenmetrics := integration.Config{
			Name:      "openmetrics",
			Instances: []integration.Data{[]byte("openmetrics_endpoint: http://10.0.0.2:9091/metrics")},
		}
		changes, _ := h.cm.processNewConfig(staticOpenmetrics)
		assertConfigsMatch(t, changes.Schedule, matchName("openmetrics"))
		assert.True(t, idx.Has("openmetrics"))

		h.releaseAndAssertSuppressed(t)
	})
}

// makeDiscoveryCM is a small constructor used by the lifecycle tests below.
// It wires up a reconcilingConfigManager with a stub discoverer that returns
// the supplied JSON for every call, starts the worker, and registers a
// t.Cleanup so the worker is stopped exactly once.
func makeDiscoveryCM(t *testing.T, payload string) (*reconcilingConfigManager, *stubDiscoverer, *configChangeRecorder) {
	t.Helper()
	mockResolver := MockSecretResolver{}
	disco := newStubDiscoverer(func(_, _ string) (string, error) { return payload, nil })
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
	cm.start()
	t.Cleanup(cm.stop)
	return cm, disco, published
}

// configChangeRecorder observes received change publications.
type configChangeRecorder struct {
	mu      sync.Mutex
	changes []integration.ConfigChanges
}

func (r *configChangeRecorder) record(changes integration.ConfigChanges, _ map[checkid.ID]checkid.ID) {
	if len(changes.Schedule)+len(changes.Unschedule) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = append(r.changes, changes)
}

func (r *configChangeRecorder) snapshot() []integration.ConfigChanges {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.changes)
}

func (r *configChangeRecorder) wait(t *testing.T) integration.ConfigChanges {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.snapshot()) > 0 }, 2*time.Second, time.Millisecond,
		"timed out waiting for published changes")
	r.mu.Lock()
	defer r.mu.Unlock()
	changes := r.changes[0]
	r.changes = r.changes[1:]
	return changes
}

// TestConfigMgr_Lifecycle_ServiceArrivesBeforeTemplate is the symmetric flow
// to the happy-path test: when the service is registered first and the
// Discovery template arrives later, the probe still fires once both are
// known and the resolved config is delivered.
func TestConfigMgr_Lifecycle_ServiceArrivesBeforeTemplate(t *testing.T) {
	cm, disco, published := makeDiscoveryCM(t, `[{"instances":[{"openmetrics_endpoint":"http://%%host%%:8080/metrics"}]}]`)

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

	got := published.wait(t)
	assertConfigsMatch(t, got.Schedule, matchName("krakend"))
	require.Len(t, got.Schedule, 1)
	assert.Contains(t, string(got.Schedule[0].Instances[0]), "10.0.0.1")
	assert.GreaterOrEqual(t, int(disco.called.Load()), 1)
}

// TestConfigMgr_Lifecycle_ServiceRemovedAfterDiscovery: once a discovered
// config is scheduled, deleting the service unschedules it (so the
// downstream scheduler tears the check down).
func TestConfigMgr_Lifecycle_ServiceRemovedAfterDiscovery(t *testing.T) {
	cm, _, published := makeDiscoveryCM(t, `[{"instances":[{"port":8080}]}]`)

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
	got := published.wait(t)
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
	cm, _, published := makeDiscoveryCM(t, `[{"instances":[{"port":8080}]}]`)

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

	got := published.wait(t)
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
	cm, disco, published := makeDiscoveryCM(t, `[]`)
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

	// Join the worker before checking that it published nothing.
	cm.stop()
	assert.Empty(t, published.snapshot(), "no configs should be scheduled when the integration only returns empty arrays")
	assert.Equal(t, int32(fastWorkerMaxAttempts), disco.called.Load(),
		"worker should run exactly maxAttempts probes before giving up")
}

// TestConfigMgr_Lifecycle_MultipleInstances: a discovered config with two
// instances in the payload schedules a single config carrying both
// instances. (Matches the PoC convention: only the first discoveredConfig
// in the JSON array is consumed, but it may carry many instances.)
func TestConfigMgr_Lifecycle_MultipleInstances(t *testing.T) {
	cm, _, published := makeDiscoveryCM(t, `[{"instances":[{"port":8080},{"port":9090}]}]`)

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

	got := published.wait(t)
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
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
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

	_ = published.wait(t)

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
	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, nil, disco, nil, published.record).(*reconcilingConfigManager)
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

	_ = published.wait(t)

	var payload struct {
		Host string `json:"host"`
	}
	require.NoError(t, json.Unmarshal([]byte(capturedJSON.Load().(string)), &payload))
	assert.Equal(t, "10.0.0.1", payload.Host, "discovery must pick the bridge IP when multiple networks exist")
}

// fastWorkerMaxAttempts is the retry budget used by the
// EmptyDiscoveryResult lifecycle test — small so the test finishes quickly.
const fastWorkerMaxAttempts = 3
