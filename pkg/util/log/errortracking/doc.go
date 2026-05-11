// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errortracking forwards Agent error logs to Datadog Cross-Org Agent
// Telemetry (COAT). It exposes:
//
//   - ErrorLog: the value-typed DTO that crosses the boundary from
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
// Wiring: pkg/util/log/setup installs the Handler at logger build time with
// a closure that atomically loads the package-global Submitter slot. The
// Fx graph in cmd/agent/subcommands/run/ calls
// RegisterErrortrackingSubmitter exactly once during startup, pointing at
// agenttelemetry's SubmitErrorRecord method.
package errortracking
