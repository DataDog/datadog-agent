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
)

// team: agent-runtimes

// Component is the component type
type Component interface {
	// SendEvent sends event payload.
	//    payloadType - should be registered in datadog-agent\comp\core\agenttelemetry\impl\config.go
	//    payload     - de-serializable into JSON
	SendEvent(eventType string, eventPayload []byte) error

	// SendErrorLogs ships a batch of slog records to the Cross-Org Agent
	// Telemetry (COAT) intake using the apmtelemetry-style logs envelope
	// (request_type=logs). The wire payload conforms to dd-go's
	// trace/apps/tracer-telemetry-intake/telemetry-payload/logs.go schema.
	//
	// Implementations MUST be safe for concurrent use. Empty batches are a
	// no-op. A non-nil error signals a retryable transport failure
	// (network error, 5xx). 4xx responses and the "component disabled"
	// case return nil so the calling Pipeline does not waste its retry on
	// a request that will never succeed.
	SendErrorLogs(ctx context.Context, batch []slog.Record) error

	StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context)
}
