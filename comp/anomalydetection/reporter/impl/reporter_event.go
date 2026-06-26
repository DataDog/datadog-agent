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

// defaultMaxRetryAttempts is the number of consecutive send failures after
// which a pending CorrelationDetected entry is evicted from retryPending.
const defaultMaxRetryAttempts = 5

// retryEntry pairs a correlation with the number of send attempts made so far.
type retryEntry struct {
	correlation observerdef.ActiveCorrelation
	attempts    int
}

// EventReporter sends Datadog events for correlator lifecycle events.
// Deduplication and recurrence logic live inside each correlator via the
// correlationEmitter helper. The reporter forwards each CorrelatorEvent to the
// appropriate sender method.
//
// CorrelationDetected sends that fail transiently are buffered in retryPending
// and retried at the start of the next Report call. This preserves the
// pre-refactor behaviour where seenCorrelations was only marked after a
// successful send, so transient forwarder/intake failures were automatically
// retried on the next advance cycle. An entry is evicted after maxRetries
// consecutive failures; a warning is logged at eviction time.
//
// Episode events (EpisodeStarted/EpisodeEnded) remain at-most-once; each
// transition fires exactly once so there is nothing to retry.
//
// It implements reporterdef.StorageConsumer so the observer can inject engine
// storage post-construction for windowed log-rate annotations in change messages.
type EventReporter struct {
	sender     *eventSender
	logger     log.Component
	maxRetries int
	// retryPending holds CorrelationDetected entries whose last send attempt
	// failed transiently. Retried at the start of each Report call; evicted
	// after maxRetries consecutive failures.
	retryPending []retryEntry
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

// Report forwards all correlator events from this advance cycle.
//
//   - EpisodeStarted / EpisodeEnded  → sendEpisodeEvent (scorer severity transitions, at-most-once)
//   - CorrelationDetected            → send (cluster/pattern first-seen, emitter-deduplicated)
//
// CorrelationDetected sends that fail are queued in retryPending and retried
// at the start of the next call. Each entry is evicted after r.maxRetries
// consecutive failures. Episode events are at-most-once (no retry).
func (r *EventReporter) Report(output reporterdef.ReportOutput) bool {
	emitted := false

	// Retry CorrelationDetected sends that failed on a previous cycle.
	var stillPending []retryEntry
	for _, entry := range r.retryPending {
		if err := r.sender.send(entry.correlation); err != nil {
			entry.attempts++
			if entry.attempts >= r.maxRetries {
				r.logger.Warnf("[observer] dropping correlation event pattern=%s after %d failed attempts: %v",
					entry.correlation.Pattern, entry.attempts, err)
				continue // evict
			}
			r.logger.Errorf("[observer] retry %d/%d: failed to send correlation event pattern=%s: %v",
				entry.attempts, r.maxRetries, entry.correlation.Pattern, err)
			stillPending = append(stillPending, entry)
			continue
		}
		emitted = true
	}
	r.retryPending = stillPending

	for _, ce := range output.CorrelatorEvents {
		switch ce.Kind {
		case observerdef.CorrelatorEventEpisodeStarted, observerdef.CorrelatorEventEpisodeEnded:
			if err := r.sender.sendEpisodeEvent(ce); err != nil {
				r.logger.Errorf("[observer] failed to send scorer episode event pattern=%s kind=%d: %v",
					ce.Correlation.Pattern, ce.Kind, err)
				continue
			}
		case observerdef.CorrelatorEventCorrelationDetected:
			if err := r.sender.send(ce.Correlation); err != nil {
				r.logger.Errorf("[observer] failed to send correlation event pattern=%s: %v",
					ce.Correlation.Pattern, err)
				r.retryPending = append(r.retryPending, retryEntry{correlation: ce.Correlation, attempts: 1})
				continue
			}
		default:
			r.logger.Warnf("[observer] unknown correlator event kind %d, skipping", ce.Kind)
			continue
		}
		emitted = true
	}
	return emitted
}
