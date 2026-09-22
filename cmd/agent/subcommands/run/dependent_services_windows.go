// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package run

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	processProcmgrDefinitionFile = "datadog-agent-process.yaml"
	parProcmgrDefinitionFile     = "datadog-agent-action.yaml"
	ddotProcmgrDefinitionFile    = "datadog-agent-ddot.yaml"
)

// Servicedef defines a service
type Servicedef struct {
	name       string
	configKeys map[string]model.Reader
	// procmgrDefinitionFile, when set, is the processes.d YAML basename used to decide
	// whether the legacy SCM service is suppressed in favor of dd-procmgr.
	procmgrDefinitionFile string
	shouldShutdown        bool

	serviceName string
}

func subservices(coreConf model.Reader, sysprobeConf model.Reader) []Servicedef {
	return []Servicedef{
		{
			name: "apm",
			configKeys: map[string]model.Reader{
				"apm_config.enabled": coreConf,
			},
			serviceName:    "datadog-trace-agent",
			shouldShutdown: false,
		},
		{
			name: "process",
			configKeys: map[string]model.Reader{
				"process_config.enabled":                      coreConf,
				"process_config.process_collection.enabled":   coreConf,
				"process_config.container_collection.enabled": coreConf,
				"process_config.process_discovery.enabled":    coreConf,
				"network_config.enabled":                      sysprobeConf,
				"system_probe_config.enabled":                 sysprobeConf,
			},
			// NOTE: no procmgrDefinitionFile here on purpose. The fleet process-agent
			// definition still ships auto_start: false, so suppressing this service would
			// leave no process-agent running at all. Both must flip in the same change.
			serviceName:    "datadog-process-agent",
			shouldShutdown: false,
		},
		{
			name: "sysprobe",
			configKeys: map[string]model.Reader{
				"network_config.enabled": sysprobeConf,
				// NOTE: may be set at runtime if any modules are enabled (e.g. traceroute.enabled)
				"system_probe_config.enabled":     sysprobeConf,
				"windows_crash_detection.enabled": sysprobeConf,
				"runtime_security_config.enabled": sysprobeConf,
				"software_inventory.enabled":      coreConf,
			},
			serviceName:    "datadog-system-probe",
			shouldShutdown: false,
		},
		{
			name: "cws",
			configKeys: map[string]model.Reader{
				"runtime_security_config.enabled": sysprobeConf,
			},
			serviceName:    "datadog-security-agent",
			shouldShutdown: false,
		},
		{
			name: "datadog-installer",
			configKeys: map[string]model.Reader{
				"remote_updates": coreConf,
			},
			serviceName:    "Datadog Installer",
			shouldShutdown: true,
		},
		{
			name: "private-action-runner",
			configKeys: map[string]model.Reader{
				"private_action_runner.enabled": coreConf,
			},
			procmgrDefinitionFile: parProcmgrDefinitionFile,
			serviceName:           "datadog-agent-action",
			shouldShutdown:        true,
		},
		{
			name: "otel",
			configKeys: map[string]model.Reader{
				"otelcollector.enabled": coreConf,
			},
			procmgrDefinitionFile: ddotProcmgrDefinitionFile,
			serviceName:           "datadog-otel-agent",
			shouldShutdown:        true, // NOTE: not really necessary with SCM dependency in place
		},
		{
			name: "procmgr",
			configKeys: map[string]model.Reader{
				"process_manager.enabled": coreConf,
			},
			serviceName:    "dd-procmgr-service",
			shouldShutdown: true,
		},
	}
}

// Start starts the service
func (s *Servicedef) Start() error {
	// we use the winutil StartService because it opens the service
	// with the correct permissions for us and not the default of SC_MANAGER_ALL
	// that the svc package uses
	return winutil.StartService(s.serviceName)
}

// Stop stops the service
func (s *Servicedef) Stop() error {
	// note that this will stop the service and any services that depend on it
	// it will also wait for the service to stop and return an error if it doesn't stop
	// the default timeout is 30 seconds
	return winutil.StopService(s.serviceName)
}

// start various subservices (apm, logs, process, system-probe) based on the config file settings

// IsEnabled checks whether a dependent service should be started. When install policy
// would suppress a legacy SCM service in favor of procmgr, suppression applies only if
// dd-procmgr-service started successfully; otherwise the legacy service is used.
//
// This is the only place processes.d is read during a startup pass. procmgrGated says the
// caller already waited for the dd-procmgr-service outcome, so procmgrStartedSuccessfully
// is a real answer rather than an assumed one.
func (s *Servicedef) IsEnabled(procmgrGated bool, procmgrStartedSuccessfully bool) bool {
	if procmgrGated && procmgrProcessDefinitionExists(s.procmgrDefinitionFile) {
		if procmgrStartedSuccessfully {
			log.Infof("Service %s suppressed (install policy)", s.name)
			return false
		}
		log.Warnf("Service %s not suppressed: dd-procmgr-service unavailable, using legacy Windows service", s.name)
	}
	return s.isEnabledByConfig()
}

