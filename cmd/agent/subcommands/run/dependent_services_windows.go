// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package run

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceInitFunc func() (err error)

// Servicedef defines a service
type Servicedef struct {
	name       string
	configKeys map[string]config.Config

	serviceName string
	serviceInit serviceInitFunc
}

var subservices = []Servicedef{
	{
		name: "apm",
		configKeys: map[string]config.Config{
			"apm_config.enabled": config.Datadog,
		},
		serviceName: "datadog-trace-agent",
		serviceInit: apmInit,
	},
	{
		name: "process",
		configKeys: map[string]config.Config{
			"process_config.enabled":                      config.Datadog,
			"process_config.process_collection.enabled":   config.Datadog,
			"process_config.container_collection.enabled": config.Datadog,
			"process_config.process_discovery.enabled":    config.Datadog,
			"network_config.enabled":                      config.SystemProbe,
			"system_probe_config.enabled":                 config.SystemProbe,
		},
		serviceName: "datadog-process-agent",
		serviceInit: processInit,
	},
	{
		name: "sysprobe",
		configKeys: map[string]config.Config{
			"network_config.enabled":          config.SystemProbe,
			"system_probe_config.enabled":     config.SystemProbe,
			"windows_crash_detection.enabled": config.SystemProbe,
		},
		serviceName: "datadog-system-probe",
		serviceInit: sysprobeInit,
	},
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

// Start starts the service
func (s *Servicedef) Start() error {
	if s.serviceInit != nil {
		err := s.serviceInit()
		if err != nil {
			log.Warnf("Failed to initialize %s service: %s", s.name, err.Error())
			return err
		}
	}

	/*
	 * default go implementations of mgr.Connect and mgr.OpenService use way too
	 * open permissions by default.  Use those structures so the other methods
	 * work properly, but initialize them here using restrictive enough permissions
	 * that we can actually open/start the service when running as non-root.
	 */
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		log.Warnf("Failed to connect to scm %v", err)
		return err
	}
	m := &mgr.Mgr{Handle: h}
	defer m.Disconnect()

	snptr, err := syscall.UTF16PtrFromString(s.serviceName)
	if err != nil {
		log.Warnf("Failed to get service name %v", err)
		return fmt.Errorf("could not create service name pointer: %s", err)
	}

	hSvc, err := windows.OpenService(m.Handle, snptr,
		windows.SERVICE_START|windows.SERVICE_STOP)
	if err != nil {
		log.Warnf("Failed to open service %v", err)
		return fmt.Errorf("could not access service: %v", err)
	}
	scm := &mgr.Service{Name: s.serviceName, Handle: hSvc}
	defer scm.Close()
	err = scm.Start("is", "manual-started")
	if err != nil {
		log.Warnf("Failed to start service %v", err)
		return fmt.Errorf("could not start service: %v", err)
	}

	return nil
}

// Stop stops the service
func (s *Servicedef) Stop() error {
	return nil
}
