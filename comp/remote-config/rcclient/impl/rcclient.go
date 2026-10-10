// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package rcclientimpl is a remote config client that can run within the agent to receive
// configurations.
package rcclientimpl

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	agentTaskTimeout            = 5 * time.Minute
	failoverMetricsSetting      = "multi_region_failover.failover_metrics"
	failoverLogsSetting         = "multi_region_failover.failover_logs"
	failoverAPMSetting          = "multi_region_failover.failover_apm"
	metricsAllowlistSetting     = "multi_region_failover.metric_allowlist"
	logsServiceAllowlistSetting = "multi_region_failover.logs_service_allowlist"
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
	agentName         string
	agentVersion      string
	IPC               ipc.Component
}

var _ rcclient.TUFProofProvider = (*rcClient)(nil)

// Dependencies defines the dependencies for the rcclient component.
type Dependencies struct {
	compdef.In

	Log log.Component
	Lc  compdef.Lifecycle

	Params            rcclient.Params             `optional:"true"`
	Listeners         []types.RCListener          `group:"rCListener"`          // <-- Fill automatically by Fx
	TaskListeners     []types.RCAgentTaskListener `group:"rCAgentTaskListener"` // <-- Fill automatically by Fx
	SettingsComponent settings.Component
	Config            configcomp.Component
	SysprobeConfig    option.Option[sysprobeconfig.Component]
	IPC               ipc.Component
}

// NewComponent must not populate any Fx groups or return any types that would be consumed as dependencies by
// other components. To avoid dependency cycles between our components we need to have "pure leaf" components (i.e.
// components that are instantiated last).  Remote configuration client is a good candidate for this since it must be
// able to interact with any other components (i.e. be at the end of the dependency graph).
func NewComponent(deps Dependencies) (rcclient.Component, error) {
	if deps.Params.AgentName == "" || deps.Params.AgentVersion == "" {
		return nil, errors.New("Remote config client is missing agent name or version parameter")
	}

	rc := &rcClient{
		listeners:         types.FilterListeners(deps.Listeners),
		taskListeners:     types.FilterTaskListeners(deps.TaskListeners),
		m:                 &sync.Mutex{},
		settingsComponent: deps.SettingsComponent,
		config:            deps.Config,
		sysprobeConfig:    deps.SysprobeConfig,
		isSystemProbe:     deps.Params.IsSystemProbe,
		agentName:         deps.Params.AgentName,
		agentVersion:      deps.Params.AgentVersion,
		IPC:               deps.IPC,
	}

	if configUtils.IsRemoteConfigEnabled(deps.Config) {
		deps.Lc.Append(compdef.Hook{
			OnStart: func(context.Context) error {
				return rc.start()
			},
		})
	}

	deps.Lc.Append(compdef.Hook{
		OnStop: func(context.Context) error {
			if rc.client != nil {
				rc.client.Close()
			}
			return nil
		},
	})

	return rc, nil
}

