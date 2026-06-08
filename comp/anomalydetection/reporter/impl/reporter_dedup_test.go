// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// stubForwarder records how many times it was called. It always succeeds.
type stubForwarder struct{ calls int }

func (f *stubForwarder) SendEventPlatformEvent(_ *message.Message, _ string) error {
	f.calls++
	return nil
}
func (f *stubForwarder) SendEventPlatformEventBlocking(_ *message.Message, _ string) error {
	f.calls++
	return nil
}
func (f *stubForwarder) Purge() map[string][]*message.Message { return nil }

// failingForwarder always returns an error.
type failingForwarder struct{}

func (f *failingForwarder) SendEventPlatformEvent(_ *message.Message, _ string) error {
	return assert.AnError
}
func (f *failingForwarder) SendEventPlatformEventBlocking(_ *message.Message, _ string) error {
	return assert.AnError
}
func (f *failingForwarder) Purge() map[string][]*message.Message { return nil }

// newTestEventReporter builds an EventReporter wired to a stub forwarder.
func newTestEventReporter(t *testing.T) (*EventReporter, *stubForwarder) {
	t.Helper()
	fwd := &stubForwarder{}
	sender, err := newEventSender(fwd, logmock.New(t), nil, nil)
	require.NoError(t, err)
	return &EventReporter{sender: sender, logger: logmock.New(t)}, fwd
}

// correlation builds a minimal ActiveCorrelation.
func correlation(pattern string, lastUpdated int64) observerdef.ActiveCorrelation {
	return observerdef.ActiveCorrelation{
		Pattern:     pattern,
		Title:       "test: " + pattern,
		LastUpdated: lastUpdated,
	}
}

// output builds a ReportOutput carrying the given correlations as NewCorrelations.
func output(corrs ...observerdef.ActiveCorrelation) reporterdef.ReportOutput {
	return reporterdef.ReportOutput{NewCorrelations: corrs}
}

// --- EventReporter tests ---

// TestEventReporter_SendsForEachNewCorrelation verifies that the reporter fires
// one send per entry in NewCorrelations.
func TestEventReporter_SendsForEachNewCorrelation(t *testing.T) {
	r, fwd := newTestEventReporter(t)
	r.Report(output(correlation("A", 100), correlation("B", 100)))
	assert.Equal(t, 2, fwd.calls)
}

// TestEventReporter_SilentWhenNoNewCorrelations verifies an advance cycle with
// no new correlations produces no sends.
func TestEventReporter_SilentWhenNoNewCorrelations(t *testing.T) {
	r, fwd := newTestEventReporter(t)
	r.Report(reporterdef.ReportOutput{})
	assert.Equal(t, 0, fwd.calls)
}

// TestEventReporter_RetryQueuedOnFailure verifies that a correlation whose
// send() fails is held in retryQ and retried on the next advance.
func TestEventReporter_RetryQueuedOnFailure(t *testing.T) {
	fwd := &failingForwarder{}
	sender, err := newEventSender(fwd, logmock.New(t), nil, nil)
	require.NoError(t, err)
	r := &EventReporter{sender: sender, logger: logmock.New(t)}

	// Cycle 1: send fails → pattern lands in retryQ.
	r.Report(output(correlation("A", 100)))
	assert.Len(t, r.retryQ, 1, "failed pattern must be queued for retry")

	// Cycle 2: engine sends nothing new, but the retry should attempt again
	// (and fail again since forwarder is still broken).
	r.Report(reporterdef.ReportOutput{})
	assert.Len(t, r.retryQ, 1, "still queued after second failure")
}

// TestEventReporter_RetrySucceedsAndClearsQueue verifies that once the
// forwarder recovers, the retryQ drains.
func TestEventReporter_RetrySucceedsAndClearsQueue(t *testing.T) {
	// Start with a failing forwarder.
	failFwd := &failingForwarder{}
	sender, err := newEventSender(failFwd, logmock.New(t), nil, nil)
	require.NoError(t, err)
	r := &EventReporter{sender: sender, logger: logmock.New(t)}

	r.Report(output(correlation("A", 100)))
	assert.Len(t, r.retryQ, 1)

	// Switch to a working forwarder by replacing sender.
	okFwd := &stubForwarder{}
	sender2, err := newEventSender(okFwd, logmock.New(t), nil, nil)
	require.NoError(t, err)
	r.sender = sender2

	// Next advance (no new correlations) — retry drains the queue.
	r.Report(reporterdef.ReportOutput{})
	assert.Empty(t, r.retryQ, "retryQ must be empty after successful send")
	assert.Equal(t, 1, okFwd.calls)
}

// TestEventReporter_NewCorrelationReplacesRetryEntry verifies that if the
// engine provides a newer version of a pattern that is already in retryQ,
// the newer entry replaces the stale one.
func TestEventReporter_NewCorrelationReplacesRetryEntry(t *testing.T) {
	failFwd := &failingForwarder{}
	sender, err := newEventSender(failFwd, logmock.New(t), nil, nil)
	require.NoError(t, err)
	r := &EventReporter{sender: sender, logger: logmock.New(t)}

	// Cycle 1: A fails → retryQ["A"] = {LastUpdated:100}.
	r.Report(output(correlation("A", 100)))
	require.Len(t, r.retryQ, 1)

	// Cycle 2: engine offers A with a newer timestamp (genuinely recurred).
	// The newer version should replace the stale retryQ entry.
	r.Report(output(correlation("A", 200)))
	require.Len(t, r.retryQ, 1)
	assert.Equal(t, int64(200), r.retryQ["A"].LastUpdated,
		"retryQ entry must be updated to the newer version")
}

// --- stdoutReporter tests ---

// captureStdout redirects os.Stdout for the duration of fn and returns the output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	rp, wp, err := os.Pipe()
	require.NoError(t, err)
	prev := os.Stdout
	os.Stdout = wp
	fn()
	wp.Close()
	os.Stdout = prev
	out, err := io.ReadAll(rp)
	require.NoError(t, err)
	return string(out)
}

// TestStdoutReporter_LogsEachNewCorrelation verifies that each entry in
// NewCorrelations produces one log line.
func TestStdoutReporter_LogsEachNewCorrelation(t *testing.T) {
	r := &stdoutReporter{}
	out := captureStdout(t, func() {
		r.Report(output(correlation("A", 100), correlation("B", 100)))
	})
	assert.Equal(t, 1, strings.Count(out, "pattern=A"))
	assert.Equal(t, 1, strings.Count(out, "pattern=B"))
}

// TestStdoutReporter_SilentWhenNoNewCorrelations verifies no output for an
// empty NewCorrelations.
func TestStdoutReporter_SilentWhenNoNewCorrelations(t *testing.T) {
	r := &stdoutReporter{}
	out := captureStdout(t, func() { r.Report(reporterdef.ReportOutput{}) })
	assert.Empty(t, out)
}

// TestStdoutReporter_PrintsNewAnomalies verifies that NewAnomalies are printed
// even when there are no new correlations.
func TestStdoutReporter_PrintsNewAnomalies(t *testing.T) {
	r := &stdoutReporter{}
	ro := reporterdef.ReportOutput{
		NewAnomalies: []observerdef.Anomaly{
			{DetectorName: "cusum", Source: observerdef.SeriesDescriptor{Name: "cpu"}},
		},
	}
	out := captureStdout(t, func() { r.Report(ro) })
	assert.Contains(t, out, "cusum:cpu")
}
