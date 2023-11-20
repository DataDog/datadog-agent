// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package forwarderimpl

// Params defines the parameters for the orchestrator forwarder.
type Params struct {
	UseNoopOrchestratorForwarder bool
	UseOrchestratorForwarder     bool
}

// NewDefaultParams returns the default parameters for the orchestrator forwarder.
func NewDefaultParams() Params {
	return Params{UseOrchestratorForwarder: true, UseNoopOrchestratorForwarder: false}
}
