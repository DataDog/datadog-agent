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
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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
			procmgrDefinitionFile: processProcmgrDefinitionFile,
			serviceName:           "datadog-process-agent",
			shouldShutdown:        false,
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
func (s *Servicedef) IsEnabled(procmgrStartedSuccessfully bool, coreConf model.Reader) bool {
	if s.procmgrDefinitionFile != "" &&
		coreConf.GetBool("process_manager.enabled") &&
		procmgrProcessDefinitionExists(s.procmgrDefinitionFile) {
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
// services (process, PAR, DDOT) need procmgrStarted for suppression decisions;
// apm, sysprobe, and other dependents start independently of procmgr health.
func (s *Servicedef) needsProcmgrStartupGate(coreConf model.Reader) bool {
	return s.procmgrDefinitionFile != "" &&
		coreConf.GetBool("process_manager.enabled") &&
		procmgrProcessDefinitionExists(s.procmgrDefinitionFile)
}

// ShouldStop reports whether the dependent service should be stopped on agent shutdown.
func (s *Servicedef) ShouldStop() bool {
	// Services like DDOT can be started individually and should still be shut down.
	return s.shouldShutdown
}

func startDependentServices(coreConf model.Reader, sysprobeConf model.Reader) {
	svcs := subservices(coreConf, sysprobeConf)

	procmgrWait := make(chan bool, 1)
	go func() {
		procmgr, ok := findService(svcs, "procmgr")
		procmgrWait <- startProcmgrIfEnabled(procmgr, ok)
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

	startServices := func(services []Servicedef, procmgrStarted bool) {
		for _, svc := range services {
			if !svc.IsEnabled(procmgrStarted, coreConf) {
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

	startServices(independent, false)
	startServices(gated, <-procmgrWait)
}

func findService(svcs []Servicedef, name string) (Servicedef, bool) {
	for _, svc := range svcs {
		if svc.name == name {
			return svc, true
		}
	}
	return Servicedef{}, false
}

func startProcmgrIfEnabled(procmgr Servicedef, ok bool) bool {
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
	if waitForProcmgrStartupOutcome(procmgr.serviceName) {
		log.Debugf("Started service %s", procmgr.name)
		return true
	}
	log.Warnf("Failed to start services %s: service did not reach running state", procmgr.name)
	return false
}

// waitForProcmgrStartupOutcome waits for dd-procmgr-service to reach Running or fail.
// Legacy services are suppressed when procmgr is Running or still StartPending after
// startup waits, avoiding duplicate workloads; fallback is allowed only on Stopped.
func waitForProcmgrStartupOutcome(serviceName string) bool {
	if running, done := waitForProcmgrInitialState(serviceName); done {
		return running
	}

	state, err := winutil.GetServiceState(serviceName)
	if err != nil {
		log.Warnf("Failed to query service %s after startup wait: %v", serviceName, err)
		return true
	}
	if state == svc.Running {
		return true
	}
	if state == svc.Stopped {
		return false
	}
	if state != svc.StartPending {
		return false
	}

	log.Warnf("Service %s is still in StartPending after initial wait; waiting for SCM transition", serviceName)
	ctx, cancel := context.WithTimeout(context.Background(), winutil.DefaultServiceCommandTimeout*time.Second)
	defer cancel()
	finalState, err := winutil.WaitForPendingStateChange(ctx, serviceName, svc.StartPending)
	if err != nil {
		log.Warnf("Service %s still StartPending after startup waits; suppressing legacy services: %v", serviceName, err)
		return true
	}
	return finalState == svc.Running
}

// waitForProcmgrInitialState polls until dd-procmgr-service reaches Running or Stopped.
// Returns (running, true) on success/failure terminal states, or (_, false) on timeout.
func waitForProcmgrInitialState(serviceName string) (running bool, done bool) {
	ctx, cancel := context.WithTimeout(context.Background(), winutil.DefaultServiceCommandTimeout*time.Second)
	defer cancel()

	checkState := func() (svc.State, bool) {
		state, err := getServiceStateForStartupWait(serviceName)
		if err != nil {
			return 0, false
		}
		switch state {
		case svc.Running:
			return state, true
		case svc.Stopped:
			return state, true
		default:
			return state, false
		}
	}

	if state, terminal := checkState(); terminal {
		return state == svc.Running, true
	}

	ticker := time.NewTicker(300 * time.Millisecond)
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
