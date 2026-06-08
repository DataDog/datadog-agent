// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// EventReporter sends Datadog events for new/recurred correlations via eventSender.
// Dedup is handled entirely by the engine: this reporter fires on every entry in
// output.NewCorrelations. It maintains a small retryQ for patterns whose send()
// failed in a prior cycle so transient forwarder errors are retried without
// requiring re-detection.
// It implements reporterdef.StorageConsumer so the observer can inject engine
// storage post-construction for windowed log-rate annotations in change messages.
type EventReporter struct {
	sender *eventSender
	logger log.Component
	retryQ map[string]observerdef.ActiveCorrelation // patterns whose last send() failed
}

// Ensure EventReporter satisfies both interfaces at compile time.
var _ reporterdef.Reporter = (*EventReporter)(nil)
var _ reporterdef.StorageConsumer = (*EventReporter)(nil)

// Name returns the reporter name.
func (r *EventReporter) Name() string {
	return "event_reporter"
}

// SetStorage implements reporterdef.StorageConsumer.
// Called by the observer after engine construction to enable windowed log-rate
// annotations in change-event messages.
func (r *EventReporter) SetStorage(storage observerdef.StorageReader) {
	r.sender.storage = storage
}

// Report attempts to send a Datadog change event for each entry in
// output.NewCorrelations and for any previously-failed patterns in retryQ.
//
// Retry semantics: a transient forwarder error keeps the pattern in retryQ so
// the next advance retries publication. A pattern is removed from retryQ once
// its send succeeds. If NewCorrelations brings a newer version of a pattern
// that is already in retryQ, the newer version replaces the queued one.
func (r *EventReporter) Report(output reporterdef.ReportOutput) {
	if r.retryQ == nil {
		r.retryQ = make(map[string]observerdef.ActiveCorrelation)
	}

	// Merge new correlations into the retry queue, replacing stale entries.
	for _, ac := range output.NewCorrelations {
		r.retryQ[ac.Pattern] = ac
	}

	// Attempt all pending sends (both retried and freshly-arrived patterns).
	for pattern, ac := range r.retryQ {
		if err := r.sender.send(ac); err != nil {
			r.logger.Errorf("[observer] failed to send event for pattern %s: %v", ac.Pattern, err)
			continue
		}
		delete(r.retryQ, pattern)
	}
}
