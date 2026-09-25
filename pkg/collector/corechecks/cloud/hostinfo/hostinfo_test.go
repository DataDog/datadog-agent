// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/host"
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

	attempts := 0
	// A transient failure mode (IMDS unreachable — not one of the sentinel
	// errors that stops polling), so the check must keep retrying, but only
	// once per backoff window after repeated failures (#55269).
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		attempts++
		return time.Time{}, errors.New("connection timed out")
	}
	// Healthy no-notice 404 for rebalance so only the preemption failures
	// advance the shared counter in this test.
	rebalanceNotFound := &httputils.StatusCodeError{StatusCode: 404, Method: "GET", URL: "http://169.254.169.254/latest/meta-data/events/recommendations/rebalance"}
	getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
		return time.Time{}, rebalanceNotFound
	}
	uptime = uptimeSampler

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")
	mocksender.SetSender(mockSender, check.ID())

	// Simulate the default 15s check interval: four consecutive runs hit the
	// lookup, then the backoff window must skip it.
	for i := 0; i < 4; i++ {
		check.Run()
	}
	assert.Equal(t, 4, attempts)
	assert.Equal(t, 4, check.metadataFailures)

	// The backoff window is 60s; without advancing the clock, further runs
	// must not reach the lookup.
	check.Run()
	check.Run()
	assert.Equal(t, 4, attempts)

	// After the window elapses, the lookup is attempted again.
	check.metadataLastAttempt = check.metadataLastAttempt.Add(-metadataFailureBackoff)
	check.Run()
	assert.Equal(t, 5, attempts)

	// A later success resets the failure count so every run polls again.
	// (A past termination time is a success path that stores no event.)
	getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
		attempts++
		return time.Now().Add(-time.Minute), nil
	}
	check.metadataFailures = metadataFailureThreshold - 1
	check.metadataLastAttempt = time.Now().Add(-metadataFailureBackoff)
	check.Run()
	assert.Equal(t, 0, check.metadataFailures)
}

func TestMetadataLookupFailureClassification(t *testing.T) {
	// The healthy no-active-notice response is a 404 from IMDS and must not
	// count toward the backoff; every other failure (timeout, connection
	// refused, 5xx) must.
	assert.True(t, metadataLookupFailure(nil) == false)
	assert.False(t, metadataLookupFailure(&httputils.StatusCodeError{StatusCode: 404, Method: "GET", URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}))
	assert.True(t, metadataLookupFailure(&httputils.StatusCodeError{StatusCode: 503, Method: "GET", URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}))
	assert.True(t, metadataLookupFailure(errors.New("connection timed out")))
	// Wrapped StatusCodeError still classifies by status code.
	assert.False(t, metadataLookupFailure(fmt.Errorf("unable to retrieve spot instance-action from IMDS: %w", &httputils.StatusCodeError{StatusCode: 404, Method: "GET", URL: "u"})))
}

func TestHostInfoCheckHealthy404DoesNotBackOff(t *testing.T) {
	defer resetTestVars()

	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	attempts := 0
	// The healthy steady state on a spot instance without an active notice:
	// the termination endpoint answers 404 every time.
	notFound := &httputils.StatusCodeError{StatusCode: 404, Method: "GET", URL: "http://169.254.169.254/latest/meta-data/spot/instance-action"}
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
