// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// fakeClock is a manually-advanced clock for deterministic time-based tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (p *fakeClock) now() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.t
}

func (p *fakeClock) advance(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.t = p.t.Add(d)
}

func newTestDynamicConcurrencySettings(workers int, maxLimit int64) dynamicConcurrencySettings {
	return dynamicConcurrencySettings{
		workers:             workers,
		maxLimit:            maxLimit,
		initialLimit:        1,
		evalInterval:        15 * time.Second,
		saturationThreshold: 0.8,
		decayThreshold:      0.2,
		increaseStep:        1,
		minDecreaseInterval: time.Second,
		increaseCooldown:    15 * time.Second,
	}
}

func newTestDynamicConcurrencyDomain(t *testing.T, workers int, maxLimit int64) *dynamicConcurrencyDomain {
	policy := dynamicConcurrencyPolicy{log: logmock.New(t), settings: newTestDynamicConcurrencySettings(workers, maxLimit)}
	return policy.newDomain("test").(*dynamicConcurrencyDomain)
}

func newTestDynamicDomain(t *testing.T, maxLimit int64) (*dynamicConcurrencyDomain, *resizableSemaphore, *fakeClock) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	p := newTestDynamicConcurrencyDomain(t, 1, maxLimit)
	p.now = clock.now
	p.lastEval = clock.now()
	return p, p.sem, clock
}

func TestDynamicDomainIncreasesWhenSaturated(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 10)

	// A full window spent saturated grows the limit by one step.
	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval
	p.onEval()

	assert.Equal(t, p.settings.initialLimit+p.settings.increaseStep, p.limit)
	assert.Equal(t, p.limit, sem.limit)
}

func TestDynamicDomainReportsSaturation(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 10)

	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval / 4
	p.onEval()

	assert.Equal(t, 0.25, tlmConcurrencySaturation.WithValues(p.domain).Get())
}

func TestDynamicDomainCapsAtMaxLimit(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 3)
	p.limit = 3

	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval
	p.onEval()

	assert.Equal(t, int64(3), p.limit)
}

func TestDynamicDomainHoldsWhenNotSaturated(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 10)
	p.limit = 4

	// Saturated for only a fraction of the window (below the threshold).
	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval / 2
	p.onEval()

	assert.Equal(t, int64(4), p.limit)
}

func TestDynamicDomainDecaysWhenIdle(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 10)
	p.limit = 4

	// Saturated for less than decayThreshold of the window: step down by one.
	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval / 10
	p.onEval()
	assert.Equal(t, int64(3), p.limit)
	assert.Equal(t, int64(3), sem.limit)

	// Exactly at the threshold the limit holds.
	clock.advance(p.settings.evalInterval)
	sem.saturatedAccum = time.Duration(float64(p.settings.evalInterval) * p.settings.decayThreshold)
	p.onEval()
	assert.Equal(t, int64(3), p.limit)
}

func TestDynamicDomainDecayFloorsAtOne(t *testing.T) {
	p, _, clock := newTestDynamicDomain(t, 10)
	p.limit = 2

	for range 3 {
		clock.advance(p.settings.evalInterval)
		p.onEval()
	}
	assert.Equal(t, int64(1), p.limit)
}

func TestDynamicDomainDecayDisabled(t *testing.T) {
	p, _, clock := newTestDynamicDomain(t, 10)
	p.settings.decayThreshold = 0
	p.limit = 4

	clock.advance(p.settings.evalInterval)
	p.onEval()
	assert.Equal(t, int64(4), p.limit)
}

func TestDynamicDomainDecreasesImmediatelyAndDebounces(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 16)
	p.limit = 8

	// First backoff halves immediately.
	p.onBackoff()
	assert.Equal(t, int64(4), p.limit)
	assert.Equal(t, int64(4), sem.limit)

	// A burst within minDecreaseInterval is debounced into the same step.
	p.onBackoff()
	assert.Equal(t, int64(4), p.limit)

	// After the debounce window, a further backoff steps down again.
	clock.advance(p.settings.minDecreaseInterval + time.Millisecond)
	p.onBackoff()
	assert.Equal(t, int64(2), p.limit)
}

func TestDynamicDomainDecreaseFloorsAtOne(t *testing.T) {
	p, _, clock := newTestDynamicDomain(t, 16)
	p.limit = 1

	clock.advance(p.settings.minDecreaseInterval + time.Millisecond)
	p.onBackoff()
	assert.Equal(t, int64(1), p.limit)
}