func (s *Servicedef) isEnabledByConfig() bool {
	for configKey, cfg := range s.configKeys {
		if cfg.GetBool(configKey) {
			return true
		}
	}
	return false
}

// needsProcmgrStartupGate reports whether starting this service must wait for
// dd-procmgr-service to reach a final startup outcome. Only procmgr-managed legacy
// services need procmgrStarted for suppression decisions; apm, sysprobe, and other
// dependents start independently of procmgr health.
//
// It deliberately does not look at processes.d. An installer run can create or remove a
// definition while the agent is starting, so reading it here and again when the decision
// is made would let one startup pass act on two different answers.
func (s *Servicedef) needsProcmgrStartupGate(coreConf model.Reader) bool {
	return s.procmgrDefinitionFile != "" && coreConf.GetBool("process_manager.enabled")
}

// ShouldStop reports whether the dependent service should be stopped on agent shutdown.
func (s *Servicedef) ShouldStop() bool {
	// Services like DDOT can be started individually and should still be shut down.
	return s.shouldShutdown
}

// dependentServicesStartup tracks the in-flight startup pass so shutdown can join it
// rather than race it. Add happens in startDependentServicesAsync, before the goroutine
// exists, so it can never be concurrent with the Wait in stopDependentServices.
var dependentServicesStartup sync.WaitGroup

// startDependentServicesAsync runs the startup pass in the background. It has to be in the
// background because the agent may still be in service start pending, where the SCM calls
// below would block or fail.
func startDependentServicesAsync(coreConf model.Reader, sysprobeConf model.Reader) {
	dependentServicesStartup.Add(1)
	go func() {
		defer dependentServicesStartup.Done()
		startDependentServices(coreConf, sysprobeConf)
	}()
}

func startDependentServices(coreConf model.Reader, sysprobeConf model.Reader) {
	// stopAgent cancels the main context immediately before calling stopDependentServices,
	// so it doubles as this goroutine's shutdown signal. That matters because shutdown
	// stops dd-procmgr-service, and procmgr going Stopped is exactly what resolves the
	// startup wait toward the legacy fallback: without this, a shutdown during the wait
	// would start the legacy services again just after the stop pass had stopped them.
	ctx, _ := pkgcommon.GetMainCtxCancel()

	svcs := subservices(coreConf, sysprobeConf)

	procmgrWait := make(chan bool, 1)
	go func() {
		procmgr, ok := findService(svcs, "procmgr")
		procmgrWait <- startProcmgrIfEnabled(ctx, procmgr, ok)
	}()

	var independent, gated []Servicedef
	for _, svc := range svcs {
		if svc.name == "procmgr" {
			continue
		}
		if svc.needsProcmgrStartupGate(coreConf) {
			gated = append(gated, svc)
		} else {
			independent = append(independent, svc)
		}
	}

	startServices := func(services []Servicedef, procmgrGated bool, procmgrStarted bool) {
		for _, svc := range services {
			if ctx.Err() != nil {
				log.Infof("Agent is shutting down, not starting remaining dependent services")
				return
			}
			if !svc.IsEnabled(procmgrGated, procmgrStarted) {
				log.Infof("Service %s is disabled, not starting", svc.name)
				continue
			}
			log.Debugf("Attempting to start service: %s", svc.name)
			if err := svc.Start(); err != nil {
				log.Warnf("Failed to start services %s: %s", svc.name, err.Error())
			} else {
				log.Debugf("Started service %s", svc.name)
			}
		}
	}

	startServices(independent, false, false)
	startServices(gated, true, <-procmgrWait)
}

func findService(svcs []Servicedef, name string) (Servicedef, bool) {
	for _, svc := range svcs {
		if svc.name == name {
			return svc, true
		}
	}
	return Servicedef{}, false
}

func startProcmgrIfEnabled(ctx context.Context, procmgr Servicedef, ok bool) bool {
	if !ok {
		return false
	}
	if !procmgr.isEnabledByConfig() {
		log.Infof("Service %s is disabled, not starting", procmgr.name)
		return false
	}
	log.Debugf("Attempting to start service: %s", procmgr.name)
	if err := procmgr.Start(); err != nil {
		log.Warnf("Failed to start services %s: %s", procmgr.name, err.Error())
		return false
	}
	if waitForProcmgrStartupOutcome(ctx, procmgr.serviceName) {
		log.Debugf("Started service %s", procmgr.name)
		return true
	}
	log.Warnf("Failed to start services %s: service did not reach running state", procmgr.name)
	return false
}

