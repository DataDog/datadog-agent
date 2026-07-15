// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	adtelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// fakeDiscoverer is a ConfigDiscoverer stub whose DiscoverConfig is supplied
// per-test. It counts calls so tests can assert retry semantics.
type fakeDiscoverer struct {
	mu    sync.Mutex
	calls int
	fn    func(integrationName, serviceJSON string) (string, error)
}

func (f *fakeDiscoverer) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.fn(integrationName, serviceJSON)
}

func (f *fakeDiscoverer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// recordingCallback is a ResultCallback that records every invocation so
// tests can assert what configs the worker emitted.
type recordingCallback struct {
	mu      sync.Mutex
	results []recordedResult
}

type recordedResult struct {
	svcID     string
	tplDigest string
	configs   []integration.Config
}

func (r *recordingCallback) callback(svcID, tplDigest string, configs []integration.Config) {
	r.mu.Lock()
	r.results = append(r.results, recordedResult{svcID, tplDigest, configs})
	r.mu.Unlock()
}

func (r *recordingCallback) snapshot() []recordedResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedResult, len(r.results))
	copy(out, r.results)
	return out
}

const testTplDigest = "test-tpl-digest"

func newSvc(id string) ServiceInfo {
	return &fakeService{
		id:    id,
		hosts: map[string]string{"main": "10.0.0.1"},
		ports: []servicePort{{Name: "http", Port: 8080}},
	}
}

func newTestTelemetryStore(t testing.TB) *adtelemetry.Store {
	t.Helper()
	telemetryComp := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	return adtelemetry.NewStore(telemetryComp)
}

// TestWorker_Success_TriggersCallbackOnce: a probe that returns valid JSON
// fires onResult exactly once and does not retry.
func TestWorker_Success_TriggersCallbackOnce(t *testing.T) {
	const svcID = "docker://svc-1"
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		return `[{"instances":[{"host":"x"}]}]`, nil
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{svcID: newSvc(svcID)}}
	cb := &recordingCallback{}
	telStore := newTestTelemetryStore(t)

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 3, RetryDelay: 10 * time.Millisecond}, telStore)
	w.Enqueue(svcID, testTplDigest, "myinteg")
	assert.Equal(t, float64(1), telStore.DiscoveryQueueDepth.WithValues("myinteg", "docker").Get())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.runOnce(ctx)

	require.Eventually(t, func() bool { return len(cb.snapshot()) == 1 }, time.Second, 5*time.Millisecond)
	got := cb.snapshot()[0]
	assert.Equal(t, svcID, got.svcID)
	assert.Equal(t, testTplDigest, got.tplDigest)
	require.Len(t, got.configs, 1)
	assert.Equal(t, "myinteg", got.configs[0].Name)
	assert.Equal(t, 1, disco.callCount())
	assert.Equal(t, float64(0), telStore.DiscoveryQueueDepth.WithValues("myinteg", "docker").Get())
	assert.Equal(t, float64(1), telStore.DiscoveryResults.WithValues("myinteg", "success", "docker").Get())
}

// TestWorker_RetriesUpToMax: a probe that always returns an error retries
// until maxAttempts is reached, then gives up — onResult is never fired.
func TestWorker_RetriesUpToMax(t *testing.T) {
	const svcID = "containerd://svc-1"
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		return "", errors.New("nope")
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{svcID: newSvc(svcID)}}
	cb := &recordingCallback{}
	telStore := newTestTelemetryStore(t)

	const max = 4
	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: max, RetryDelay: 5 * time.Millisecond}, telStore)
	w.Start()
	defer w.Stop()
	w.Enqueue(svcID, testTplDigest, "myinteg")
	assert.Equal(t, float64(1), telStore.DiscoveryQueueDepth.WithValues("myinteg", "containerd").Get())

	require.Eventually(t, func() bool { return disco.callCount() >= max }, time.Second, 2*time.Millisecond)
	// Give one extra retry window to confirm there's no (max+1)th attempt.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, max, disco.callCount(), "worker should give up after max attempts")
	assert.Empty(t, cb.snapshot(), "onResult should never fire when discovery fails")
	assert.Equal(t, float64(0), telStore.DiscoveryQueueDepth.WithValues("myinteg", "containerd").Get())
	assert.Equal(t, float64(1), telStore.DiscoveryResults.WithValues("myinteg", "max_attempts_exceeded", "containerd").Get())
}

