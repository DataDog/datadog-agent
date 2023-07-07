// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclient

import (
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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
	configState   *state.AgentConfigState

	listeners []RCAgentTaskListener
}

type dependencies struct {
	fx.In

	Log log.Component

	Listeners []RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
}

func newRemoteConfigClient(deps dependencies) (Component, error) {
	level, err := pkglog.GetLogLevel()
	if err != nil {
		return nil, err
	}

	rc := rcClient{
		listeners: deps.Listeners,
		m:         &sync.Mutex{},
		configState: &state.AgentConfigState{
			FallbackLogLevel: level.String(),
		},
		client: nil,
	}

	return rc, nil
}

// Listen start the remote config client to listen to AGENT_TASK configurations
func (rc rcClient) Listen(clientName string, products []data.Product) error {
	c, err := remote.NewUnverifiedGRPCClient(
		clientName, version.AgentVersion, products, 1*time.Second,
	)
	if err != nil {
		return err
	}

	rc.client = c
	rc.taskProcessed = map[string]bool{}

	for _, product := range products {
		switch product {
		case state.ProductAgentTask:
			rc.client.Subscribe(state.ProductAgentTask, rc.agentTaskUpdateCallback)
			break
		case state.ProductAgentConfig:
			rc.client.Subscribe(state.ProductAgentConfig, rc.agentConfigUpdateCallback)
			break
		default:
			pkglog.Infof("remote config client %s started unsupported product: %s", clientName, product)
		}
	}

	rc.client.Start()

	return nil
}

func (rc rcClient) agentConfigUpdateCallback(updates map[string]state.RawConfig) {
	mergedConfig, err := state.MergeRCAgentConfig(rc.client.UpdateApplyStatus, updates)
	if err != nil {
		return
	}

	// TODO RCM-1064: implement priority between CLI and remote-config
	// If there is no error, override the configs
	if len(mergedConfig.LogLevel) > 0 && mergedConfig.LogLevel != rc.configState.LatestLogLevel {
		pkglog.Infof("Changing log level to %s through remote config", mergedConfig.LogLevel)
		// Get the current log level
		var newFallback seelog.LogLevel
		newFallback, err = pkglog.GetLogLevel()
		if err == nil {
			rc.configState.FallbackLogLevel = newFallback.String()
			err = settings.SetRuntimeSetting("log_level", mergedConfig.LogLevel)
			rc.configState.LatestLogLevel = mergedConfig.LogLevel
		}
	} else {
		var currentLogLevel seelog.LogLevel
		currentLogLevel, err = pkglog.GetLogLevel()
		if err == nil && currentLogLevel.String() == rc.configState.LatestLogLevel {
			pkglog.Infof("Removing remote-config log level override, falling back to %s", rc.configState.FallbackLogLevel)
			err = settings.SetRuntimeSetting("log_level", rc.configState.FallbackLogLevel)
		}
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			rc.client.UpdateApplyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			rc.client.UpdateApplyStatus(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}
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
					if err == nil {
						err = oneErr
					} else {
						err = errors.Wrap(oneErr, err.Error())
					}
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
