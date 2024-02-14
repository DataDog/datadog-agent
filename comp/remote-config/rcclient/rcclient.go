// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcclient is a remote config client that can run within the agent to receive
// configurations.
package rcclient

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"
	yamlv2 "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
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
	client        *client.Client
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

type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

func newRemoteConfigClient(deps dependencies) (provides, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return provides{}, err
	}

	// We have to create the client in the constructor and set its name later
	c, err := client.NewUnverifiedGRPCClient(
		ipcAddress,
		config.GetIPCPort(),
		security.FetchAuthToken,
		client.WithAgent("unknown", version.AgentVersion),
		client.WithPollInterval(5*time.Second),
	)
	if err != nil {
		return provides{}, err
	}

	rc := rcClient{
		listeners:     deps.Listeners,
		taskListeners: deps.TaskListeners,
		m:             &sync.Mutex{},
		client:        c,
	}

	return provides{
		Comp:           rc,
		StatusProvider: status.NewInformationProvider(rc),
	}, nil
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

func (rc rcClient) SubscribeApmTracing() {
	if rc.client == nil {
		pkglog.Errorf("No remote-config client")
		return
	}
	rc.client.Subscribe(state.ProductAPMTracing, rc.onAPMTracingUpdate)
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

func (rc rcClient) onAPMTracingUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) { //nolint:revive
	if len(update) == 0 {
		// Empty update means revert to default behavior, so remove any existing config file
		err := os.Remove(apmTracingFilePath)
		if err == nil {
			pkglog.Infof("Removed APM_TRACING remote config file, APM injection will revert to default behavior")
		} else if !os.IsNotExist(err) {
			// If the file already wasn't there then it wasn't an error
			pkglog.Errorf("Failed to remove APM_TRACING remote config file, previous APM injection behavior will continue: %v", err)
		}
		return
	}

	var senvConfigs []serviceEnvConfig
	// Maps update IDs to their error, empty string indicates success
	updateStatus := map[string]string{}
	var hostTracingEnabled bool
	var hostEnvTarget string
	var hostConfigID string
	for id, rawConfig := range update {
		tcu := tracingConfigUpdate{}
		err := json.Unmarshal(rawConfig.Config, &tcu)
		updateStatus[id] = ""
		if err != nil {
			pkglog.Warnf("Skipping invalid APM_TRACING remote update %s: %v, any err: %v", id, tcu, err)
			updateStatus[id] = InvalidAPMTracingPayload
			continue
		}
		pkglog.Infof("Received APM_TRACING remote update %s: %v, any err: %v", id, tcu, err)
		if tcu.InfraTarget != nil {
			// This is an infra targeting payload, skip adding it to the service env config map
			if hostConfigID != "" && tcu.LibConfig.Env != hostEnvTarget {
				// We already saw a InfraTarget configuration and the envs are different, this is generally not desired
				// To be consistent we will apply the "lowest" config ID and report a failure for the un-applied host config
				pkglog.Warnf("Received more than 1 InfraTarget APM_TRACING config, the 'lowest' config will be used, but inconsistent behavior may occur. Check your Single Step Instrumentation configurations.")
				if id < hostConfigID {
					updateStatus[hostConfigID] = DuplicateHostConfig
					// fallthrough to use this update's config values
				} else {
					// The previous infra target was lower, keep the current values
					updateStatus[id] = DuplicateHostConfig
					continue
				}
			}
			hostTracingEnabled = tcu.LibConfig.TracingEnabled
			hostEnvTarget = tcu.LibConfig.Env
			hostConfigID = id
			continue
		}
		if tcu.ServiceTarget == nil {
			pkglog.Warnf("Missing service_target from APM_TRACING config update, SKIPPING: %v", tcu)
			updateStatus[id] = MissingServiceTarget
			continue
		}
		senvConfigs = append(senvConfigs, serviceEnvConfig{
			Service:        tcu.ServiceTarget.Service,
			Env:            tcu.ServiceTarget.Env,
			TracingEnabled: tcu.LibConfig.TracingEnabled,
		})
	}
	tec := tracingEnabledConfig{
		TracingEnabled:    hostTracingEnabled,
		Env:               hostEnvTarget,
		ServiceEnvConfigs: senvConfigs,
	}
	configFile, err := yamlv2.Marshal(tec)
	if err != nil {
		pkglog.Errorf("Failed to marshal APM_TRACING config update %v", err)
		return
	}
	err = os.WriteFile(apmTracingFilePath, configFile, 0644)
	if err != nil {
		pkglog.Errorf("Failed to write single step config data file from APM_TRACING config: %v", err)
		// Failed to write file, report failure for all updates
		for id := range update {
			applyStateCallback(id, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: FileWriteFailure,
			})
		}
		return
	}
	pkglog.Debugf("Successfully wrote APM_TRACING config to %s", apmTracingFilePath)
	// Successfully wrote file, report success/failure per update
	for id, errStatus := range updateStatus {
		applyState := state.ApplyStateAcknowledged
		if errStatus != "" {
			applyState = state.ApplyStateError
		}
		applyStateCallback(id, state.ApplyStatus{
			State: applyState,
			Error: errStatus,
		})
	}

}
