// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityevents

// SeverityLevel represents one of three severity states: Low, Medium, High.
type SeverityLevel int

const (
	SeverityLow SeverityLevel = iota
	SeverityMedium
	SeverityHigh

	// NumSeverityLevels is the number of valid SeverityLevel values.
	NumSeverityLevels
)

// SeverityEventDirection describes whether a severity transition is an
// escalation or de-escalation.
type SeverityEventDirection int

const (
	// SeverityEventBoth delivers transitions in either direction (default zero value).
	SeverityEventBoth SeverityEventDirection = 0
	// SeverityEventEscalation delivers only transitions where ToLevel > FromLevel.
	SeverityEventEscalation SeverityEventDirection = 1
	// SeverityEventDeescalation delivers only transitions where ToLevel < FromLevel.
	SeverityEventDeescalation SeverityEventDirection = 2
)

// SeverityEvent records a severity state-machine transition.
type SeverityEvent struct {
	// Timestamp is the data time (unix seconds) when the transition occurred.
	Timestamp int64 `json:"timestamp"`
	// FromLevel is the state before the transition.
	FromLevel SeverityLevel `json:"from_level"`
	// ToLevel is the state after the transition.
	ToLevel SeverityLevel `json:"to_level"`
	// Direction is SeverityEventEscalation when ToLevel > FromLevel, and
	// SeverityEventDeescalation when ToLevel < FromLevel.
	Direction SeverityEventDirection `json:"direction"`
}

// SeverityEventListener receives severity state-machine transitions from the scorer.
type SeverityEventListener interface {
	OnSeverityTransition(event SeverityEvent)
}

// Dispatcher is the handle returned for one push-based severity event stream.
// Its concrete type is owned by the implementation package.
type Dispatcher interface{}

// SeverityEventFilter selects which SeverityEvents are delivered to a listener.
// All conditions are ANDed; a nil or empty slice means "any value".
// The zero value SeverityEventFilter{} matches every transition.
type SeverityEventFilter struct {
	// FromLevels restricts to events whose FromLevel is in the set.
	FromLevels []SeverityLevel
	// ToLevels restricts to events whose ToLevel is in the set.
	ToLevels []SeverityLevel
	// Direction restricts by escalation or de-escalation.
	Direction SeverityEventDirection
}

// SeverityEventsConfiguration configures one severity event subscription's
// filter/cooldown. The listener is passed separately to SubscribeSeverityEvents.
type SeverityEventsConfiguration struct {
	// Filter controls which transitions are delivered.
	// Zero value SeverityEventFilter{} delivers all transitions.
	Filter SeverityEventFilter
	// CooldownSecs is the minimum number of seconds that must elapse after a
	// delivered transition before a downward (de-escalation) transition can be
	// delivered again.
	// Zero means no cooldown (every matching transition is delivered).
	CooldownSecs int64
}

// SeverityEventsSubscription is returned by SubscribeSeverityEvents.
type SeverityEventsSubscription struct {
	Dispatcher  Dispatcher
	Unsubscribe func()
}

// Reader is a pull-based view of the current severity level, backed by a
// dedicated severity event subscription.
type Reader interface {
	// GetSeverity returns the most recently observed severity level. Safe
	// for concurrent use from any goroutine.
	GetSeverity() SeverityLevel
}

// SeverityEventsReaderSubscription is returned by SubscribeSeverityEventsReader.
type SeverityEventsReaderSubscription struct {
	Reader      Reader
	Unsubscribe func()
}
