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

func newBackoffTestCheck(t *testing.T) (*Check, *mocksender.MockSender) {
	t.Helper()
	t.Cleanup(resetTestVars)
	detectCloudProviderFn = func(_ context.Context, _ bool) (string, string) {
		return "AWS", ""
	}

	mockSender := mocksender.NewMockSender(t, CheckName)
	mockSender.On("FinalizeCheckServiceTag").Return()
	mockSender.On("Commit").Return()

	check := newCheck().(*Check)
	if err := check.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider"); err != nil {
		t.Fatalf("configure backoff test check: %v", err)
	}
	mocksender.SetSender(mockSender, check.ID())
	return check, mockSender
}

func TestHostInfoCheckMetadataBackoff(t *testing.T) {
	check, mockSender := newBackoffTestCheck(t)

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
	assertAttempts := func(expected int) {
		t.Helper()
		assert.Equal(t, expected, preemptionAttempts, "preemption attempts")
		assert.Equal(t, expected, rebalanceAttempts, "rebalance attempts")
	}
	expireBackoffWindow := func() {
		check.metadataLastFailure = check.metadataLastFailure.Add(-metadataFailureBackoff)
	}

	// Both lookups fail, but each run counts only once toward the threshold.
	for run := 1; run <= metadataFailureThreshold; run++ {
		assert.NoError(t, check.Run())
		assertAttempts(run)
		assert.Equal(t, run, check.metadataFailures)
	}

	// No lookups during backoff. A failed retry starts a new window.
	assert.NoError(t, check.Run())
	assertAttempts(metadataFailureThreshold)
	expireBackoffWindow()
	assert.NoError(t, check.Run())
	assertAttempts(metadataFailureThreshold + 1)
	assert.Equal(t, metadataFailureThreshold+1, check.metadataFailures)
	assert.NoError(t, check.Run())
	assertAttempts(metadataFailureThreshold + 1)

	// A 404 clears the active backoff and both lookups resume every run.
	lookupErr = &httputils.StatusCodeError{StatusCode: http.StatusNotFound}
	expireBackoffWindow()
	assert.NoError(t, check.Run())
	assertAttempts(metadataFailureThreshold + 2)
	assert.Zero(t, check.metadataFailures)
	assert.NoError(t, check.Run())
	assertAttempts(metadataFailureThreshold + 3)
	assert.Zero(t, check.metadataFailures)
	mockSender.AssertNumberOfCalls(t, "Event", 0)
}

func TestHostInfoCheckBackoffRecoveryEmitsNoticeOnce(t *testing.T) {
	check, mockSender := newBackoffTestCheck(t)

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
	mockSender.On("Event", mock.MatchedBy(func(ev event.Event) bool {
		return ev.Title == "Instance Preemption" && ev.EventType == PreemptionEventType
	})).Return().Once()

	for range metadataFailureThreshold {
		assert.NoError(t, check.Run())
	}

	// IMDS recovers with a notice, but the check waits for the backoff to expire.
	terminationTime = time.Now().Add(2 * time.Minute)
	terminationErr = nil
	assert.NoError(t, check.Run())
	assert.Equal(t, metadataFailureThreshold, preemptionAttempts)
	mockSender.AssertNumberOfCalls(t, "Event", 0)

	check.metadataLastFailure = check.metadataLastFailure.Add(-metadataFailureBackoff)
	assert.NoError(t, check.Run())
	assert.Zero(t, check.metadataFailures)
	mockSender.AssertNumberOfCalls(t, "Event", 1)

	// The known termination time stops further polling and duplicate events.
	assert.NoError(t, check.Run())
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

func TestHostInfoCheckHealthyLookupPreventsBackoff(t *testing.T) {
	notFound := &httputils.StatusCodeError{StatusCode: http.StatusNotFound}
	lookupErr := errors.New("metadata lookup failed")
	for _, tc := range []struct {
		name          string
		preemptionErr error
		rebalanceErr  error
	}{
		{name: "both_answer", preemptionErr: notFound, rebalanceErr: notFound},
		{name: "only_preemption_answers", preemptionErr: notFound, rebalanceErr: lookupErr},
		{name: "only_rebalance_answers", preemptionErr: lookupErr, rebalanceErr: notFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, mockSender := newBackoffTestCheck(t)
			preemptionAttempts, rebalanceAttempts := 0, 0
			getPreemptionTerminationFn = func(_ context.Context, _ string) (time.Time, error) {
				preemptionAttempts++
				return time.Time{}, tc.preemptionErr
			}
			getRebalanceRecommendationFn = func(_ context.Context, _ string) (time.Time, error) {
				rebalanceAttempts++
				return time.Time{}, tc.rebalanceErr
			}

			// A healthy answer clears a partial failure streak and keeps both lookups polling.
			check.metadataFailures = metadataFailureThreshold - 1
			runs := 2 * metadataFailureThreshold
			for range runs {
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
