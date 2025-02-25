// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package gatewayusageimpl implements the gatewayusage component interface
package gatewayusageimpl

import (
	gatewayusage "github.com/DataDog/datadog-agent/comp/otelcol/gatewayusage/def"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
)

// NewComponent creates a new gatewayusage component
func NewComponent() gatewayusage.Component {
	return attributes.NewGatewayUsage()
}
