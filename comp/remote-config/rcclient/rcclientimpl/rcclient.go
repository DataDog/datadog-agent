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

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"go.uber.org/fx"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigClient),
		fxutil.ProvideOptional[rcclient.Component](),
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
	config            configcomp.Component
	sysprobeConfig    option.Option[sysprobeconfig.Component]
	isSystemProbe     bool
}

type dependencies struct {
	fx.In

	Log log.Component
	Lc  fx.Lifecycle

	Params            rcclient.Params             `optional:"true"`
	Listeners         []types.RCListener          `group:"rCListener"`          // <-- Fill automatically by Fx
	TaskListeners     []types.RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
	SettingsComponent settings.Component
	Config            configcomp.Component
	SysprobeConfig    option.Option[sysprobeconfig.Component]
	IPC               ipc.Component
}

// newRemoteConfigClient must not populate any Fx groups or return any types that would be consumed as dependencies by
// other components. To avoid dependency cycles between our components we need to have "pure leaf" components (i.e.
// components that are instantiated last).  Remote configuration client is a good candidate for this since it must be
// able to interact with any other components (i.e. be at the end of the dependency graph).
func newRemoteConfigClient(deps dependencies) (rcclient.Component, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
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
		pkgconfigsetup.GetIPCPort(),
		deps.IPC.GetAuthToken(),
		deps.IPC.GetTLSClientConfig(),
		optsWithDefault...,
	)
	if err != nil {
		return nil, err
	}

	var clientMRF *client.Client
	if pkgconfigsetup.Datadog().GetBool("multi_region_failover.enabled") {
		clientMRF, err = client.NewUnverifiedMRFGRPCClient(
			ipcAddress,
			pkgconfigsetup.GetIPCPort(),
			deps.IPC.GetAuthToken(),
			deps.IPC.GetTLSClientConfig(),
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
		config:            deps.Config,
		sysprobeConfig:    deps.SysprobeConfig,
		isSystemProbe:     deps.Params.IsSystemProbe,
	}

	if pkgconfigsetup.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
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

// mrfUpdateCallback is the callback function for the AGENT_FAILOVER configs.
// It fetches all the configs targeting the agent and applies the failover settings
// using an OR strategy. In case of nil the value is not updated, for a false it does not update if
// the setting is already set to true.
//
// If a setting is not set via any config, it will fallback if the source was RC.
func (rc rcClient) mrfUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var enableLogs, enableMetrics, enableAPM *bool
	var enableLogsCfgPth, enableMetricsCfgPth, enableAPMCfgPth string
	for cfgPath, update := range updates {
		if (enableLogs != nil && *enableLogs) && (enableMetrics != nil && *enableMetrics) && (enableAPM != nil && *enableAPM) {
			break
		}

		mrfUpdate, err := parseMultiRegionFailoverConfig(update.Config)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover update unmarshal failed: %s", err)
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		if mrfUpdate == nil || (mrfUpdate.FailoverMetrics == nil && mrfUpdate.FailoverLogs == nil && mrfUpdate.FailoverAPM == nil) {
			continue
		}

		if !(enableMetrics != nil && *enableMetrics) && mrfUpdate.FailoverMetrics != nil {
			enableMetrics = mrfUpdate.FailoverMetrics
			enableMetricsCfgPth = cfgPath
		}

		if !(enableLogs != nil && *enableLogs) && mrfUpdate.FailoverLogs != nil {
			enableLogs = mrfUpdate.FailoverLogs
			enableLogsCfgPth = cfgPath
		}

		if !(enableAPM != nil && *enableAPM) && mrfUpdate.FailoverAPM != nil {
			enableAPM = mrfUpdate.FailoverAPM
			enableAPMCfgPth = cfgPath
		}
	}

	if enableMetrics != nil {
		err := rc.applyMRFRuntimeSetting("multi_region_failover.failover_metrics", *enableMetrics, enableMetricsCfgPth, applyStateCallback)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover failed to apply new metrics settings : %s", err)
			applyStateCallback(enableMetricsCfgPth, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			return
		}
		change := "disabled"
		if *enableMetrics {
			change = "enabled"
		}
		pkglog.Infof("Received remote update for Multi-Region Failover configuration: %s failover for metrics", change)
		applyStateCallback(enableMetricsCfgPth, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	} else {
		mrfFailoverMetricsSource := pkgconfigsetup.Datadog().GetSource("multi_region_failover.failover_metrics")
		pkgconfigsetup.Datadog().UnsetForSource("multi_region_failover.failover_metrics", model.SourceRC)
		if mrfFailoverMetricsSource == model.SourceRC {
			pkglog.Infof("Falling back to `multi_region_failover.failover_metrics: %t`", pkgconfigsetup.Datadog().GetBool("multi_region_failover.failover_metrics"))
		}
	}

	if enableLogs != nil {
		err := rc.applyMRFRuntimeSetting("multi_region_failover.failover_logs", *enableLogs, enableLogsCfgPth, applyStateCallback)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover failed to apply new logs settings : %s", err)
			applyStateCallback(enableLogsCfgPth, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			return
		}
		change := "disabled"
		if *enableLogs {
			change = "enabled"
		}
		pkglog.Infof("Received remote update for Multi-Region Failover configuration: %s failover for logs", change)
		applyStateCallback(enableLogsCfgPth, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	} else {
		mrfFailoverLogsSource := pkgconfigsetup.Datadog().GetSource("multi_region_failover.failover_logs")
		pkgconfigsetup.Datadog().UnsetForSource("multi_region_failover.failover_logs", model.SourceRC)
		if mrfFailoverLogsSource == model.SourceRC {
			pkglog.Infof("Falling back to `multi_region_failover.failover_logs: %t`", pkgconfigsetup.Datadog().GetBool("multi_region_failover.failover_logs"))
		}
	}

	if enableAPM != nil {
		err := rc.applyMRFRuntimeSetting("multi_region_failover.failover_apm", *enableAPM, enableAPMCfgPth, applyStateCallback)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover failed to apply new apm settings : %s", err)
			applyStateCallback(enableAPMCfgPth, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			return
		}
		change := "disabled"
		if *enableAPM {
			change = "enabled"
		}
		pkglog.Infof("Received remote update for Multi-Region Failover configuration: %s failover for apm", change)
		applyStateCallback(enableAPMCfgPth, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	} else {
		mrfFailoverAPMSource := pkgconfigsetup.Datadog().GetSource("multi_region_failover.failover_apm")
		pkgconfigsetup.Datadog().UnsetForSource("multi_region_failover.failover_apm", model.SourceRC)
		if mrfFailoverAPMSource == model.SourceRC {
			pkglog.Infof("Falling back to `multi_region_failover.failover_apm: %t`", pkgconfigsetup.Datadog().GetBool("multi_region_failover.failover_apm"))
		}
	}
}

func (rc rcClient) applyMRFRuntimeSetting(setting string, value bool, cfgPath string, applyStateCallback func(string, state.ApplyStatus)) error {
	pkglog.Debugf("Setting `%s: %t` through remote config", setting, value)
	err := rc.settingsComponent.SetRuntimeSetting(setting, value, model.SourceRC)
	if err != nil {
		pkglog.Errorf("Failed to set %s runtime setting to %t: %s", setting, value, err)
		applyStateCallback(cfgPath, state.ApplyStatus{
			State: state.ApplyStateError,
			Error: err.Error(),
		})
	}
	return err
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

	var errs error

	targetCmp := rc.config
	localSysProbeConf, isSet := rc.sysprobeConfig.Get()
	if isSet && rc.isSystemProbe {
		pkglog.Infof("Using system probe config for remote config")
		targetCmp = localSysProbeConf
	}

	// Checks who (the source) is responsible for the last logLevel change
	source := targetCmp.GetSource("log_level")

	pkglog.Infof("A new log level configuration has been received through remote config, (source: %s, log_level '%s')", source, mergedConfig.LogLevel)

	switch source {
	case model.SourceRC:
		// 2 possible situations:
		//     - we want to change (once again) the log level through RC
		//     - we want to fall back to the log level we had saved as fallback (in that case mergedConfig.LogLevel == "")
		if len(mergedConfig.LogLevel) == 0 {
			targetCmp.UnsetForSource("log_level", model.SourceRC)
			pkglog.Infof("Removing remote-config log level override, falling back to '%s'", targetCmp.Get("log_level"))
		} else {
			newLevel := mergedConfig.LogLevel
			pkglog.Infof("Changing log level to '%s' through remote config", newLevel)
			if err := rc.settingsComponent.SetRuntimeSetting("log_level", newLevel, model.SourceRC); err != nil {
				errs = multierror.Append(errs, err)
			}
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
		pkglog.Infof("Changing log level to '%s' through remote config (new source)", mergedConfig.LogLevel)
		if err := rc.settingsComponent.SetRuntimeSetting("log_level", mergedConfig.LogLevel, model.SourceRC); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if errs == nil {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			err := fmt.Errorf("error while applying remote config: %s", errs.Error())
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

			rc.m.Lock()
			// Check that the flare task wasn't already processed
			if !rc.taskProcessed[task.Config.UUID] {
				rc.taskProcessed[task.Config.UUID] = true
				rc.m.Unlock()

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
						pkglog.Errorf("Error while processing agent task %s: %s", configPath, oneErr)
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
			} else {
				rc.m.Unlock()
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
