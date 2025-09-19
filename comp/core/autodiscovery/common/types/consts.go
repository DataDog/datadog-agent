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
	// CelContainerIdentifier is the CEL identifier for container resources.
	CelContainerIdentifier = CelIdentifierPrefix + "container"
	// CelPodIdentifier is the CEL identifier for pod resources.
	CelPodIdentifier = CelIdentifierPrefix + "pod"
	// CelServiceIdentifier is the CEL identifier for service resources.
	CelServiceIdentifier = CelIdentifierPrefix + "kube_service"
	// CelEndpointIdentifier is the CEL identifier for endpoint resources.
	CelEndpointIdentifier = CelIdentifierPrefix + "kube_endpoint"
)
