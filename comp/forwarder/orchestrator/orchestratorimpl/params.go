// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package orchestratorimpl

// Params defines the parameters for the orchestrator forwarder.
type Params struct {
	useNoopOrchestratorForwarder bool
	useOrchestratorForwarder     bool
}

// NewDefaultParams returns the default parameters for the orchestrator forwarder.
func NewDefaultParams() Params {
	return Params{useOrchestratorForwarder: true, useNoopOrchestratorForwarder: false}
}

// NewDisabledParams returns the parameters for the orchestrator forwarder when it is disabled.
func NewDisabledParams() Params {
	return Params{useOrchestratorForwarder: false, useNoopOrchestratorForwarder: false}
}

// NewNoopParams returns the parameters for the orchestrator forwarder when it is a noop.
func NewNoopParams() Params {
	return Params{useOrchestratorForwarder: false, useNoopOrchestratorForwarder: true}
}
