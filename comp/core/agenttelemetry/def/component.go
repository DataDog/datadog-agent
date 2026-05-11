// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry implements a component to generate Agent telemetry
package agenttelemetry

import (
	"context"
	"log/slog"

	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// team: agent-runtimes

// Component is the component type
type Component interface {
	// SendEvent sends event payload.
	//    payloadType - should be registered in datadog-agent\comp\core\agenttelemetry\impl\config.go
	//    payload     - de-serializable into JSON
	SendEvent(eventType string, eventPayload []byte) error

	// SubmitErrorRecord accepts a single error log record from the
	// pkg/util/log slog handler and enqueues it for asynchronous flush
	// to the Cross-Org Agent Telemetry (COAT) intake. Implementations
	// MUST be non-blocking on the hot path: enqueue to a bounded buffer
	// and drop silently on overflow. Records emitted from inside the
	// agenttelemetry component (recursion guard) MUST be dropped.
	//
	// This method receives the foundational ErrorLog DTO defined at
	// pkg/util/log/errortracking; the component never sees raw slog
	// types on its public surface.
	SubmitErrorRecord(log errortracking.ErrorLog)

	// SendErrorLogs is the v2 batch-mode entry point retained for the
	// existing wiring. Deprecated: prefer SubmitErrorRecord on the new
	// per-record path. Will be removed once the v3 migration completes.
	SendErrorLogs(ctx context.Context, batch []slog.Record) error

	StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context)
}
