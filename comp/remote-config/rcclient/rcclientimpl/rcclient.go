// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcclientimpl is a remote config client that can run within the agent to receive
// configurations.
package rcclientimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigClient),
		fx.Provide(func(c rcclient.Component) optional.Option[rcclient.Component] {
			return optional.NewOption[rcclient.Component](c)
		}),
	)
}

const (
	agentTaskTimeout = 5 * time.Minute
)

type rcClient struct {
	client        *client.Client
	clientMRF     *client.Client
	m             *sync.Mutex
	taskProcessed map[string]bool

	listeners []types.RCListener
	// Tasks are separated from the other products, because they must be executed once
	taskListeners     []types.RCAgentTaskListener
	settingsComponent settings.Component
}

type dependencies struct {
	fx.In

	Log log.Component
	Lc  fx.Lifecycle

	Params            rcclient.Params             `optional:"true"`
	Listeners         []types.RCListener          `group:"rCListener"`          // <-- Fill automatically by Fx
	TaskListeners     []types.RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
	SettingsComponent settings.Component
}

// newRemoteConfigClient must not populate any Fx groups or return any types that would be consumed as dependencies by
// other components. To avoid dependency cycles between our components we need to have "pure leaf" components (i.e.
// components that are instantiated last).  Remote configuration client is a good candidate for this since it must be
// able to interact with any other components (i.e. be at the end of the dependency graph).
func newRemoteConfigClient(deps dependencies) (rcclient.Component, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	if deps.Params.AgentName == "" || deps.Params.AgentVersion == "" {
		return nil, fmt.Errorf("Remote config client is missing agent name or version parameter")
	}

	// Append client options
	optsWithDefault := []func(*client.Options){
		client.WithPollInterval(5 * time.Second),
		client.WithAgent(deps.Params.AgentName, deps.Params.AgentVersion),
	}

	// We have to create the client in the constructor and set its name later
	c, err := client.NewUnverifiedGRPCClient(
		ipcAddress,
		config.GetIPCPort(),
		func() (string, error) { return security.FetchAuthToken(config.Datadog) },
		optsWithDefault...,
	)
	if err != nil {
		return nil, err
	}

	var clientMRF *client.Client
	if config.Datadog.GetBool("multi_region_failover.enabled") {
		clientMRF, err = client.NewUnverifiedMRFGRPCClient(
			ipcAddress,
			config.GetIPCPort(),
			func() (string, error) { return security.FetchAuthToken(config.Datadog) },
			optsWithDefault...,
		)
		if err != nil {
			return nil, err
		}
	}

	rc := rcClient{
		listeners:         fxutil.GetAndFilterGroup(deps.Listeners),
		taskListeners:     fxutil.GetAndFilterGroup(deps.TaskListeners),
		m:                 &sync.Mutex{},
		client:            c,
		clientMRF:         clientMRF,
		settingsComponent: deps.SettingsComponent,
	}

	if config.IsRemoteConfigEnabled(config.Datadog) {
		deps.Lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				rc.start()
				return nil
			},
		})
	}

	deps.Lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			rc.client.Close()
			return nil
		},
	})

	return rc, nil
}

// Start subscribes to AGENT_CONFIG configurations and start the remote config client
func (rc rcClient) start() {
	rc.client.Subscribe(state.ProductAgentConfig, rc.agentConfigUpdateCallback)

	// Register every product for every listener
	for _, l := range rc.listeners {
		for product, callback := range l {
			rc.client.Subscribe(string(product), callback)
		}
	}

	rc.client.Start()

	if rc.clientMRF != nil {
		rc.clientMRF.Subscribe(state.ProductAgentFailover, rc.mrfUpdateCallback)
		rc.clientMRF.Start()
	}
}

func (rc rcClient) mrfUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var failover *bool
	applied := false
	for cfgPath, update := range updates {
		mrfUpdate, err := parseMultiRegionFailoverConfig(update.Config)
		if err != nil {
			pkglog.Errorf("MRF update unmarshal failed: %s", err)
			continue
		}
		if mrfUpdate != nil && mrfUpdate.Failover != nil {
			if applied {
				pkglog.Infof("Multiple MRF updates received, disregarding update of `multi_region_failover.failover_metrics`/`multi_region_failover.failover_logs` to %t", *mrfUpdate.Failover)
				continue
			}
			failover = mrfUpdate.Failover
			pkglog.Infof("Setting `multi_region_failover.failover_metrics` and `multi_region_failover.failover_logs` to `%t` through Remote Configuration", *failover)

			applyError := false
			for _, setting := range []string{"multi_region_failover.failover_metrics", "multi_region_failover.failover_logs"} {
				// Don't try to apply any further if we already encountered an error while applying it to a specific setting.
				if !applyError {
					err = rc.settingsComponent.SetRuntimeSetting(setting, *failover, model.SourceRC)
					if err != nil {
						pkglog.Errorf("MRF failover update failed: %s", err)
						applyError = true
						applyStateCallback(cfgPath, state.ApplyStatus{
							State: state.ApplyStateError,
							Error: err.Error(),
						})
					}
				}
			}

			if !applyError {
				applied = true
				applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			}
		}
	}

	// Config update is nil, so we should unset the failover value if it was set via RC previously
	if failover == nil {
		for _, setting := range []string{"multi_region_failover.failover_metrics", "multi_region_failover.failover_logs"} {
			// Determine the current source of the multi_region_failover.failover cfg value
			mrfCfgSource := config.Datadog.GetSource(setting)

			// Unset the RC-sourced setting value regardless of what it is
			config.Datadog.UnsetForSource(setting, model.SourceRC)

			// If the failover setting value was previously set via RC, log the current value now that we've unset it
			if mrfCfgSource == model.SourceRC {
				pkglog.Infof("Falling back to `%s: %t`", setting, config.Datadog.GetBool(setting))
			}
		}
	}
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
			err = rc.settingsComponent.SetRuntimeSetting("log_level", newLevel, model.SourceRC)
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
		err = rc.settingsComponent.SetRuntimeSetting("log_level", mergedConfig.LogLevel, model.SourceRC)
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
			task, err := types.ParseConfigAgentTask(c.Config, c.Metadata)
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
					oneProcessed, oneErr := l(types.TaskType(task.Config.TaskType), task)
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
