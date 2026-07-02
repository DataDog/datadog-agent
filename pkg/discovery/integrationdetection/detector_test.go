// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package integrationdetection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// fakeAD captures the scheduler registered via AddScheduler, letting tests
// call Schedule/Unschedule directly. Unused methods panic to surface mistakes.
type fakeAD struct {
	schedulerCh chan scheduler.Scheduler
}

func newFakeAD() *fakeAD {
	return &fakeAD{schedulerCh: make(chan scheduler.Scheduler, 1)}
}

func (f *fakeAD) AddScheduler(_ string, s scheduler.Scheduler, _ bool) {
	f.schedulerCh <- s
}

func (f *fakeAD) RemoveScheduler(_ string) {}

// waitForScheduler blocks until AddScheduler has been called or the test finishes.
func (f *fakeAD) waitForScheduler(t *testing.T) scheduler.Scheduler {
	t.Helper()
	select {
	case s := <-f.schedulerCh:
		return s
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for scheduler registration: %v", t.Context().Err())
		return nil // unreachable: t.Fatalf calls runtime.Goexit
	}
}

// Unimplemented methods — panic to make accidental calls obvious.

func (f *fakeAD) AddConfigProvider(types.ConfigProvider, bool, time.Duration) {
	panic("not implemented")
}
func (f *fakeAD) LoadAndRun(context.Context)                              { panic("not implemented") }
func (f *fakeAD) GetAllConfigs() []integration.Config                    { panic("not implemented") }
func (f *fakeAD) GetUnresolvedConfigs() []integration.Config             { panic("not implemented") }
func (f *fakeAD) AddListeners([]pkgconfigsetup.Listeners)                { panic("not implemented") }
func (f *fakeAD) GetIDOfCheckWithEncryptedSecrets(checkid.ID) checkid.ID { panic("not implemented") }
func (f *fakeAD) GetAutodiscoveryErrors() map[string]map[string]types.ErrorMsgSet {
	panic("not implemented")
}
func (f *fakeAD) AddConfigProviderFromCatalog(pkgconfigsetup.ConfigurationProviders) error {
	panic("not implemented")
}
func (f *fakeAD) GetTelemetryStore() *telemetry.Store            { panic("not implemented") }
func (f *fakeAD) GetConfigCheck() integration.ConfigCheckResponse { panic("not implemented") }

// makeConfig creates a minimal integration.Config for testing.
// A non-empty Instances slice is required for IsCheckConfig() to return true.
func makeConfig(name, serviceID string, isCheck bool) integration.Config {
	cfg := integration.Config{
		Name:      name,
		ServiceID: serviceID,
	}
	if isCheck {
		cfg.Instances = []integration.Data{[]byte("{}")}
	}
	return cfg
}

// startDetector creates a Detector, registers it with a fakeAD, and returns
// both the detector and the captured scheduler for test use. It registers a
// cleanup that calls d.Stop(); tests that call d.Stop() manually are safe
// because Stop is guarded by sync.Once and the cleanup becomes a no-op.
func startDetector(t *testing.T) (*Detector, scheduler.Scheduler) {
	t.Helper()
	d := NewDetector()
	ac := newFakeAD()
	d.Start(ac)
	t.Cleanup(d.Stop)
	return d, ac.waitForScheduler(t)
}

// --- integrationForCheck allowlist tests ---

func TestIntegrationsAllowlist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		checkName     string
		wantCanon     string
		wantSupported bool
	}{
		{checkName: "redisdb", wantCanon: "redis", wantSupported: true},
		{checkName: "elastic", wantCanon: "elasticsearch", wantSupported: true},
		{checkName: "nginx", wantCanon: "nginx", wantSupported: true},
		{checkName: "etcd", wantCanon: "etcd", wantSupported: true},
		{checkName: "unknown", wantCanon: "", wantSupported: false},
		{checkName: "redis", wantCanon: "", wantSupported: false}, // binary name, not check name
	}
	for _, tc := range tests {
		t.Run(tc.checkName, func(t *testing.T) {
			t.Parallel()
			canon, ok := integrationForCheck(tc.checkName)
			assert.Equal(t, tc.wantSupported, ok)
			assert.Equal(t, tc.wantCanon, canon)
		})
	}
}

// --- classifyServiceID unit tests ---

func TestClassifyServiceID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		serviceID   string
		wantScope   Scope
		wantRuntime string
		wantCID     string
	}{
		{
			name:      "empty — file-based static config",
			wantScope: ScopeHost,
		},
		{
			name:      "process-based AD listener",
			serviceID: "process://12345",
			wantScope: ScopeHost,
		},
		{
			name:        "docker container",
			serviceID:   "docker://abc123",
			wantScope:   ScopeContainer,
			wantRuntime: "docker",
			wantCID:     "abc123",
		},
		{
			name:        "containerd container",
			serviceID:   "containerd://def456",
			wantScope:   ScopeContainer,
			wantRuntime: "containerd",
			wantCID:     "def456",
		},
		{
			// Malformed "://" with no runtime or ID should not produce a ScopeContainer entry.
			name:      "malformed — empty runtime and id",
			serviceID: "://",
			wantScope: ScopeHost,
		},
		{
			// Malformed: non-empty ID but empty runtime — also treated as host.
			name:      "malformed — empty runtime non-empty id",
			serviceID: "://abc123",
			wantScope: ScopeHost,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scope, runtime, cid := classifyServiceID(tc.serviceID)
			assert.Equal(t, tc.wantScope, scope)
			assert.Equal(t, tc.wantRuntime, runtime)
			assert.Equal(t, tc.wantCID, cid)
		})
	}
}

