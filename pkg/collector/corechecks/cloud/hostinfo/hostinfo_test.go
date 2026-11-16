// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func uptimeSampler() (uint64, error) {
	return 555, nil
}

func resetTestVars() {
	detectCloudProviderFn = cloudproviders.DetectCloudProvider
	getPreemptionTerminationFn = cloudproviders.GetPreemptionTerminationTime
	getRebalanceRecommendationFn = cloudproviders.GetRebalanceRecommendationTime
	uptime = host.Uptime
}

func TestHostInfoCheckNoCloudProvider(t *testing.T) {
	defer resetTestVars()

	// Mock cloud provider detection to return empty (no cloud provider)
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "", ""
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	// No Event should be sent when no cloud provider is detected
	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostInfoCheckWithPreemptionTermination(t *testing.T) {
	defer resetTestVars()

	terminationTime := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)

	// Mock cloud provider detection to return AWS
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// Mock preemption termination to return a scheduled termination
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return terminationTime, nil
	}

	// Mock uptime
	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.Title == "Instance Preemption" &&
			ev.AlertType == event.AlertTypeInfo &&
			ev.EventType == PreemptionEventType
	})).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostInfoCheckNoPreemptionScheduled(t *testing.T) {
	defer resetTestVars()

	// Mock cloud provider detection to return AWS
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// Mock preemption termination to return no termination scheduled
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no preemption scheduled")
	}

	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no rebalance recommendation")
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	// No Event should be sent
	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostInfoCheckPreemptionEventSentOnlyOnce(t *testing.T) {
	defer resetTestVars()

	terminationTime := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)

	// Mock cloud provider detection to return AWS
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// Mock preemption termination to return a scheduled termination
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return terminationTime, nil
	}

	// Mock uptime
	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	// Event should only be sent once
	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.Title == "Instance Preemption" &&
			ev.AlertType == event.AlertTypeInfo &&
			ev.EventType == PreemptionEventType
	})).Return().Times(1)
	mockSender.On("Commit").Return()

	// Run the check twice
	check.Run()
	check.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 2)
}

func TestHostInfoCheckNotPreemptibleStopsPolling(t *testing.T) {
	defer resetTestVars()

	callCount := 0

	// Mock cloud provider detection to return AWS
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// Mock preemption termination to return ErrNotPreemptible
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		callCount++
		return time.Time{}, cloudproviders.ErrNotPreemptible
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Commit").Return()

	// Run the check three times
	check.Run()
	check.Run()
	check.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 3)

	// Preemption function should only be called once, then polling stops
	if callCount != 1 {
		t.Errorf("expected getPreemptionTerminationFn to be called 1 time, got %d", callCount)
	}
}

func TestHostInfoCheckPreemptionUnsupportedStopsPolling(t *testing.T) {
	defer resetTestVars()

	callCount := 0

	// Mock cloud provider detection to return an unsupported provider
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "UnsupportedCloud", ""
	}

	// Mock preemption termination to return ErrPreemptionUnsupported
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		callCount++
		return time.Time{}, cloudproviders.ErrPreemptionUnsupported
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Commit").Return()

	// Run the check three times
	check.Run()
	check.Run()
	check.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 3)

	// Preemption function should only be called once, then polling stops
	if callCount != 1 {
		t.Errorf("expected getPreemptionTerminationFn to be called 1 time, got %d", callCount)
	}
}

func TestHostInfoCheckWithRebalanceRecommendation(t *testing.T) {
	defer resetTestVars()

	noticeTime := time.Date(2020, 10, 27, 8, 22, 0, 0, time.UTC)

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// No termination scheduled
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no preemption scheduled")
	}

	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return noticeTime, nil
	}

	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.Title == "Elevated risk of Instance Preemption" &&
			ev.AlertType == event.AlertTypeInfo &&
			ev.EventType == PreemptionRiskEventType
	})).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostInfoCheckRebalanceEventSentOnlyOnce(t *testing.T) {
	defer resetTestVars()

	noticeTime := time.Date(2020, 10, 27, 8, 22, 0, 0, time.UTC)

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no preemption scheduled")
	}

	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return noticeTime, nil
	}

	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.EventType == PreemptionRiskEventType
	})).Return().Times(1)
	mockSender.On("Commit").Return()

	// Run the check twice — event should only be sent once
	check.Run()
	check.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 2)
}

