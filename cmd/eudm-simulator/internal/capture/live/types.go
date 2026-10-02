// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package live coordinates authenticated, bounded capture sessions in running
// producers. It does not collect or alter production submission schedules.
package live

import (
	"context"
	"errors"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Bound duplicate-cycle history independently of the producer's retained queue.
// At most three participating producers each retain this many cycle identities.
const maxSessionCycles = 65536

// Discovery retries only transient unavailability. Authentication and build
// incompatibility need an operator correction, not a cadence wait.
var (
	ErrUnavailable    = errors.New("capture producer API unavailable")
	ErrIncompatible   = errors.New("capture producer API incompatible; install a compatible producer build")
	ErrAuthentication = errors.New("capture producer API authentication failed")
)

// Client accesses one installed producer's authenticated local API.
type Client interface {
	Role() string
	Capabilities(context.Context) (tc.Status, error)
	Prepare(context.Context, tc.PrepareRequest) (tc.Status, error)
	Activate(context.Context, tc.Control) (tc.Status, error)
	Heartbeat(context.Context, tc.Control) (tc.Status, error)
	Records(context.Context, tc.ReadRequest) (Batch, error)
	Stop(context.Context, tc.Control) (tc.Status, error)
}

// Batch is one bounded producer response. Records are acknowledged by the next
// request's cursor only after the coordinator has processed them successfully.
type Batch struct {
	Status  tc.Status   `json:"status"`
	Records []tc.Record `json:"records"`
}

// Participant contains the actual activation acknowledgement and selected
// streams of one producer. Its other advertised capabilities are not selected.
type Participant struct {
	Status  tc.Status
	Streams []tc.Stream
}

// Session identifies a single cross-process capture. Origin is the earliest
// acknowledged activation boundary, shared by all sanitized relative times.
type Session struct {
	ID           string
	Origin       time.Time
	Participants []Participant
}

// Sink owns the single sanitizer and sanitized output writer. Accept is always
// serial: one complete logical cycle is handled before the next is delivered.
type Sink interface {
	Start(context.Context, Session) error
	Accept(context.Context, tc.Record) error
	Coverage() (bool, string)
	Finish(context.Context, []tc.Status, time.Time) error
}
