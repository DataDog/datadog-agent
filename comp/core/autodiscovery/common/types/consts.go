// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// ServiceTracker reports whether a service is being tracked for endpoint checks
// by an external source (e.g., DatadogInstrumentation CRs).
type ServiceTracker interface {
	HasService(namespace, name string) bool
	// NotifyOnChange registers a callback invoked with the namespace and name of a
	// service whose templates or tracked-state change. Multiple subscribers are supported.
	NotifyOnChange(func(namespace, name string))
}

const (
	// CheckCmdName is the check name for autodiscovery component, used by cli mode.
	CheckCmdName = "check-cmd"
	// CelIdentifierPrefix is the prefix used to identify CEL-based AD identifiers.
	CelIdentifierPrefix = "cel://"
	// KubeContainerNameIdentifierPrefix is the prefix used to identify Kubernetes containers by container name.
	KubeContainerNameIdentifierPrefix = "kube_container_name://"
)

// CelIdentifier represents a CEL-based AD identifier.
type CelIdentifier string

// ConfigRequired returns true if this CEL identifier should be
// injected into a config's ADIdentifiers
func (c CelIdentifier) ConfigRequired(hasADIDs bool) bool {
	return c == CelProcessIdentifier || !hasADIDs
}

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

// KubeContainerNameIdentifier returns the AD identifier for a Kubernetes container name.
func KubeContainerNameIdentifier(containerName string) string {
	return KubeContainerNameIdentifierPrefix + containerName
}
