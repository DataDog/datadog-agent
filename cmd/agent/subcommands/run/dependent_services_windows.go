// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package run

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

type serviceInitFunc func() (err error)

// Servicedef defines a service
type Servicedef struct {
	name           string
	configKeys     map[string]model.Reader
	shouldShutdown bool

	serviceName string
	serviceInit serviceInitFunc
}

func subservices(coreConf model.Reader, sysprobeConf model.Reader) []Servicedef {
	return []Servicedef{
		{
			name: "apm",
			configKeys: map[string]model.Reader{
				"apm_config.enabled": coreConf,
			},
			serviceName:    "datadog-trace-agent",
			serviceInit:    apmInit,
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
			serviceName:    "datadog-process-agent",
			serviceInit:    processInit,
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
			serviceInit:    sysprobeInit,
			shouldShutdown: false,
		},
		{
			name: "cws",
			configKeys: map[string]model.Reader{
				"runtime_security_config.enabled": sysprobeConf,
			},
			serviceName:    "datadog-security-agent",
			serviceInit:    securityInit,
			shouldShutdown: false,
		},
		{
			name: "datadog-installer",
			configKeys: map[string]model.Reader{
				"remote_updates": coreConf,
			},
			serviceName:    "Datadog Installer",
			serviceInit:    installerInit,
			shouldShutdown: true,
		},
		{
			name: "private-action-runner",
			configKeys: map[string]model.Reader{
				"private_action_runner.enabled": coreConf,
			},
			serviceName:    "datadog-agent-action",
			serviceInit:    parInit,
			shouldShutdown: true,
		},
		{
			name: "otel",
			configKeys: map[string]model.Reader{
				"otelcollector.enabled": coreConf,
			},
			serviceName:    "datadog-otel-agent",
			serviceInit:    otelInit,
			shouldShutdown: true, // NOTE: not really ncessary with SCM dependency in place
		},
	}
}

func apmInit() error {
	return nil
}

func processInit() error {
	return nil
}

func sysprobeInit() error {
	return nil
}

func securityInit() error {
	return nil
}

func installerInit() error {
	return nil
}

func otelInit() error {
	return nil
}

func parInit() error {
	return nil
}

// Start starts the service
func (s *Servicedef) Start() error {
	// Initialize the service if it has an init function
	if s.serviceInit != nil {
		err := s.serviceInit()
		if err != nil {
			log.Warnf("Failed to initialize %s service: %s", s.name, err.Error())
			return err
		}
	}
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

// IsEnabled checks to see if a given service should be started
func (s *Servicedef) IsEnabled() bool {
	for configKey, cfg := range s.configKeys {
		if cfg.GetBool(configKey) {
			return true
		}
	}
	return false
}

// ShouldStop checks to see if a service should be stopped
func (s *Servicedef) ShouldStop() bool {
	// Note: we do not check if the service is enabled as service like DDOT have a could be brought up individually and should still be shutdown

	if !s.shouldShutdown {
		log.Infof("Service %s is not configured to stop, not stopping", s.name)
		return false
	}
	return true
}

func startDependentServices(coreConf model.Reader, sysprobeConf model.Reader) {
	for _, svc := range subservices(coreConf, sysprobeConf) {
		if svc.IsEnabled() {
			log.Debugf("Attempting to start service: %s", svc.name)
			err := svc.Start()
			if err != nil {
				log.Warnf("Failed to start services %s: %s", svc.name, err.Error())
			} else {
				log.Debugf("Started service %s", svc.name)
			}
		} else {
			log.Infof("Service %s is disabled, not starting", svc.name)
		}
	}
}

func stopDependentServices(coreConf model.Reader, sysprobeConf model.Reader) {
	for _, svc := range subservices(coreConf, sysprobeConf) {
		if svc.ShouldStop() {
			log.Debugf("Attempting to stop service: %s", svc.name)
			err := svc.Stop()
			if err != nil {
				log.Warnf("Failed to stop services %s: %s", svc.name, err.Error())
			} else {
				log.Debugf("Stopped service %s", svc.name)
			}
		} else {
			log.Infof("Service %s is not configured to stop, not stopping", svc.name)
		}
	}
}
