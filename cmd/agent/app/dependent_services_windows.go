// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build windows

package app

import (
	"fmt"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type serviceInitFunc func() (err error)

// Servicedef defines a service
type Servicedef struct {
	name      string
	configKey string

	serviceName string
	serviceInit serviceInitFunc
}

var subservices = []Servicedef{
	{
		name:        "apm",
		configKey:   "apm_config.enabled",
		serviceName: "datadog-trace-agent",
		serviceInit: apmInit,
	},
	{
		name:        "process",
		configKey:   "process_config.enabled",
		serviceName: "datadog-process-agent",
		serviceInit: processInit,
	}}

func apmInit() error {

	return nil

}

func processInit() error {
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
	 * default go impolementations of mgr.Connect and mgr.OpenService use way too
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

	hSvc, err := windows.OpenService(m.Handle, syscall.StringToUTF16Ptr(s.serviceName),
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
