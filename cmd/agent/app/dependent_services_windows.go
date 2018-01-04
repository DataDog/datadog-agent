// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package app

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/go-ini/ini"
	"golang.org/x/sys/windows/svc/mgr"
	"path/filepath"
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
		configKey:   "apm_enabled",
		serviceName: "datadog-trace-agent",
		serviceInit: apmInit,
	}}

func apmInit() error {
	traceAgentConfPath := filepath.Join(common.DefaultConfPath, "trace-agent.conf")
	iniFile, err := ini.Load(traceAgentConfPath)
	if err != nil {
		log.Warnf("Failed to load APM config file, creating")
		iniFile = ini.Empty()
	}
	// this will create the section if it's not there
	main := iniFile.Section("Main")
	k, err := main.GetKey("api_key")
	if err != nil {
		log.Warnf("API key not found in trace-agent.conf, adding")
		main.NewKey("api_key", config.Datadog.GetString("api_key"))
		err = iniFile.SaveTo(traceAgentConfPath)
	} else if k.Value() == "" {
		log.Warnf("API key not found in trace-agent.conf, adding")
		main.NewKey("api_key", config.Datadog.GetString("api_key"))
		err = iniFile.SaveTo(traceAgentConfPath)
	}
	return err

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
