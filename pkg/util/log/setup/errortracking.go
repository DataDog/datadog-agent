// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// --- Late-binding handlers ---------------------------------------------
//
// A "late-binding handler" is a slog.Handler whose downstream consumer
// is constructed AFTER the slog chain itself is built. SetupLogger
// runs very early — before the Fx graph has had a chance to create
// any comp/core component. pkg/util/log/* must not import comp/*
// (layering + cycle), so the producer cannot hold a typed pointer to
// the consumer; instead, the producer reads from a package-global
// atomic.Pointer slot, and the consumer publishes itself into the slot
// during its Fx OnStart hook.
//
// The errortracking handler uses this pattern via two slots: a
// Submitter for the per-record callback, and an optional Bouncer for
// per-PC dedup. Both are atomically loaded on every Handle call so the
// hot path remains lock-free, and both slots default to nil so the
// handler short-circuits via Enabled() == false until startup wiring
// completes. The same shape can be reused for any future handler with
// the same lifecycle gap (flare upload, remote config, etc.) — copy
// the slot + setter + loader trio below.
//
// errortrackingSubmitterSlot is the package-global atomic pointer that
// backs every errortracking.Handler returned by buildSlogLogger. The Fx
// wiring at cmd/agent/subcommands/run/ registers the agenttelemetry
// component's SubmitErrorRecord method here at startup; until then the slot
// is nil and the handler in the chain reports Enabled = false so the
// parent multi-handler skips it entirely.
var errortrackingSubmitterSlot atomic.Pointer[errortracking.Submitter]

// RegisterErrortrackingSubmitter installs s as the destination for error
// records routed through the errortracking branch of every slog chain
// built via buildSlogLogger. Passing nil clears the registration so the
// branch becomes a no-op again - tests use this to clean up between cases.
//
// This is a setter rather than a constructor argument because SetupLogger
// is invoked from many call-sites (most without Fx access) and we do not
// want to thread a Submitter through every one. The Fx graph in the run
// command resolves the agenttelemetry component and calls
// RegisterErrortrackingSubmitter exactly once during startup. The
// errortracking.Handler atomic-loads the slot on every record, so there
// is no slow-path mutex on the logger hot path.
func RegisterErrortrackingSubmitter(s errortracking.Submitter) {
	if s == nil {
		errortrackingSubmitterSlot.Store(nil)
		return
	}
	errortrackingSubmitterSlot.Store(&s)
}

// errortrackingBouncerSlot mirrors errortrackingSubmitterSlot for the
// per-PC dedup Bouncer. Late-bound for the same reason: SetupLogger
// runs before the agenttelemetry component (which owns the config that
// chooses the window) is constructed.
var errortrackingBouncerSlot atomic.Pointer[errortracking.Bouncer]

// RegisterErrortrackingBouncer installs b as the per-PC dedup
// Bouncer for the errortracking branch of every slog chain built via
// buildSlogLogger. Passing nil clears the registration so subsequent
// records bypass dedup. The Fx graph in the run command's
// installErrortrackingHandler registers the agenttelemetry component's
// Bouncer here at startup, alongside the Submitter.
func RegisterErrortrackingBouncer(b *errortracking.Bouncer) {
	errortrackingBouncerSlot.Store(b)
}
