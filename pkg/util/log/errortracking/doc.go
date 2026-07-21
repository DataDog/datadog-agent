// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errortracking forwards Agent error logs to the internal agent
// telemetry intake. It exposes:
//
//   - ErrorLog: the value-type that crosses the boundary from
//     pkg/util/log into comp/core/agenttelemetry. Keeping a plain struct on
//     the boundary lets the foundational logger subtree stay free of
//     comp/core dependencies and lets agenttelemetry's public method avoid
//     leaking log/slog into every consumer.
//
//   - Submitter: the function type that consumers register to receive
//     ErrorLog values. Implementations MUST be non-blocking and safe for
//     concurrent use; the agenttelemetry component owns an internal bounded
//     channel and flushes asynchronously.
//
//   - Handler: the slog.Handler installed in the Agent's logger chain at
//     construction. It captures records at level >= Error, builds an
//     ErrorLog, and calls the currently registered Submitter (atomically
//     loaded on every record). When no Submitter is registered, Handle is
//     a silent no-op so the chain works whether or not errortracking is
//     opted in.
//
//   - Bouncer: an optional per-PC first-sighting deduplicator with a
//     sliding time window. When attached via Handler.WithBouncerLoader,
//     duplicate sightings of the same call site inside the window are
//     suppressed; the suppressed-duplicate count rides on the next
//     non-suppressed sighting via ErrorLog.Count.
//
// Why the slot-and-loader indirection (atomic.Pointer-backed): the slog
// chain is built at logger setup time, very early in agent startup —
// before the Fx graph has constructed the agenttelemetry component that
// owns the Submitter and the Bouncer. The foundational logger subtree
// (pkg/util/log/*) cannot import comp/* (layering rule + import cycle).
// Atomic-pointer slots let the producer publish a value and the consumer
// read it lock-free on every Handle call, without the producer ever
// taking a static dependency on the consumer's package. The same pattern
// can be reused for any future late-bound handler (flare upload, remote
// config, …) — see pkg/util/log/setup/log.go's "late-binding handlers"
// section comment.
//
// Wiring: pkg/util/log/setup installs the Handler at logger build time
// with closures that atomically load the package-global Submitter and
// Bouncer slots. The Fx graph in cmd/agent/subcommands/run/ calls
// RegisterErrortrackingSubmitter and RegisterErrortrackingBouncer
// exactly once at OnStart, pointing at agenttelemetry's SubmitErrorLog
// method and a freshly-constructed Bouncer. agenttelemetry.stop()
// clears both slots before its own cancel/drain so producers stop
// reaching the channel before the final flush begins.
package errortracking
