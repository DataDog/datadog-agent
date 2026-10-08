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
	// TaskTriggerPayloads is the task sent to request the agent to send some payloads immediately
	TaskTriggerPayloads TaskType = "trigger_payloads"
)

// AgentTaskConfig is a deserialized agent task configuration file
// along with the associated metadata
type AgentTaskConfig struct {
	Config   agentTaskData
	Metadata state.Metadata
}

// agentTaskData is the content of a agent task configuration file
type agentTaskData struct {
	TaskType string `json:"task_type"`
	UUID     string `json:"uuid"`
	// TaskArgs contains the string arguments of the task
	TaskArgs map[string]string `json:"-"`
	// RawTaskArgs contains all the arguments of the task, whatever their JSON type
	RawTaskArgs map[string]json.RawMessage `json:"args"`
}

// UnmarshalJSON decodes the task arguments, keeping the string ones in TaskArgs
func (d *agentTaskData) UnmarshalJSON(data []byte) error {
	type alias agentTaskData
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*d = agentTaskData(a)

	if d.RawTaskArgs != nil {
		d.TaskArgs = make(map[string]string, len(d.RawTaskArgs))
		for k, raw := range d.RawTaskArgs {
			var str string
			if err := json.Unmarshal(raw, &str); err == nil {
				d.TaskArgs[k] = str
			}
		}
	}
	return nil
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

// PartialFailureError is returned by a RCAgentTaskListener when the task was handled but partially failed.
// The task is then acknowledged, with the error reported to remote-config.
type PartialFailureError struct {
	Err error
}

// NewPartialFailureError wraps err in a PartialFailureError
func NewPartialFailureError(err error) error {
	return &PartialFailureError{Err: err}
}

func (e *PartialFailureError) Error() string {
	if e.Err == nil {
		return "partial failure"
	}
	return "partial failure: " + e.Err.Error()
}

func (e *PartialFailureError) Unwrap() error {
	return e.Err
}

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// RCListener is the generic type for components to register a callback for any product
type RCListener map[data.Product]func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))

// FilterListeners removes nil/zero values from an fx group of RCListener.
func FilterListeners(group []RCListener) []RCListener {
	return fxutil.GetAndFilterGroup(group)
}

// FilterTaskListeners removes nil/zero values from an fx group of RCAgentTaskListener.
func FilterTaskListeners(group []RCAgentTaskListener) []RCAgentTaskListener {
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
