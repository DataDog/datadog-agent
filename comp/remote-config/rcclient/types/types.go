// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package types provides the different types used by other component to provider remote-config task listeners.
package types

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// TaskType contains the type of the remote config task to execute
type TaskType string

const (
	// TaskFlare is the task sent to request a flare from the agent
	TaskFlare TaskType = "flare"
	// TaskDeviceScan is the task sent to request a device scan for NDM device onboarding.
	TaskDeviceScan TaskType = "ndm-device-scan"
)

// AgentTaskConfig is a deserialized agent task configuration file
// along with the associated metadata
type AgentTaskConfig struct {
	Config   agentTaskData
	Metadata state.Metadata
}

// agentTaskData is the content of a agent task configuration file
type agentTaskData struct {
	TaskType string            `json:"task_type"`
	UUID     string            `json:"uuid"`
	TaskArgs map[string]string `json:"args"`
}

// ParseConfigAgentTask parses an agent task config
func ParseConfigAgentTask(data []byte, metadata state.Metadata) (AgentTaskConfig, error) {
	var d agentTaskData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentTaskConfig{}, fmt.Errorf("Unexpected AGENT_TASK received through remote-config: %s", err)
	}

	return AgentTaskConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// RCListener is the generic type for components to register a callback for any product
type RCListener map[data.Product]func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))

// RCStatusListener receives the complete current snapshot and authoritative
// status after every successful poll, plus connectivity state changes.
type RCStatusListener struct {
	Product       data.Product
	OnStatus      func(map[string]state.RawConfig, pbgo.ConfigStatus, func(string, state.ApplyStatus))
	OnStateChange func(bool)
}

// FilterListeners removes nil/zero values from an fx group of RCListener.
func FilterListeners(group []RCListener) []RCListener {
	return fxutil.GetAndFilterGroup(group)
}

// FilterTaskListeners removes nil/zero values from an fx group of RCAgentTaskListener.
func FilterTaskListeners(group []RCAgentTaskListener) []RCAgentTaskListener {
	return fxutil.GetAndFilterGroup(group)
}

// FilterStatusListeners removes nil/zero values from an fx group of RCStatusListener.
func FilterStatusListeners(group []RCStatusListener) []RCStatusListener {
	return fxutil.GetAndFilterGroup(group)
}

// TaskListenerProvider defines component that can receive RC updates
type TaskListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentTaskListener"`
}

// NewTaskListener returns a TaskListenerProvider registering a RCAgentTaskListener listener to the RC group.
func NewTaskListener(listener RCAgentTaskListener) TaskListenerProvider {
	return TaskListenerProvider{
		Listener: listener,
	}
}

// ListenerProvider defines component that can receive RC updates for any product
type ListenerProvider struct {
	fx.Out

	ListenerProvider RCListener `group:"rCListener"`
}

// StatusListenerProvider registers an authoritative status listener with the RC client.
type StatusListenerProvider struct {
	fx.Out

	Listener RCStatusListener `group:"rCStatusListener"`
}