func (rc *rcClient) createGRPCClient() error {
	ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}

	// Append client options
	optsWithDefault := []func(*client.Options){
		client.WithPollInterval(5 * time.Second),
		client.WithAgent(rc.agentName, rc.agentVersion),
	}

	rc.client, err = client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfighelper.GetIPCPort(pkgconfigsetup.Datadog()),
		rc.IPC.GetAuthToken(),
		rc.IPC.GetTLSClientConfig(),
		optsWithDefault...,
	)
	if err != nil {
		return err
	}

	if pkgconfigsetup.Datadog().GetBool("multi_region_failover.enabled") {
		rc.clientMRF, err = client.NewUnverifiedMRFGRPCClient(
			ipcAddress,
			pkgconfighelper.GetIPCPort(pkgconfigsetup.Datadog()),
			rc.IPC.GetAuthToken(),
			rc.IPC.GetTLSClientConfig(),
			optsWithDefault...,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// start subscribes to AGENT_CONFIG configurations and starts the remote config client
func (rc *rcClient) start() error {
	if err := rc.createGRPCClient(); err != nil {
		return err

	}
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

	return nil
}

// mrfUpdateCallback is the callback function for the AGENT_FAILOVER configs.
// It fetches all the configs targeting the agent and applies the failover settings
// using an OR strategy. In case of nil the value is not updated, for a false it does not update if
// the setting is already set to true.
//
// If a setting is not set via any config, it will fallback if the source was RC.
func (rc *rcClient) mrfUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var enableLogs, enableMetrics, enableAPM *bool
	var enableLogsCfgPth, enableMetricsCfgPth, enableAPMCfgPth string
	// Configs setting an allowlist. Empty when none does, in which case the setting falls back.
	var metricsAllowlistCfgPths, logsServiceAllowlistCfgPths []string
	allowedMetrics := make(map[string]struct{})
	allowedServices := make(map[string]struct{})

	// A config may contribute to several settings. Once one of its settings failed to apply, a later
	// setting that applied must not turn its status back to acknowledged: an error sticks.
	errored := make(map[string]struct{})
	reportApplyStatus := func(cfgPath string, status state.ApplyStatus) {
		if status.State == state.ApplyStateError {
			errored[cfgPath] = struct{}{}
		} else if _, failed := errored[cfgPath]; failed {
			return
		}
		applyStateCallback(cfgPath, status)
	}

	for cfgPath, update := range updates {
		mrfUpdate, err := parseMultiRegionFailoverConfig(update.Config)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover update unmarshal failed: %s", err)
			reportApplyStatus(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		if mrfUpdate == nil || (mrfUpdate.FailoverMetrics == nil &&
			mrfUpdate.FailoverLogs == nil &&
			mrfUpdate.FailoverAPM == nil &&
			mrfUpdate.MetricsAllowlist == nil &&
			mrfUpdate.LogsServiceAllowlist == nil) {
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

		// The allowlists of all configs are merged. As with the configuration file, an empty allowlist
		// does not filter: every metric, or every log, is forwarded while the matching failover is enabled.
		if mrfUpdate.MetricsAllowlist != nil {
			metricsAllowlistCfgPths = append(metricsAllowlistCfgPths, cfgPath)
			for _, metric := range mrfUpdate.MetricsAllowlist {
				allowedMetrics[metric] = struct{}{}
			}
		}

		if mrfUpdate.LogsServiceAllowlist != nil {
			logsServiceAllowlistCfgPths = append(logsServiceAllowlistCfgPths, cfgPath)
			for _, service := range mrfUpdate.LogsServiceAllowlist {
				allowedServices[service] = struct{}{}
			}
		}
	}

	// A failed write stops its pair (see applyMRFFailover) but not the other pairs: the settings are
	// independent, and the error is reported on the configs that contributed to the failed setting.
	rc.applyMRFFailover(mrfFailoverUpdate{
		what:              "metrics",
		flagSetting:       failoverMetricsSetting,
		enable:            enableMetrics,
		enableCfgPath:     enableMetricsCfgPth,
		allowlistSetting:  metricsAllowlistSetting,
		allowed:           allowedMetrics,
		allowlistCfgPaths: metricsAllowlistCfgPths,
	}, reportApplyStatus)

	rc.applyMRFFailover(mrfFailoverUpdate{
		what:              "logs",
		flagSetting:       failoverLogsSetting,
		enable:            enableLogs,
		enableCfgPath:     enableLogsCfgPth,
		allowlistSetting:  logsServiceAllowlistSetting,
		allowed:           allowedServices,
		allowlistCfgPaths: logsServiceAllowlistCfgPths,
	}, reportApplyStatus)

	rc.applyMRFFailoverFlag(failoverAPMSetting, "apm", enableAPM, enableAPMCfgPth, reportApplyStatus)
}

// mrfFailoverUpdate is the update to apply to a failover flag and to the allowlist that goes with it.
type mrfFailoverUpdate struct {
	what              string // "metrics" or "logs", for the log lines
	flagSetting       string
	enable            *bool // nil when no config sets the flag, which then falls back
	enableCfgPath     string
	allowlistSetting  string
	allowed           map[string]struct{}
	allowlistCfgPaths []string // configs setting the allowlist; empty when none does, which then falls back
}

// applyMRFFailover applies a failover flag and its allowlist.
//
// The two settings are written one after the other, and each write publishes a configuration of its
// own, which the logs processors are notified of. The writes are ordered from the current state so that
// no published configuration forwards data that neither the previous nor the new configuration forwards:
//   - while failover is active, the flag is applied first: it can only stay on or turn off, and
//     once off the allowlist change forwards nothing;
//   - while failover is inactive, the allowlist is applied first: the flag can only stay off or
//     turn on, and it turns on with the new allowlist already in place.
//
// This holds whether a setting comes from the configs or falls back because no config sets it. It is a
// guarantee on the published configurations, not on a consumer that reads the two settings separately.
//
// The pair stops at its first failed write and returns false: on enable the allowlist comes first, so a
// failed allowlist write never turns the flag on; on disable the flag comes first, so a failed flag write
// never widens the allowlist while the flag is still on.
func (rc *rcClient) applyMRFFailover(update mrfFailoverUpdate, applyStateCallback func(string, state.ApplyStatus)) bool {
	applyFlag := func() bool {
		return rc.applyMRFFailoverFlag(update.flagSetting, update.what, update.enable, update.enableCfgPath, applyStateCallback)
	}
	applyAllowlist := func() bool {
		return rc.applyMRFAllowlist(update.allowlistSetting, update.allowed, update.allowlistCfgPaths, applyStateCallback)
	}

	if pkgconfigsetup.Datadog().GetBool(update.flagSetting) {
		return applyFlag() && applyAllowlist()
	}
	return applyAllowlist() && applyFlag()
}

// applyMRFFailoverFlag applies a failover flag, or unsets its remote config value when no config sets it.
// It returns false when the setting could not be applied.
func (rc *rcClient) applyMRFFailoverFlag(setting, what string, enable *bool, cfgPath string, applyStateCallback func(string, state.ApplyStatus)) bool {
	if enable == nil {
		rc.unsetMRFRuntimeSetting(setting)
		return true
	}

	if err := rc.applyMRFRuntimeSetting(setting, *enable, applyStateCallback, cfgPath); err != nil {
		pkglog.Errorf("Multi-Region Failover failed to apply new %s settings : %s", what, err)
		return false
	}
	change := "disabled"
	if *enable {
		change = "enabled"
	}
	pkglog.Infof("Received remote update for Multi-Region Failover configuration: %s failover for %s", change, what)
	return true
}

// applyMRFAllowlist applies an allowlist merged from the configs that set it, or unsets its remote
// config value when no config sets it. It returns false when the setting could not be applied.
func (rc *rcClient) applyMRFAllowlist(setting string, allowed map[string]struct{}, cfgPaths []string, applyStateCallback func(string, state.ApplyStatus)) bool {
	if len(cfgPaths) == 0 {
		rc.unsetMRFRuntimeSetting(setting)
		return true
	}

	// Sorted, so that the same set of entries is the same value whatever the order of the configs.
	allowlist := make([]string, 0, len(allowed))
	for entry := range allowed {
		allowlist = append(allowlist, entry)
	}
	slices.Sort(allowlist)

	if err := rc.applyMRFRuntimeSetting(setting, allowlist, applyStateCallback, cfgPaths...); err != nil {
		pkglog.Errorf("Multi-Region Failover failed to apply new `%s` : %s", setting, err)
		return false
	}
	pkglog.Infof("Received remote update for Multi-Region Failover configuration: `%s` updated (%d entries)", setting, len(allowlist))
	return true
}

// unsetMRFRuntimeSetting removes the remote config value of a setting, which then falls back to its other sources.
func (rc *rcClient) unsetMRFRuntimeSetting(setting string) {
	source := pkgconfigsetup.Datadog().GetSource(setting)
	pkgconfigsetup.Datadog().UnsetForSource(setting, model.SourceRC)
	if source == model.SourceRC {
		pkglog.Infof("Falling back to `%s: %v`", setting, pkgconfigsetup.Datadog().Get(setting))
	}
}

// applyMRFRuntimeSetting sets a runtime setting through remote config and reports the outcome for
// every config the value comes from.
func (rc *rcClient) applyMRFRuntimeSetting(setting string, value any, applyStateCallback func(string, state.ApplyStatus), cfgPaths ...string) error {
	pkglog.Debugf("Setting `%s: %v` through remote config", setting, value)
	err := rc.settingsComponent.SetRuntimeSetting(setting, value, model.SourceRC)
	status := state.ApplyStatus{State: state.ApplyStateAcknowledged}
	if err != nil {
		pkglog.Errorf("Failed to set %s runtime setting to %v: %s", setting, value, err)
		status = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
	}
	for _, cfgPath := range cfgPaths {
		applyStateCallback(cfgPath, status)
	}
	return err
}

// SubscribeAgentTask subscribes the remote-config client to AGENT_TASK
func (rc *rcClient) SubscribeAgentTask() {
	rc.taskProcessed = map[string]bool{}
	if rc.client == nil {
		pkglog.Errorf("No remote-config client")
		return
	}
	rc.client.Subscribe(state.ProductAgentTask, rc.agentTaskUpdateCallback)
}

// Subscribe is the generic way to start listening to a specific product update
func (rc *rcClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	if rc.client == nil {
		pkglog.Errorf("No remote-config client")
		return
	}
	rc.client.Subscribe(string(product), fn)
}

// GetConfigTUFProof returns the current Director proof for a Remote Config target.
func (rc *rcClient) GetConfigTUFProof(targetPath string) (state.ConfigTUFProof, bool) {
	if rc.client == nil {
		return state.ConfigTUFProof{}, false
	}
	return rc.client.GetConfigTUFProof(targetPath)
}

func (rc *rcClient) agentConfigUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	mergedConfig, err := state.MergeRCAgentConfig(rc.client.UpdateApplyStatus, updates)
	if err != nil {
		return
	}

	var errList []error

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
				errList = append(errList, err)
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
			errList = append(errList, err)
		}
	}

	errs := errors.Join(errList...)

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
func (rc *rcClient) agentTaskUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
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
							err = fmt.Errorf("%s: %w", err.Error(), oneErr)
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
