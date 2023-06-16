// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// TaskType contains the type of the remote config task to execute
type TaskType string

const (
	// TaskFlare is the task sent to request a flare from the agent
	TaskFlare TaskType = "flare"
)

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

type rcClient struct {
	client        *remote.Client
	m             *sync.Mutex
	taskProcessed map[string]bool

	listeners []RCAgentTaskListener
}

type dependencies struct {
	fx.In

	Listeners []RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
}

func newRemoteConfigClient(deps dependencies) (Component, error) {
	rc := rcClient{
		listeners: deps.Listeners,
		m:         &sync.Mutex{},
		client:    nil,
	}

	return rc, nil
}

// Listen start the remote config client to listen to AGENT_TASK configurations
func (rc rcClient) Listen() error {
	c, err := remote.NewUnverifiedGRPCClient(
		"core-agent", version.AgentVersion, []data.Product{data.ProductAgentTask}, 1*time.Second,
	)
	if err != nil {
		return err
	}

	rc.client = c
	rc.taskProcessed = map[string]bool{}

	rc.client.Subscribe(state.ProductAgentTask, rc.agentTaskUpdateCallback)

	rc.client.Start()

	return nil
}

// agentTaskUpdateCallback is the callback function called when there is an AGENT_TASK config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func (rc rcClient) agentTaskUpdateCallback(updates map[string]state.RawConfig) {
	rc.m.Lock()
	defer rc.m.Unlock()
	for configPath, c := range updates {
		task, err := parseConfigAgentTask(c.Config, c.Metadata)
		if err != nil {
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		// Check that the flare task wasn't already processed
		if !rc.taskProcessed[task.Config.UUID] {
			rc.taskProcessed[task.Config.UUID] = true

			// Mark it as unack first
			rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
			})

			var err error
			var processed bool
			// Call all the listeners component
			for _, l := range rc.listeners {
				oneProcessed, oneErr := l(TaskType(task.Config.TaskType), task)
				// Check if the task was processed at least once
				processed = oneProcessed || processed
				if oneErr != nil {
					err = errors.Wrap(err, oneErr.Error())
				}
			}
			if processed && err != nil {
				// One failure
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
			} else if processed && err == nil {
				// Only success
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			} else {
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateUnknown,
				})
			}
		}
	}
}

// ListenerProvider defines component that can receive RC updates
type ListenerProvider struct {
	fx.Out

	Listener RCAgentTaskListener `group:"rCAgentTaskListener"`
}
