// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build windows

package app

import (
	"fmt"

	"github.com/StackVista/stackstate-agent/pkg/util/log"
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
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	scm, err := m.OpenService(s.serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer scm.Close()
	err = scm.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}

	return nil
}

// Stop stops the service
func (s *Servicedef) Stop() error {
	return nil
}
