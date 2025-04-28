// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry implements a component to generate Agent telemetry
package agenttelemetry

import (
	"context"

	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// team: agent-runtimes

// Component is the component type
type Component interface {
	// SendEvent sends event payload.
	//    payloadType - should be registered in datadog-agent\comp\core\agenttelemetry\impl\config.go
	//    payload     - de-serializable into JSON
	SendEvent(eventType string, eventPayload []byte) error

	StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context)
}