// --- Detector behaviour tests ---

func TestDetector_SupportedCheckScheduled(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	s.Schedule([]integration.Config{makeConfig("redisdb", "", true)})

	snap := d.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "redis", snap[0].Integration)
	assert.Equal(t, "redisdb", snap[0].CheckName)
	assert.Equal(t, ScopeHost, snap[0].Scope)
	assert.Empty(t, snap[0].Runtime)
	assert.Empty(t, snap[0].ContainerID)
}

func TestDetector_UnsupportedCheckNotInSnapshot(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	s.Schedule([]integration.Config{makeConfig("myapp", "", true)})

	assert.Nil(t, d.Snapshot())
}

func TestDetector_LogsOnlyConfigIgnored(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	// isCheck=false → no Instances → IsCheckConfig() returns false.
	s.Schedule([]integration.Config{makeConfig("redisdb", "", false)})

	assert.Nil(t, d.Snapshot())
}

func TestDetector_ProcessBasedConfig(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	s.Schedule([]integration.Config{makeConfig("redisdb", "process://12345", true)})

	snap := d.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, ScopeHost, snap[0].Scope)
	assert.Empty(t, snap[0].Runtime)
	assert.Empty(t, snap[0].ContainerID)
}

func TestDetector_ContainerConfig(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	s.Schedule([]integration.Config{makeConfig("redisdb", "docker://abc123", true)})

	snap := d.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, ScopeContainer, snap[0].Scope)
	assert.Equal(t, "docker", snap[0].Runtime)
	assert.Equal(t, "abc123", snap[0].ContainerID)
}

func TestDetector_TwoInstancesSameIntegration(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	// Two distinct configs — different ServiceIDs produce different digests.
	cfg1 := makeConfig("redisdb", "process://1111", true)
	cfg2 := makeConfig("redisdb", "process://2222", true)
	s.Schedule([]integration.Config{cfg1, cfg2})

	snap := d.Snapshot()
	require.Len(t, snap, 2)
	for _, ei := range snap {
		assert.Equal(t, "redis", ei.Integration)
	}
	// Verify the two entries have distinct digests — the precondition for two
	// separate map entries is that the configs produce different digests.
	assert.NotEqual(t, snap[0].Digest, snap[1].Digest)
}

func TestDetector_ScheduleThenUnschedule(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	cfg := makeConfig("redisdb", "", true)
	s.Schedule([]integration.Config{cfg})
	require.Len(t, d.Snapshot(), 1)

	s.Unschedule([]integration.Config{cfg})
	assert.Nil(t, d.Snapshot())
}

func TestDetector_DuplicateDigestDeduped(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	cfg := makeConfig("redisdb", "", true)
	s.Schedule([]integration.Config{cfg, cfg}) // same config twice

	assert.Len(t, d.Snapshot(), 1)
}

func TestDetector_StopClearsMap(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	s.Schedule([]integration.Config{makeConfig("redisdb", "", true)})
	require.Len(t, d.Snapshot(), 1)

	d.Stop()
	assert.Nil(t, d.Snapshot())
}

func TestDetector_StaleCallbackAfterStop(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	d.Stop()
	// Simulate a stale Schedule callback arriving after Stop returns.
	s.Schedule([]integration.Config{makeConfig("redisdb", "", true)})

	assert.Nil(t, d.Snapshot())
}

func TestDetector_StartIdempotent(t *testing.T) {
	t.Parallel()
	ac1 := newFakeAD()
	ac2 := newFakeAD()
	d := NewDetector()
	t.Cleanup(d.Stop)

	d.Start(ac1)
	_ = ac1.waitForScheduler(t)

	// Second Start must be a no-op; ac2 must not receive a scheduler.
	// The non-blocking select is correct here because Start is synchronous:
	// startOnce.Do is already exhausted so it returns immediately without sending.
	d.Start(ac2)
	select {
	case <-ac2.schedulerCh:
		t.Fatal("second Start registered a scheduler — Start must be idempotent")
	default:
	}
}

// TestDetector_ConcurrentStopSchedule verifies that a concurrent Stop call
// does not race with in-flight Schedule callbacks — the primary race condition
// documented in the Detector struct comment. Run with `go test -race`.
func TestDetector_ConcurrentStopSchedule(t *testing.T) {
	t.Parallel()
	cfg := makeConfig("redisdb", "", true)

	// wg.Go is available from Go 1.25 (adds a task and spawns a goroutine).
	var wg sync.WaitGroup
	for range 10 {
		// Each iteration uses a fresh detector to avoid the stopOnce guard
		// preventing re-entry in subsequent iterations.
		d := NewDetector()
		t.Cleanup(d.Stop) // ensures deregistration even if the Stop goroutine loses the race
		ac := newFakeAD()
		d.Start(ac)
		s := ac.waitForScheduler(t)

		wg.Go(func() { s.Schedule([]integration.Config{cfg}) })
		wg.Go(func() { d.Stop() })
	}
	wg.Wait()
}

// TestDetector_ConcurrentScheduleSnapshot exercises the race detector on
// concurrent Schedule, Unschedule, and Snapshot calls. No specific final state
// is asserted — the test verifies absence of data races only.
// Run with `go test -race` to fully exercise the mutex paths.
func TestDetector_ConcurrentScheduleSnapshot(t *testing.T) {
	t.Parallel()
	d, s := startDetector(t)

	cfg := makeConfig("redisdb", "", true)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { s.Schedule([]integration.Config{cfg}) })
		wg.Go(func() { s.Unschedule([]integration.Config{cfg}) })
		wg.Go(func() { _ = d.Snapshot() })
	}
	wg.Wait()
}
