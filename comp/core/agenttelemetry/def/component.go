// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry implements a component to generate Agent telemetry
package agenttelemetry

import (
	"context"

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

	// SubmitErrorLog accepts a single error log record from the
	// pkg/util/log slog handler and enqueues it for asynchronous flush
	// to the internal agent telemetry intake. Implementations
	// MUST be non-blocking on the hot path: enqueue to a bounded buffer
	// and drop silently on overflow.
	//
	// Recursion prevention: the flush path
	// (sendLogsBatch → sendPayload) MUST NOT log at
	// Error or above. A flush-path Errorf would re-enter the slog
	// handler and feed records back into this same channel. This
	// invariant is enforced by convention — there is no runtime
	// caller-identity guard. See
	// comp/core/agenttelemetry/impl/errortracking_sender.go. If a
	// flush fails, the failed batch is not re-attempted; the Debug-level
	// log is the only signal.
	//
	// This method receives the ErrorLog value-type defined at
	// pkg/util/log/errortracking; the component never sees raw slog
	// types on its public surface.
	SubmitErrorLog(log errortracking.ErrorLog)

	StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context)
}