func TestHostInfoCheckRebalanceSkippedWhenTerminationSet(t *testing.T) {
	defer resetTestVars()

	terminationTime := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return terminationTime, nil
	}

	rebalanceCalled := false
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		rebalanceCalled = true
		return time.Date(2020, 10, 27, 8, 22, 0, 0, time.UTC), nil
	}

	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	// Only preemption event should be sent, not rebalance
	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.EventType == PreemptionEventType
	})).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)

	if rebalanceCalled {
		t.Error("rebalance recommendation should not be checked when termination is already scheduled")
	}
}

func TestHostInfoCheckNoRebalanceRecommendation(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no preemption scheduled")
	}

	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("no rebalance recommendation")
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mocksender.SetSender(mockSender, check.ID())

	mockSender.On("Commit").Return().Times(1)

	check.Run()
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostInfoCheckPreemptionFailureBackoff(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// A transient failure mode (IMDS unreachable — not one of the sentinel
	// errors that stops polling), so the check must keep retrying, but only
	// once per backoff window after repeated failures (#55269). Both lookups
	// fail, so the shared counter advances.
	lookupErr := errors.New("connection timed out")
	preemptionAttempts, rebalanceAttempts := 0, 0
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		preemptionAttempts++
		return time.Time{}, lookupErr
	}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		rebalanceAttempts++
		return time.Time{}, lookupErr
	}
	uptime = uptimeSampler
	assertAttempts := func(expected int) {
		t.Helper()
		assert.Equal(t, expected, preemptionAttempts, "preemption attempts")
		assert.Equal(t, expected, rebalanceAttempts, "rebalance attempts")
	}
	expireBackoffWindow := func(check *Check) {
		check.metadataLastFailure = check.metadataLastFailure.Add(-metadataFailureBackoff)
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// Four consecutive failed runs reach both lookups and arm the backoff.
	for i := 0; i < metadataFailureThreshold; i++ {
		check.Run()
	}
	assertAttempts(metadataFailureThreshold)
	assert.Equal(t, metadataFailureThreshold, check.metadataFailures)

	// The backoff window is 60s; without advancing the clock, further runs
	// must not reach either lookup.
	check.Run()
	check.Run()
	assertAttempts(metadataFailureThreshold)

	// After the window elapses, both lookups are retried once. The retry
	// fails, so the backoff stays armed and a new full window starts.
	expireBackoffWindow(check)
	check.Run()
	assertAttempts(metadataFailureThreshold + 1)
	assert.Equal(t, metadataFailureThreshold+1, check.metadataFailures)
	check.Run()
	assertAttempts(metadataFailureThreshold + 1)

	// IMDS recovers to the healthy no-active-notice steady state. The next
	// retry after the window gets a 404, which clears the armed backoff.
	lookupErr = &httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "u"}
	expireBackoffWindow(check)
	check.Run()
	assertAttempts(metadataFailureThreshold + 2)
	assert.Zero(t, check.metadataFailures)

	// From then on, every ordinary run polls both lookups again, without
	// touching the clock.
	check.Run()
	check.Run()
	assertAttempts(metadataFailureThreshold + 4)
	assert.Zero(t, check.metadataFailures)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
}

