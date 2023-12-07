// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcclient is a remote config client that can run within the agent to receive
// configurations.
package rcclient

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
	TaskFlare        TaskType = "flare"
	agentTaskTimeout          = 5 * time.Minute
)

// RCAgentTaskListener is the FX-compatible listener, so RC can push updates through it
type RCAgentTaskListener func(taskType TaskType, task AgentTaskConfig) (bool, error)

// RCListener is the generic type for components to register a callback for any product
type RCListener map[data.Product]func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))

type rcClient struct {
	client        *remote.Client
	m             *sync.Mutex
	taskProcessed map[string]bool

	listeners []RCListener
	// Tasks are separated from the other products, because they must be executed once
	taskListeners []RCAgentTaskListener
}

type dependencies struct {
	fx.In

	Log log.Component

	Listeners     []RCListener          `group:"rCListener"`          // <-- Fill automatically by Fx
	TaskListeners []RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
}

func newRemoteConfigClient(deps dependencies) (Component, error) {
	// We have to create the client in the constructor and set its name later
	c, err := remote.NewUnverifiedGRPCClient(
		"unknown", version.AgentVersion, []data.Product{}, 5*time.Second,
	)
	if err != nil {
		return nil, err
	}

	rc := rcClient{
		listeners:     deps.Listeners,
		taskListeners: deps.TaskListeners,
		m:             &sync.Mutex{},
		client:        c,
	}

	return rc, nil
}

// Listen subscribes to AGENT_CONFIG configurations and start the remote config client
func (rc rcClient) Start(agentName string) error {
	rc.client.SetAgentName(agentName)

	rc.client.Subscribe(state.ProductAgentConfig, rc.agentConfigUpdateCallback)

	// Register every product for every listener
	for _, l := range rc.listeners {
		for product, callback := range l {
			rc.client.Subscribe(string(product), callback)
		}
	}

	rc.client.Start()

	return nil
}

func (rc rcClient) SubscribeAgentTask() {
	rc.taskProcessed = map[string]bool{}
	if rc.client == nil {
		pkglog.Errorf("No remote-config client")
		return
	}
	rc.client.Subscribe(state.ProductAgentTask, rc.agentTaskUpdateCallback)
}

func (rc rcClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	rc.client.Subscribe(string(product), fn)
}

func (rc rcClient) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	mergedConfig, err := state.MergeRCAgentConfig(rc.client.UpdateApplyStatus, updates)
	if err != nil {
		return
	}

	// Checks who (the source) is responsible for the last logLevel change
	source := config.Datadog.GetSource("log_level")

	switch source {
	case model.SourceRC:
		// 2 possible situations:
		//     - we want to change (once again) the log level through RC
		//     - we want to fall back to the log level we had saved as fallback (in that case mergedConfig.LogLevel == "")
		if len(mergedConfig.LogLevel) == 0 {
			pkglog.Infof("Removing remote-config log level override, falling back to '%s'", config.Datadog.Get("log_level"))
			config.Datadog.UnsetForSource("log_level", model.SourceRC)
		} else {
			newLevel := mergedConfig.LogLevel
			pkglog.Infof("Changing log level to '%s' through remote config", newLevel)
			err = settings.SetRuntimeSetting("log_level", newLevel, model.SourceRC)
		}

	case model.SourceCLI:
		pkglog.Warnf("Remote config could not change the log level due to CLI override")
		return

	// default case handles every other source (lower in the hierarchy)
	default:
		// If we receive an empty value for log level in the config
		// then there is nothing to do
		if len(mergedConfig.LogLevel) == 0 {
			return
		}

		// Need to update the log level even if the level stays the same because we need to update the source
		// Might be possible to add a check in deeper functions to avoid unnecessary work
		err = settings.SetRuntimeSetting("log_level", mergedConfig.LogLevel, model.SourceRC)
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}
}

// agentTaskUpdateCallback is the callback function called when there is an AGENT_TASK config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func (rc rcClient) agentTaskUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	rc.m.Lock()
	defer rc.m.Unlock()

	wg := &sync.WaitGroup{}
	wg.Add(len(updates))

	// Executes all AGENT_TASK in separate routines, so we don't block if one of them deadlock
	for originalConfigPath, originalConfig := range updates {
		go func(configPath string, c state.RawConfig) {
			pkglog.Debugf("Agent task %s started", configPath)
			defer wg.Done()
			defer pkglog.Debugf("Agent task %s completed", configPath)
			task, err := parseConfigAgentTask(c.Config, c.Metadata)
			if err != nil {
				rc.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				return
			}

			// Check that the flare task wasn't already processed
			if !rc.taskProcessed[task.Config.UUID] {
				rc.taskProcessed[task.Config.UUID] = true

				// Mark it as unack first
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateUnacknowledged,
				})

				var err error
				var processed bool
				// Call all the listeners component
				for _, l := range rc.taskListeners {
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
					applyStateCallback(configPath, state.ApplyStatus{
						State: state.ApplyStateError,
						Error: err.Error(),
					})
				} else if processed && err == nil {
					// Only success
					applyStateCallback(configPath, state.ApplyStatus{
						State: state.ApplyStateAcknowledged,
					})
				} else {
					applyStateCallback(configPath, state.ApplyStatus{
						State: state.ApplyStateUnknown,
					})
				}
			}
		}(originalConfigPath, originalConfig)
	}

	// Check if one of the task reaches timeout
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		// completed normally
		pkglog.Debugf("All %d agent tasks were applied successfully", len(updates))
		return
	case <-time.After(agentTaskTimeout):
		// timed out
		pkglog.Warnf("Timeout of at least one agent task configuration")
	}
}

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
