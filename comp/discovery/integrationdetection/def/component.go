// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrationdetection defines the integration detection component.
// Enable by setting discovery.integration_detection.enabled=true in datadog.yaml.
//
// team: agent-discovery
package integrationdetection

import integrationdetectionpkg "github.com/DataDog/datadog-agent/pkg/discovery/integrationdetection"

// EnabledIntegration is a type alias for the core package type. It allows
// callers that only depend on this component interface to consume the returned
// slice without importing the full implementation package. This component
// interface is intended for read-only consumers; construction of
// EnabledIntegration values is an implementation detail of the core package.
type EnabledIntegration = integrationdetectionpkg.EnabledIntegration

// Component exposes the live set of integrations detected via Autodiscovery events.
type Component interface {
	// EnabledIntegrations returns a snapshot of currently-enabled integrations.
	// Returns nil when no integrations are detected or the component is disabled.
	EnabledIntegrations() []EnabledIntegration
}