// TestWorker_PermFail_DropsImmediately: a PermFail error drops the job after
// the first attempt without consuming any retry budget.
func TestWorker_PermFail_DropsImmediately(t *testing.T) {
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		return "", PermFail{Err: errors.New("permanent")}
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": newSvc("svc-1")}}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 10, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	w.Enqueue("svc-1", testTplDigest, "myinteg")

	require.Eventually(t, func() bool { return disco.callCount() >= 1 }, time.Second, 2*time.Millisecond)
	assert.Equal(t, 1, disco.callCount(), "PermFail must not trigger a retry")
	assert.Empty(t, cb.snapshot())
}

// TestWorker_RetriesOnEmptyResult: an empty-array result is treated the same
// as an error and retried.
func TestWorker_RetriesOnEmptyResult(t *testing.T) {
	attempts := atomic.NewInt32(0)
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		n := attempts.Add(1)
		if n < 3 {
			return `[]`, nil
		}
		return `[{"instances":[{}]}]`, nil
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": newSvc("svc-1")}}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 5, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	w.Enqueue("svc-1", testTplDigest, "myinteg")

	require.Eventually(t, func() bool { return len(cb.snapshot()) == 1 }, time.Second, 2*time.Millisecond)
	assert.GreaterOrEqual(t, int(attempts.Load()), 3, "worker must have retried until the integration returned configs")
}

// TestWorker_ServiceRemovedBetweenRetries: once the caller removes the
// service from the lookup, the worker drops the job on the next pop and
// the failed-then-retried call doesn't fire again. Service removal is the
// worker's only cancellation primitive — no separate Forget API.
func TestWorker_ServiceRemovedBetweenRetries(t *testing.T) {
	// Block the discoverer mid-call so the test controls when the first
	// probe completes, giving us a deterministic window to remove the
	// service before the retry pop.
	release := make(chan struct{})
	called := atomic.NewInt32(0)
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		called.Add(1)
		<-release
		return "", errors.New("released")
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": newSvc("svc-1")}}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 5, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	w.Enqueue("svc-1", testTplDigest, "myinteg")
	// Wait until the first DiscoverConfig call is in flight.
	require.Eventually(t, func() bool { return called.Load() == 1 }, time.Second, 2*time.Millisecond)

	// Simulate processDelService removing the service from activeServices.
	lookup.remove("svc-1")
	close(release)

	// Wait long enough that any retry would have run.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), called.Load(), "removing the service must stop further probes")
	assert.Empty(t, cb.snapshot())
}

// TestWorker_DropsWhenServiceGone: if the service has disappeared from the
// lookup, the worker drops the job without calling DiscoverConfig at all.
func TestWorker_DropsWhenServiceGone(t *testing.T) {
	const svcID = "process://svc-gone"
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		return "", errors.New("should not be called")
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{}} // empty
	cb := &recordingCallback{}
	telStore := newTestTelemetryStore(t)

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 3, RetryDelay: 5 * time.Millisecond}, telStore)
	w.Enqueue(svcID, testTplDigest, "myinteg")
	assert.Equal(t, float64(1), telStore.DiscoveryQueueDepth.WithValues("myinteg", "process").Get())

	w.Start()
	defer w.Stop()

	require.Eventually(t, func() bool {
		return telStore.DiscoveryQueueDepth.WithValues("myinteg", "process").Get() == float64(0)
	}, time.Second, 2*time.Millisecond, "worker must drain queue depth")
	assert.Zero(t, disco.callCount())
	assert.Empty(t, cb.snapshot())
	assert.Equal(t, float64(1), telStore.DiscoveryResults.WithValues("myinteg", "service_not_found", "process").Get())
}

// TestWorker_NoHost_TriggersRetry: a service whose GetHosts returns nothing
// usable is treated as a transient failure (typical container-startup race)
// and retried.
func TestWorker_NoHost_TriggersRetry(t *testing.T) {
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		return "", errors.New("should not be reached")
	}}
	noHost := &fakeService{id: "svc-1", hosts: map[string]string{}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": noHost}}
	cb := &recordingCallback{}

	const max = 3
	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: max, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	w.Enqueue("svc-1", testTplDigest, "myinteg")

	// We never expect DiscoverConfig to be called (no host) but the worker
	// should walk through its retry budget anyway.
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, disco.callCount())
	assert.Empty(t, cb.snapshot())
}

