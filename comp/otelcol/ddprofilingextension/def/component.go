// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextension defines the otel agent ddprofilingextension component.
package ddprofilingextension

import "go.opentelemetry.io/collector/extension"

// team: opentelemetry-agent

// Component implements the component.Component interface.
type Component interface {
	extension.Extension // Embed base Extension for common functionality.
}
