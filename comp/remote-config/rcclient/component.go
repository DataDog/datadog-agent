// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient //nolint:revive // TODO(RC) Fix revive linter

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// team: remote-config

// Component is the component type.
type Component interface {
	// TODO: (components) Subscribe to AGENT_CONFIG configurations and start the remote config client
	// Once the remote config client is refactored and can push updates directly to the listeners,
	// we can remove this.
	Start(agentName string) error
	// SubscribeAgentTask subscribe the remote-config client to AGENT_TASK
	SubscribeAgentTask()
	// SubscribeApmTracing subscribes the remote-config client to APM_TRACING
	SubscribeApmTracing()
	// Subscribe is the generic way to start listening to a specific product update
	// Component can also automatically subscribe to updates by returning a `ListenerProvider` struct
	Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

type TaskType string

// AgentTaskConfig is a deserialized agent task configuration file
// along with the associated metadata
type AgentTaskConfig struct {
	Config   AgentTaskData
	Metadata state.Metadata
}

// agentTaskData is the content of a agent task configuration file
type AgentTaskData struct {
	TaskType string            `json:"task_type"`
	UUID     string            `json:"uuid"`
	TaskArgs map[string]string `json:"args"`
}

const (
	// TaskFlare is the task sent to request a flare from the agent
	TaskFlare TaskType = "flare"
)

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// RCListener is the generic type for components to register a callback for any product
type RCListener map[data.Product]func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))

// TaskListenerProvider defines component that can receive RC updates
type TaskListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentTaskListener"`
}

// ListenerProvider defines component that can receive RC updates for any product
type ListenerProvider struct {
	fx.Out

	ListenerProvider RCListener `group:"rCListener"`
}
