// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"go.uber.org/fx"
)

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// ListenerProvider defines component that can receive RC updates
type ListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentTaskListener"`
}

// NewProvider returns a new Provider to be called when there is an agent task update
func NewProvider(callback RCAgentTaskListener) ListenerProvider {
	return ListenerProvider{
		Listener: callback,
	}
}
