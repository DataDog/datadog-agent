// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var errForwarderFull = errors.New("event platform forwarder pipeline channel is full")

type fakeEventPlatformForwarder struct {
	errors        []error
	calls         int
	blockingCalls int
}

func (f *fakeEventPlatformForwarder) SendEventPlatformEvent(_ *message.Message, _ string) error {
	call := f.calls
	f.calls++
	if call < len(f.errors) {
		return f.errors[call]
	}
	return nil
}

func (f *fakeEventPlatformForwarder) SendEventPlatformEventBlocking(_ *message.Message, _ string) error {
	f.blockingCalls++
	return errors.New("blocking send must not be called")
}

func (f *fakeEventPlatformForwarder) Purge() map[string][]*message.Message { return nil }

func TestEventSenderUsesNonBlockingForwarder(t *testing.T) {
	tests := []struct {
		name string
		send func(*eventSender) error
	}{
		{
			name: "correlation",
			send: func(sender *eventSender) error {
				return sender.send(observerdef.ActiveCorrelation{Pattern: "pattern", FirstSeen: 1})
			},
		},
		{
			name: "episode",
			send: func(sender *eventSender) error {
				return sender.sendEpisodeEvent(observerdef.CorrelatorEvent{
					Kind:           observerdef.CorrelatorEventEpisodeStarted,
					CorrelatorName: "anomaly_scorer",
					Correlation:    observerdef.ActiveCorrelation{Pattern: "episode", FirstSeen: 1},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forwarder := &fakeEventPlatformForwarder{errors: []error{errForwarderFull}}
			sender := &eventSender{forwarder: forwarder}

			err := tt.send(sender)

			assert.ErrorIs(t, err, errForwarderFull)
			assert.Equal(t, 1, forwarder.calls)
			assert.Zero(t, forwarder.blockingCalls)
		})
	}
}

func TestEventReporterRetriesCorrelationAfterNonBlockingSendFailure(t *testing.T) {
	forwarder := &fakeEventPlatformForwarder{errors: []error{errForwarderFull, nil}}
	reporter := &EventReporter{
		sender:     &eventSender{forwarder: forwarder},
		maxRetries: defaultMaxRetryAttempts,
	}
	output := reporterdef.ReportOutput{CorrelatorEvents: []observerdef.CorrelatorEvent{{
		Kind:        observerdef.CorrelatorEventCorrelationDetected,
		Correlation: observerdef.ActiveCorrelation{Pattern: "pattern", FirstSeen: 1},
	}}}

	assert.False(t, reporter.Report(output))
	assert.Len(t, reporter.retryPending, 1)
	assert.Equal(t, 1, forwarder.calls)

	assert.True(t, reporter.Report(reporterdef.ReportOutput{}))
	assert.Empty(t, reporter.retryPending)
	assert.Equal(t, 2, forwarder.calls)
	assert.Zero(t, forwarder.blockingCalls)
}

func TestEventReporterDoesNotRetryEpisodeAfterNonBlockingSendFailure(t *testing.T) {
	forwarder := &fakeEventPlatformForwarder{errors: []error{errForwarderFull}}
	reporter := &EventReporter{
		sender:     &eventSender{forwarder: forwarder},
		maxRetries: defaultMaxRetryAttempts,
	}
	output := reporterdef.ReportOutput{CorrelatorEvents: []observerdef.CorrelatorEvent{{
		Kind:           observerdef.CorrelatorEventEpisodeStarted,
		CorrelatorName: "anomaly_scorer",
		Correlation:    observerdef.ActiveCorrelation{Pattern: "episode", FirstSeen: 1},
	}}}

	assert.False(t, reporter.Report(output))
	assert.Empty(t, reporter.retryPending)
	assert.Equal(t, 1, forwarder.calls)

	assert.False(t, reporter.Report(reporterdef.ReportOutput{}))
	assert.Equal(t, 1, forwarder.calls)
	assert.Zero(t, forwarder.blockingCalls)
}
