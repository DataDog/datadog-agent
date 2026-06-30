// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogconnector

import (
	"go.opentelemetry.io/collector/component"
)

// Type is the type of the Datadog connector.
var Type = component.MustNewType("datadog")

const (
	// TracesToMetricsStability is the stability level of the traces-to-metrics connector.
	//
	// Upstream marks this beta, but the embedded collector intentionally keeps it at
	// development stability to preserve the connector's historical registration behavior
	// in the Agent.
	TracesToMetricsStability = component.StabilityLevelDevelopment
	// TracesToTracesStability is the stability level of the traces-to-traces connector.
	// See TracesToMetricsStability for why this is development rather than beta.
	TracesToTracesStability = component.StabilityLevelDevelopment
)
