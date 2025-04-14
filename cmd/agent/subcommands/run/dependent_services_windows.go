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
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceInitFunc func() (err error)

// Servicedef defines a service
type Servicedef struct {
	name           string
	configKeys     map[string]model.Config
	shouldShutdown bool

	serviceName string
	serviceInit serviceInitFunc
}

var subservices = []Servicedef{
	{
		name: "apm",
		configKeys: map[string]model.Config{
			"apm_config.enabled": pkgconfigsetup.Datadog(),
		},
		serviceName:    "datadog-trace-agent",
		serviceInit:    apmInit,
		shouldShutdown: false,
	},
	{
		name: "process",
		configKeys: map[string]model.Config{
			"process_config.enabled":                      pkgconfigsetup.Datadog(),
			"process_config.process_collection.enabled":   pkgconfigsetup.Datadog(),
			"process_config.container_collection.enabled": pkgconfigsetup.Datadog(),
			"process_config.process_discovery.enabled":    pkgconfigsetup.Datadog(),
			"network_config.enabled":                      pkgconfigsetup.SystemProbe(),
			"system_probe_config.enabled":                 pkgconfigsetup.SystemProbe(),
		},
		serviceName:    "datadog-process-agent",
		serviceInit:    processInit,
		shouldShutdown: false,
	},
	{
		name: "sysprobe",
		configKeys: map[string]model.Config{
			"network_config.enabled":          pkgconfigsetup.SystemProbe(),
			"system_probe_config.enabled":     pkgconfigsetup.SystemProbe(),
			"windows_crash_detection.enabled": pkgconfigsetup.SystemProbe(),
			"runtime_security_config.enabled": pkgconfigsetup.SystemProbe(),
		},
		serviceName:    "datadog-system-probe",
		serviceInit:    sysprobeInit,
		shouldShutdown: false,
	},
	{
		name: "cws",
		configKeys: map[string]model.Config{
			"runtime_security_config.enabled": pkgconfigsetup.SystemProbe(),
		},
		serviceName:    "datadog-security-agent",
		serviceInit:    securityInit,
		shouldShutdown: false,
	},
	{
		name: "datadog-installer",
		configKeys: map[string]model.Config{
			"remote_updates": pkgconfigsetup.Datadog(),
		},
		serviceName:    "Datadog Installer",
		serviceInit:    installerInit,
		shouldShutdown: true,
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

func securityInit() error {
	return nil
}

func installerInit() error {
	return nil
}

// getServiceHandle returns a service handle and manager for the given service name
func (s *Servicedef) getServiceHandle() (*mgr.Mgr, *mgr.Service, error) {
	/*
	 * default go implementations of mgr.Connect and mgr.OpenService use way too
	 * open permissions by default.  Use those structures so the other methods
	 * work properly, but initialize them here using restrictive enough permissions
	 * that we can actually open/start the service when running as non-root.
	 */
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		log.Warnf("Failed to connect to scm %v", err)
		return nil, nil, err
	}
	m := &mgr.Mgr{Handle: h}

	snptr, err := syscall.UTF16PtrFromString(s.serviceName)
	if err != nil {
		log.Warnf("Failed to get service name %v", err)
		m.Disconnect()
		return nil, nil, fmt.Errorf("could not create service name pointer: %s", err)
	}

	hSvc, err := windows.OpenService(m.Handle, snptr,
		windows.SERVICE_START|windows.SERVICE_STOP)
	if err != nil {
		log.Warnf("Failed to open service %v", err)
		m.Disconnect()
		return nil, nil, fmt.Errorf("could not access service: %v", err)
	}
	scm := &mgr.Service{Name: s.serviceName, Handle: hSvc}

	return m, scm, nil
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

	m, scm, err := s.getServiceHandle()
	if err != nil {
		return err
	}
	defer m.Disconnect()
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
	m, scm, err := s.getServiceHandle()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer scm.Close()

	status, err := scm.Control(svc.Stop)
	if err != nil {
		log.Warnf("Failed to stop service %v", err)
		return fmt.Errorf("could not stop service: %v", err)
	}
	log.Debugf("Service %s stopped with status %v", s.serviceName, status)

	return nil
}
