// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package forwarder defines the interface for the health platform forwarder.
package forwarder

import (
	"context"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// team: agent-health fleet-remediation

// Component is the stateless forwarder component. It POSTs a pre-built
// HealthReport to the Datadog intake. The periodic tick is owned by the
// egress component; the forwarder only handles the HTTP mechanics.
type Component interface {
	// Send POSTs the given report to the Datadog intake.
	// The caller is responsible for building the report and choosing when to send.
	Send(ctx context.Context, report *healthplatformpayload.HealthReport) error
}