// TestWorker_NoBookkeepingLeakAfterServiceRemoval confirms there are no
// unbounded-growth maps inside the worker: after a service is removed and
// its pending job drains, the attempts map drops the entry. With this and
// the workqueue's own dedupe, the only state the worker holds is bounded
// by the number of currently-retrying jobs.
func TestWorker_NoBookkeepingLeakAfterServiceRemoval(t *testing.T) {
	called := atomic.NewInt32(0)
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		called.Add(1)
		return "", errors.New("fail")
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": newSvc("svc-1")}}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{MaxAttempts: 5, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()

	w.Enqueue("svc-1", testTplDigest, "myinteg")
	// One failure populates an attempts entry.
	require.Eventually(t, func() bool { return called.Load() >= 1 }, time.Second, 2*time.Millisecond)

	// Remove the service. The next retry pop will see ServiceLookup return
	// false and call dropAttempts, clearing the entry.
	lookup.remove("svc-1")

	require.Eventually(t, func() bool {
		w.m.Lock()
		defer w.m.Unlock()
		return len(w.attempts) == 0
	}, time.Second, 2*time.Millisecond, "attempts map must be reaped after the service is removed")
}

// TestWorker_ParallelProbes_DifferentServices verifies that the worker pool
// processes probes for different services in parallel.
func TestWorker_ParallelProbes_DifferentServices(t *testing.T) {
	const workers = 4

	release := make(chan struct{})
	inFlight := atomic.NewInt32(0)
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		inFlight.Add(1)
		// This will block all of the workers from processing the next job.
		<-release
		inFlight.Add(-1)
		return `[{"instances":[{}]}]`, nil
	}}

	services := map[string]ServiceInfo{}
	for i := range workers {
		id := fmt.Sprintf("svc-%d", i)
		services[id] = newSvc(id)
	}
	lookup := &fixedLookup{services: services}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{Workers: workers, MaxAttempts: 1, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	for id := range services {
		w.Enqueue(id, testTplDigest, "myinteg")
	}

	require.Eventually(t, func() bool { return inFlight.Load() == workers }, time.Second, 50*time.Millisecond,
		"expected %d concurrent probes, got %d", workers, inFlight.Load())
	close(release)
	require.Eventually(t, func() bool { return inFlight.Load() == 0 }, time.Second, 50*time.Millisecond)
}

// TestWorker_SameKeyStillSerial verifies that the workqueue's per-key
// dedupe survives the move to a worker pool: enqueueing the same key many
// times while a probe is in flight never yields more than one concurrent
// probe for that key.
func TestWorker_SameKeyStillSerial(t *testing.T) {
	release := make(chan struct{})
	inFlight := atomic.NewInt32(0)
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) {
		inFlight.Add(1)
		<-release
		inFlight.Add(-1)
		return "", errors.New("fail")
	}}
	lookup := &fixedLookup{services: map[string]ServiceInfo{"svc-1": newSvc("svc-1")}}
	cb := &recordingCallback{}

	w := NewWorker(disco, lookup, cb.callback, Config{Workers: 4, MaxAttempts: 10, RetryDelay: 5 * time.Millisecond}, nil)
	w.Start()
	defer w.Stop()
	for range 20 {
		w.Enqueue("svc-1", testTplDigest, "myinteg")
	}

	require.Eventually(t, func() bool { return inFlight.Load() == 1 }, time.Second, 50*time.Millisecond)
	require.Never(t, func() bool { return inFlight.Load() != 1 }, 250*time.Millisecond, 25*time.Millisecond)
	close(release)
	require.Eventually(t, func() bool { return inFlight.Load() == 0 }, time.Second, 50*time.Millisecond)
}

// TestWorker_StopIsIdempotent: extra Stop calls do not panic and the worker
// can be Start/Stop cycled safely (matches AutoConfig's lifecycle).
func TestWorker_StopIsIdempotent(_ *testing.T) {
	disco := &fakeDiscoverer{fn: func(_, _ string) (string, error) { return `[]`, nil }}
	lookup := &fixedLookup{}
	w := NewWorker(disco, lookup, func(string, string, []integration.Config) {}, Config{}, nil)
	w.Start()
	w.Stop()
	w.Stop() // second Stop must be safe
}