// waitForProcmgrStartupOutcome reports whether legacy services should stay suppressed.
// Ambiguity resolves toward suppression, because a slow procmgr that eventually runs
// would otherwise produce two copies of the same workload: a state query that fails, or
// a service still StartPending after both waits, suppresses. Only a state that says
// procmgr is definitely not coming up, Stopped above all, allows the legacy fallback.
func waitForProcmgrStartupOutcome(ctx context.Context, serviceName string) bool {
	if running, done := waitForProcmgrInitialState(ctx, serviceName); done {
		return running
	}
	if ctx.Err() != nil {
		// The agent is shutting down. The caller starts nothing in that case, but keep
		// the safer answer so this never reads as "procmgr failed, start the legacy one".
		log.Infof("Agent is shutting down, abandoning the wait for service %s", serviceName)
		return true
	}

	state, err := getServiceStateForStartupWait(serviceName)
	if err != nil {
		// The state is unknown, so suppress rather than risk a duplicate workload.
		log.Warnf("Failed to query service %s after startup wait: %v", serviceName, err)
		return true
	}
	switch state {
	case svc.Running:
		return true
	case svc.StartPending:
		// Handled below: give the SCM more time rather than deciding now.
	default:
		// Stopped, Paused, StopPending and the rest: procmgr is not supervising
		// anything, so the legacy service is the only way to run the workload.
		return false
	}

	log.Warnf("Service %s is still in StartPending after initial wait; waiting for SCM transition", serviceName)
	waitCtx, cancel := context.WithTimeout(ctx, winutil.DefaultServiceCommandTimeout*time.Second)
	defer cancel()
	finalState, err := winutil.WaitForPendingStateChange(waitCtx, serviceName, svc.StartPending)
	if err != nil {
		log.Warnf("Service %s still StartPending after startup waits; suppressing legacy services: %v", serviceName, err)
		return true
	}
	return finalState == svc.Running
}

// waitForProcmgrInitialState polls until dd-procmgr-service reaches Running or Stopped.
// Returns (running, true) on success/failure terminal states, or (_, false) on timeout
// or agent shutdown.
func waitForProcmgrInitialState(ctx context.Context, serviceName string) (running bool, done bool) {
	ctx, cancel := context.WithTimeout(ctx, winutil.DefaultServiceCommandTimeout*time.Second)
	defer cancel()

	checkState := func() (svc.State, bool) {
		state, err := getServiceStateForStartupWait(serviceName)
		if err != nil {
			return 0, false
		}
		switch state {
		case svc.Running, svc.Stopped:
			return state, true
		default:
			return state, false
		}
	}

	if state, terminal := checkState(); terminal {
		return state == svc.Running, true
	}

	ticker := time.NewTicker(procmgrStartupPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, false
		case <-ticker.C:
			if state, terminal := checkState(); terminal {
				return state == svc.Running, true
			}
		}
	}
}

func stopDependentServices(coreConf model.Reader, sysprobeConf model.Reader) {
	// Cancelling the main context stops the startup pass from starting anything further,
	// but a service it already decided to start may be mid-Start right now. Joining the
	// pass keeps that Start from landing after the loop below, which would leave the
	// service running once the agent exits. The wait is bounded: after cancellation the
	// pass only has one winutil.StartService left to finish.
	dependentServicesStartup.Wait()

	for _, svc := range subservices(coreConf, sysprobeConf) {
		if !svc.ShouldStop() {
			log.Infof("Service %s is not configured to stop, not stopping", svc.name)
			continue
		}
		log.Debugf("Attempting to stop service: %s", svc.name)
		err := svc.Stop()
		if err != nil {
			log.Warnf("Failed to stop services %s: %s", svc.name, err.Error())
		} else {
			log.Debugf("Stopped service %s", svc.name)
		}
	}
}

// getServiceStateForStartupWait is overridable in tests.
var getServiceStateForStartupWait = winutil.GetServiceState

// procmgrStartupPollInterval is how often the startup wait polls the SCM for
// dd-procmgr-service. Overridable in tests to keep them off the wall clock.
var procmgrStartupPollInterval = 300 * time.Millisecond

func procmgrProcessDefinitionExists(fileName string) bool {
	installPath, err := procmgrInstallRootForDefinitionCheck()
	if err != nil || installPath == "" {
		return false
	}
	p := filepath.Join(installPath, "processes.d", fileName)
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// procmgrInstallRootForDefinitionCheck resolves the Agent install root used when
// checking for processes.d definitions. Tests may override it to use a temp dir.
var procmgrInstallRootForDefinitionCheck = func() (string, error) {
	return winutil.GetProgramFilesDirForProduct("Datadog Agent")
}