func TestDynamicDomainSuppressesIncreaseAfterDecrease(t *testing.T) {
	p, sem, clock := newTestDynamicDomain(t, 10)
	p.limit = 4

	p.onBackoff()
	assert.Equal(t, int64(2), p.limit)

	// Within the cooldown, a fully-saturated window must not grow the limit.
	clock.advance(p.settings.increaseCooldown - time.Second)
	p.lastEval = clock.now().Add(-p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval
	p.onEval()
	assert.Equal(t, int64(2), p.limit)

	// Within the cooldown, an idle window must not decay the limit either.
	p.lastEval = clock.now().Add(-p.settings.evalInterval)
	p.onEval()
	assert.Equal(t, int64(2), p.limit)

	// Once the cooldown elapses, growth resumes.
	clock.advance(2 * time.Second)
	p.lastEval = clock.now().Add(-p.settings.evalInterval)
	sem.saturatedAccum = p.settings.evalInterval
	p.onEval()
	assert.Equal(t, int64(3), p.limit)
}

type stubRoundTripper struct {
	resp *http.Response
	err  error
}

func (s stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return s.resp, s.err
}

// timeoutError implements net.Error reporting a timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestBackoffSignalTransport(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		err        error
		wantSignal bool
	}{
		{name: "200 ok", statusCode: 200},
		{name: "408 request timeout", statusCode: http.StatusRequestTimeout, wantSignal: true},
		{name: "429 too many requests", statusCode: http.StatusTooManyRequests, wantSignal: true},
		{name: "500 not a backoff signal", statusCode: 500},
		{name: "503 service unavailable", statusCode: http.StatusServiceUnavailable, wantSignal: true},
		{name: "net timeout", err: timeoutError{}, wantSignal: true},
		{name: "deadline exceeded", err: context.DeadlineExceeded, wantSignal: true},
		{name: "context canceled (shutdown)", err: context.Canceled},
		{name: "other error", err: errors.New("connection refused")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stubResp *http.Response
			if tc.err == nil {
				stubResp = &http.Response{StatusCode: tc.statusCode, Body: http.NoBody}
			}
			signals := 0
			rt := &backoffSignalTransport{
				base:      stubRoundTripper{resp: stubResp, err: tc.err},
				onBackoff: func() { signals++ },
			}

			req, err := http.NewRequest(http.MethodPost, "http://example.com", nil)
			require.NoError(t, err)
			resp, _ := rt.RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}

			if tc.wantSignal {
				assert.Equal(t, 1, signals)
			} else {
				assert.Equal(t, 0, signals)
			}
		})
	}
}

func TestDynamicDomainSignalBackoffCoalesces(t *testing.T) {
	p, _, _ := newTestDynamicDomain(t, 10)

	// Multiple signals collapse onto the size-1 channel without blocking.
	p.signalBackoff()
	p.signalBackoff()
	p.signalBackoff()

	select {
	case <-p.backoff:
	default:
		t.Fatal("expected a pending backoff signal")
	}
	select {
	case <-p.backoff:
		t.Fatal("expected only one coalesced signal")
	default:
	}
}

func TestDynamicConcurrencySettingsFromConfig(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.num_workers", 2)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.max_concurrent_requests", 20)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.initial_limit", 4)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.eval_interval", "30s")
	mockConfig.SetInTest("forwarder.dynamic_concurrency.saturation_threshold", 0.5)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.decay_threshold", 0.1)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.increase_step", 3)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.min_decrease_interval", "2s")
	mockConfig.SetInTest("forwarder.dynamic_concurrency.increase_cooldown", "1m")

	assert.Equal(t, dynamicConcurrencySettings{
		workers:             2,
		maxLimit:            20,
		initialLimit:        4,
		evalInterval:        30 * time.Second,
		saturationThreshold: 0.5,
		decayThreshold:      0.1,
		increaseStep:        3,
		minDecreaseInterval: 2 * time.Second,
		increaseCooldown:    time.Minute,
	}, dynamicConcurrencySettingsFromConfig(mockConfig, logmock.New(t)))
}

func TestDynamicConcurrencySettingsDefaults(t *testing.T) {
	assert.Equal(t, newTestDynamicConcurrencySettings(1, 10),
		dynamicConcurrencySettingsFromConfig(configmock.New(t), logmock.New(t)))
}

func TestDynamicConcurrencySettingsFromConfigClampsInvalid(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.max_concurrent_requests", 5)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.initial_limit", 50)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.eval_interval", "0s")
	mockConfig.SetInTest("forwarder.dynamic_concurrency.saturation_threshold", 1.5)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.decay_threshold", -0.1)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.increase_step", 0)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.min_decrease_interval", "-1s")
	mockConfig.SetInTest("forwarder.dynamic_concurrency.increase_cooldown", "-1s")

	s := dynamicConcurrencySettingsFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, int64(5), s.initialLimit)
	assert.Equal(t, time.Second, s.evalInterval)
	assert.Equal(t, 1.0, s.saturationThreshold)
	assert.Equal(t, 0.0, s.decayThreshold)
	assert.Equal(t, int64(1), s.increaseStep)
	assert.Equal(t, time.Duration(0), s.minDecreaseInterval)
	assert.Equal(t, time.Duration(0), s.increaseCooldown)

	mockConfig.SetInTest("forwarder.dynamic_concurrency.initial_limit", 0)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.saturation_threshold", -0.5)
	s = dynamicConcurrencySettingsFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, int64(1), s.initialLimit)
	assert.Equal(t, 0.0, s.saturationThreshold)

	// decay_threshold must not exceed saturation_threshold.
	mockConfig.SetInTest("forwarder.dynamic_concurrency.saturation_threshold", 0.5)
	mockConfig.SetInTest("forwarder.dynamic_concurrency.decay_threshold", 0.7)
	s = dynamicConcurrencySettingsFromConfig(mockConfig, logmock.New(t))
	assert.Equal(t, 0.5, s.decayThreshold)
}