func TestHostInfoCheckBackoffRecoveryEmitsNoticeOnce(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// IMDS is unreachable until the termination notice shows up.
	var terminationTime time.Time
	terminationErr := errors.New("connection timed out")
	preemptionAttempts := 0
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		preemptionAttempts++
		return terminationTime, terminationErr
	}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("connection timed out")
	}
	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()
	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.Title == "Instance Preemption" && ev.EventType == PreemptionEventType
	})).Return().Once()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// Arm the backoff.
	for i := 0; i < metadataFailureThreshold; i++ {
		check.Run()
	}
	assert.Equal(t, metadataFailureThreshold, check.metadataFailures)

	// IMDS recovers with an active termination notice, but the backoff is
	// still armed: the notice must wait for the window to elapse.
	terminationTime = time.Now().Add(2 * time.Minute)
	terminationErr = nil
	check.Run()
	assert.Equal(t, metadataFailureThreshold, preemptionAttempts)
	mockSender.AssertNumberOfCalls(t, "Event", 0)

	// Once the window elapses, the retry picks up the notice, emits exactly
	// one event and clears the backoff.
	check.metadataLastFailure = check.metadataLastFailure.Add(-metadataFailureBackoff)
	check.Run()
	assert.Equal(t, metadataFailureThreshold+1, preemptionAttempts)
	assert.Zero(t, check.metadataFailures)
	mockSender.AssertNumberOfCalls(t, "Event", 1)

	// The known termination time stops further polling and duplicate events.
	check.Run()
	check.Run()
	assert.Equal(t, metadataFailureThreshold+1, preemptionAttempts)
	mockSender.AssertNumberOfCalls(t, "Event", 1)
	mockSender.AssertExpectations(t)
}

func TestMetadataLookupFailureClassification(t *testing.T) {
	// The healthy no-active-notice response is a 404 from IMDS and must not
	// count toward the backoff; every other failure (timeout, connection
	// refused, 5xx) must.
	assert.False(t, metadataLookupFailure(nil))
	assert.False(t, metadataLookupFailure(&httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}))
	assert.True(t, metadataLookupFailure(&httputils.StatusCodeError{StatusCode: http.StatusServiceUnavailable, Method: http.MethodGet, URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}))
	assert.True(t, metadataLookupFailure(errors.New("connection timed out")))
	// Wrapped StatusCodeError still classifies by status code.
	assert.False(t, metadataLookupFailure(fmt.Errorf("unable to retrieve spot instance-action from IMDS: %w", &httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "u"})))
}

func TestHostInfoCheckHealthy404DoesNotBackOff(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	attempts := 0
	// The healthy steady state on a spot instance without an active notice:
	// the termination endpoint answers 404 every time.
	notFound := &httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		attempts++
		return time.Time{}, notFound
	}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, notFound
	}
	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// Far more runs than the backoff threshold: 404s must never arm it.
	for i := 0; i < 10; i++ {
		check.Run()
	}
	assert.Equal(t, 10, attempts)
	assert.Equal(t, 0, check.metadataFailures)
}

func TestHostInfoCheckHealthy404ResetsStaleFailures(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("connection timed out")
	}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, errors.New("connection timed out")
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// Build up stale failures, below the backoff threshold.
	check.Run()
	check.Run()
	assert.Equal(t, 2, check.metadataFailures)

	// IMDS recovers to the healthy no-active-notice steady state: a 404
	// must clear the stale failures, not just skip the increment, or they
	// would linger forever on a spot instance that only ever 404s.
	notFound := &httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "u"}
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, notFound
	}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, notFound
	}
	check.Run()
	assert.Equal(t, 0, check.metadataFailures)
}

