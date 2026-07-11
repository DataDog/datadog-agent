// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"time"
)

// MaxStackFrames is the upper bound on captured stack PCs per record.
// 16 is empirically deep enough to cover user code and its immediate
// callers from any agent error site and shallow enough that the
// resulting StackTrace string stays well under typical intake-side
// per-record limits.
const MaxStackFrames = 16

// ErrorLog is a value-typed snapshot of an error log record. It crosses
// the pkg/util/log -> comp/core boundary, keeping the
// foundational logger subtree free of comp/core dependencies and the
// agenttelemetry component free of log/slog leakage on its public
// interface.
//
// Producers (slog handlers under pkg/util/log) build an ErrorLog and pass
// it to a Submitter. Consumers (the agenttelemetry component) accept
// ErrorLog on their public method and translate it internally into the
// dd-go wire schema before sending.
//
// The handler captures only the wire-relevant fields (Time, PC, stack
// PCs, Count); message text and attrs are not captured because they are
// potentially user-controlled.
type ErrorLog struct {
	// Time is the wall-clock instant the record was emitted.
	Time time.Time

	// PC is the program counter of the call site that emitted the
	// record. PCs[0] is the same value when the handler captured a full
	// stack; PC is retained as a convenience for consumers that only
	// need the immediate caller (e.g. the Bouncer key).
	PC uintptr

	// PCs is a bounded stack capture starting at the immediate caller
	// and walking up. PCsLen records the number of valid entries (may
	// be less than len(PCs) when the stack is shallow). Captured at
	// log-time by the handler anchored at r.PC so slog and
	// pkg/util/log wrapper frames are excluded — see Handler.Handle.
	PCs    [MaxStackFrames]uintptr
	PCsLen int

	// Count is the number of same-PC sightings the Bouncer collapsed
	// into this record (≥ 1; 1 means "first or only sighting in the
	// current bouncer window"). Propagated to the wire Log.Count.
	Count uint32

	// ErrorKind is the reflect type name of the first error-typed slog
	// attribute found in the record (e.g. "*net.OpError"). Empty when the
	// log call carried no error attribute. The type name is not
	// user-controlled — it is determined by the code that creates the
	// error — so it is safe to ship unlike the error message itself.
	ErrorKind string
}

// Submitter is the registration target for sending an ErrorLog to a
// consumer. The slog handler under pkg/util/log calls the currently
// installed Submitter (atomically loaded) on each error record.
//
// Why a function-pointer slot rather than a constructor-injected
// dependency: the slog handler chain is built at logger setup time —
// very early in agent startup, before the Fx graph has constructed any
// component. The eventual consumer (the agenttelemetry component) does
// not exist yet at that point, and pkg/util/log/* cannot import
// comp/* (layering rule + import cycle). The atomic-pointer indirection
// is the only shape that satisfies (a) the layering constraint, (b) the
// "consumer is built later than the producer" lifecycle gap, and (c)
// the lock-free hot path requirement.
//
// Implementations MUST be non-blocking on the hot path — the consumer is
// expected to enqueue into a bounded buffer and flush asynchronously.
// Implementations MUST be safe for concurrent calls.
//
// A nil Submitter means "errortracking is not yet configured"; callers
// must guard against this and drop the record silently. This is also
// the gate the agenttelemetry component uses to enable/disable the
// feature: when
// agent_telemetry.errortracking.enabled=false (or gov/FIPS excludes
// the parent agent_telemetry feature), the component never calls
// pkg/util/log/setup.RegisterErrortrackingSubmitter and the slot
// stays nil — the handler short-circuits via Enabled() == false.
type Submitter func(ErrorLog)
