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

// EventReporter sends Datadog events for correlator lifecycle events.
// It is a stateless forwarder: all deduplication and recurrence logic lives
// inside each correlator via the correlationEmitter helper. Reporters simply
// iterate output.CorrelatorEvents and dispatch to the appropriate sender method.
//
// It implements reporterdef.StorageConsumer so the observer can inject engine
// storage post-construction for windowed log-rate annotations in change messages.
type EventReporter struct {
	sender *eventSender
	logger log.Component
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
//   - EpisodeStarted / EpisodeEnded  → sendEpisodeEvent (scorer severity transitions)
//   - CorrelationDetected            → send (cluster/pattern first-seen, emitter-deduplicated)
//
// Events are at-most-once: a transient forwarder error drops the event (the
// correlator already drained it). This matches the existing scorer episode model.
func (r *EventReporter) Report(output reporterdef.ReportOutput) bool {
	emitted := false
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
