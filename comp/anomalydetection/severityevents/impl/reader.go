// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityeventsimpl

import (
	"sync/atomic"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// SeverityReader is a pull-based severity event listener: it subscribes once
// and keeps GetSeverity() up to date from the delivered transitions.
// GetSeverity() is a single atomic load — no allocation, no locking — safe to
// call from many goroutines on a hot path.
type SeverityReader struct {
	current atomic.Int32 // holds a severityeventsdef.SeverityLevel
}

// NewSeverityReader subscribes to o and returns a SeverityEventsReaderSubscription
// whose Reader.GetSeverity() reflects the latest severity level, along with the
// Unsubscribe function that stops the underlying subscription. If o already
// knows the current level, SubscribeSeverityEvents delivers it immediately
// (see severityeventsdef.Subscriber), so GetSeverity() reflects the real
// state right away rather than defaulting to Low until the next transition.
func NewSeverityReader(o severityeventsdef.Subscriber, cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	r := &SeverityReader{}
	r.current.Store(int32(severityeventsdef.SeverityLow))

	sub, err := o.SubscribeSeverityEvents(cfg, r)
	if err != nil {
		return severityeventsdef.SeverityEventsReaderSubscription{}, err
	}
	return severityeventsdef.SeverityEventsReaderSubscription{
		Reader:      r,
		Unsubscribe: sub.Unsubscribe,
	}, nil
}

// OnSeverityTransition implements severityeventsdef.SeverityEventListener.
func (r *SeverityReader) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	r.current.Store(int32(clampSeverityLevel(evt.ToLevel)))
}

// GetSeverity returns the most recently observed severity level. Safe for
// concurrent use from any goroutine.
func (r *SeverityReader) GetSeverity() severityeventsdef.SeverityLevel {
	return severityeventsdef.SeverityLevel(r.current.Load())
}
