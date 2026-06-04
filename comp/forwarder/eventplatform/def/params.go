// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package eventplatform

// Params defines the parameters for the event platform forwarder.
type Params struct {
	UseNoopEventPlatformForwarder bool
	UseEventPlatformForwarder     bool
	// UseClusterAgentPipelines, when true, limits the forwarder to only the
	// event types needed by the Cluster Agent (network-path and, if enabled,
	// kube-actions).  All other pipelines (DBM, NDM, NetFlow, container
	// lifecycle, SBOM, Synthetics, etc.) are skipped.
	UseClusterAgentPipelines bool
}

// NewDefaultParams returns the default parameters for the event platform forwarder.
func NewDefaultParams() Params {
	return Params{UseEventPlatformForwarder: true, UseNoopEventPlatformForwarder: false}
}

// NewClusterAgentParams returns parameters for the Cluster Agent's event
// platform forwarder.  Only the pipelines required by the DCA
// (network-path + conditional kube-actions) are built.
func NewClusterAgentParams() Params {
	return Params{UseEventPlatformForwarder: true, UseClusterAgentPipelines: true}
}

// NewDisabledParams returns the disabled parameters for the event platform forwarder.
func NewDisabledParams() Params {
	return Params{UseEventPlatformForwarder: false, UseNoopEventPlatformForwarder: false}
}
