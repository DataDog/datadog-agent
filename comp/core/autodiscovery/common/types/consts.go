// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

const (
	// CheckCmdName is the check name for autodiscovery component, used by cli mode.
	CheckCmdName = "check-cmd"
	// CelIdentifierPrefix is the prefix used to identify CEL-based AD identifiers.
	CelIdentifierPrefix = "cel://"
)

// CelIdentifier represents a CEL-based AD identifier.
type CelIdentifier string

const (
	// CelContainerIdentifier is the CEL identifier for container resources.
	CelContainerIdentifier CelIdentifier = CelIdentifierPrefix + "container"
	// CelProcessIdentifier is the CEL identifier for process resources.
	CelProcessIdentifier CelIdentifier = CelIdentifierPrefix + "process"
	// CelServiceIdentifier is the CEL identifier for service resources.
	CelServiceIdentifier CelIdentifier = CelIdentifierPrefix + "kube_service"
	// CelEndpointIdentifier is the CEL identifier for endpoint resources.
	CelEndpointIdentifier CelIdentifier = CelIdentifierPrefix + "kube_endpoint"
)
