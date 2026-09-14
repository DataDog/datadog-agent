// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package egress defines the interface for the health platform egress component.
package egress

import "time"

// team: fleet-remediation

// SendStatus reports the health of the egress -> forwarder send pipeline,
// for display in `agent status`.
type SendStatus struct {
	// Healthy is true if the most recent send attempt succeeded, or if no
	// attempt has happened yet.
	Healthy bool
	// LastAttemptAt is the time of the most recent send attempt, zero if none happened yet.
	LastAttemptAt time.Time
	// LastSuccessAt is the time of the most recent successful send, zero if none succeeded yet.
	LastSuccessAt time.Time
	// LastError is the error from the most recent failed send attempt, nil if
	// the last attempt succeeded or none happened yet.
	LastError error
	// IssuesSentTotal is the cumulative number of issues sent to the Datadog intake.
	IssuesSentTotal int64
	// BytesSentTotal is the cumulative number of payload bytes sent to the Datadog intake.
	BytesSentTotal int64
	// SendErrorsTotal is the cumulative number of failed send attempts.
	SendErrorsTotal int64
}

// Component is the health platform egress component interface.
// Egress drives the periodic outbound HTTP POST to the Datadog intake:
// on each tick it calls store.GetAllIssues(), builds a HealthReport, and
// forwards it via forwarder.Send. Behaviour is driven entirely by its fx
// lifecycle hooks; Status exposes the outcome for display in `agent status`.
type Component interface {
	// Status returns the current health of the egress send pipeline.
	Status() SendStatus
}