func TestHostInfoCheckHealthyLookupPreventsBackoff(t *testing.T) {
	notFound := &httputils.StatusCodeError{StatusCode: http.StatusNotFound, Method: http.MethodGet, URL: "u"}
	for _, failure := range []struct {
		name string
		err  error
	}{
		{name: "server_error", err: &httputils.StatusCodeError{StatusCode: http.StatusServiceUnavailable, Method: http.MethodGet, URL: "u"}},
		{name: "generic_error", err: errors.New("metadata lookup failed")},
	} {
		for _, healthyLookup := range []struct {
			name          string
			preemptionErr error
			rebalanceErr  error
		}{
			{name: "preemption", preemptionErr: notFound, rebalanceErr: failure.err},
			{name: "rebalance", preemptionErr: failure.err, rebalanceErr: notFound},
		} {
			t.Run(healthyLookup.name+"/"+failure.name, func(t *testing.T) {
				defer resetTestVars()

				detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
					return "AWS", ""
				}

				preemptionAttempts, rebalanceAttempts := 0, 0
				getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
					preemptionAttempts++
					return time.Time{}, healthyLookup.preemptionErr
				}
				getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
					rebalanceAttempts++
					return time.Time{}, healthyLookup.rebalanceErr
				}

				mockSender := mocksender.NewMockSender(t, CheckName)
				mockSender.On("FinalizeCheckServiceTag").Return()
				mockSender.On("Commit").Return()

				check := newCheck().(*Check)
				check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
				mocksender.SetSender(mockSender, check.ID())

				// Either healthy endpoint must clear stale failures even when
				// the other fails, and keep both lookups polling on every run.
				check.metadataFailures = metadataFailureThreshold - 1
				runs := 2 * metadataFailureThreshold
				for i := 0; i < runs; i++ {
					assert.NoError(t, check.Run())
					assert.Zero(t, check.metadataFailures)
				}
				assert.Equal(t, runs, preemptionAttempts)
				assert.Equal(t, runs, rebalanceAttempts)
				mockSender.AssertNumberOfCalls(t, "Commit", runs)
				mockSender.AssertNumberOfCalls(t, "Event", 0)
			})
		}
	}
}

func TestHostInfoCheckBothLookupsFailingCountsOncePerRun(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	// A full IMDS outage (e.g. a hop-limited container): both the
	// preemption and rebalance lookups hit the same unreachable endpoint
	// and fail on every run.
	preemptionAttempts := 0
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		preemptionAttempts++
		return time.Time{}, errors.New("connection timed out")
	}
	rebalanceAttempts := 0
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		rebalanceAttempts++
		return time.Time{}, errors.New("connection timed out")
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// The two lookups share metadataFailures; a failing run must count as
	// a single failure, so the backoff still takes metadataFailureThreshold
	// runs to arm, not half that because both lookups failed.
	for i := 0; i < metadataFailureThreshold; i++ {
		check.Run()
	}
	assert.Equal(t, metadataFailureThreshold, preemptionAttempts)
	assert.Equal(t, metadataFailureThreshold, rebalanceAttempts)
	assert.Equal(t, metadataFailureThreshold, check.metadataFailures)

	// The backoff is now armed: neither lookup should be attempted again.
	check.Run()
	assert.Equal(t, metadataFailureThreshold, preemptionAttempts)
	assert.Equal(t, metadataFailureThreshold, rebalanceAttempts)
}

func TestUpdateMetadataBackoff(t *testing.T) {
	for _, tc := range []struct {
		name             string
		outcomes         []metadataOutcome
		expectedFailures int
		expectStamped    bool
	}{
		{name: "none_attempted", outcomes: []metadataOutcome{metadataNotAttempted, metadataNotAttempted}, expectedFailures: 2},
		{name: "one_failed_other_skipped", outcomes: []metadataOutcome{metadataFailed, metadataNotAttempted}, expectedFailures: 3, expectStamped: true},
		{name: "both_failed_counts_once", outcomes: []metadataOutcome{metadataFailed, metadataFailed}, expectedFailures: 3, expectStamped: true},
		{name: "answered_then_failed_resets", outcomes: []metadataOutcome{metadataAnswered, metadataFailed}, expectedFailures: 0},
		{name: "failed_then_answered_resets", outcomes: []metadataOutcome{metadataFailed, metadataAnswered}, expectedFailures: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lastFailure := time.Now().Add(-time.Hour)
			check := &Check{metadataFailures: 2, metadataLastFailure: lastFailure}

			check.updateMetadataBackoff(tc.outcomes...)

			assert.Equal(t, tc.expectedFailures, check.metadataFailures)
			if tc.expectStamped {
				assert.True(t, check.metadataLastFailure.After(lastFailure))
			} else {
				assert.Equal(t, lastFailure, check.metadataLastFailure)
			}
		})
	}
}
