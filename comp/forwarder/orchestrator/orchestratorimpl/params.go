// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package orchestratorimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
)

// Params defines the parameters for the orchestrator forwarder.
type Params struct {
	useNoopOrchestratorForwarder bool
	useOrchestratorForwarder     bool
	defaultForwarderParams       defaultforwarder.Params
}

// NewDefaultParams returns the default parameters for the orchestrator forwarder.
func NewDefaultParams(defaultForwarderParams defaultforwarder.Params) Params {
	return Params{
		useOrchestratorForwarder:     true,
		useNoopOrchestratorForwarder: false,
		defaultForwarderParams:       defaultForwarderParams,
	}
}

// NewDisabledParams returns the parameters for the orchestrator forwarder when it is disabled.
func NewDisabledParams() Params {
	return Params{useOrchestratorForwarder: false, useNoopOrchestratorForwarder: false}
}

// NewNoopParams returns the parameters for the orchestrator forwarder when it is a noop.
func NewNoopParams() Params {
	return Params{useOrchestratorForwarder: false, useNoopOrchestratorForwarder: true}
}

// String returns a pretty-printed representation of the Params
func (p Params) String() string {
	return fmt.Sprintf("Orchestrator Forwarder Params:\n"+
		"  useOrchestratorForwarder: %v\n"+
		"  useNoopOrchestratorForwarder: %v\n"+
		"  defaultForwarderParams.Features: %v",
		p.useOrchestratorForwarder,
		p.useNoopOrchestratorForwarder,
		p.defaultForwarderParams.Features())
}
