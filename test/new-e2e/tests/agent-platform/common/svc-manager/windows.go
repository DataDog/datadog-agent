// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package svcmanager

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// Windows struct for Windows service manager (SCM)
type Windows struct {
	host *components.RemoteHost
}

var _ ServiceManager = &Windows{}

// NewWindows returns Windows service manager
func NewWindows(host *components.RemoteHost) *Windows {
	return &Windows{host}
}

// Status returns status from service
func (s *Windows) Status(service string) (string, error) {
	status, err := windows.GetServiceStatus(s.host, service)
	if err != nil {
		return status, err
	}

	// TODO: The other service managers return an error if the service is not running,
	//       is that really what we want?
	if !strings.EqualFold(status, "Running") {
		return status, fmt.Errorf("service %s is not running", service)
	}

	return status, nil
}

// Stop executes stop command from service
func (s *Windows) Stop(service string) (string, error) {
	return "", windows.StopService(s.host, service)
}

// Start executes start command from service
func (s *Windows) Start(service string) (string, error) {
	return "", windows.StartService(s.host, service)
}

// Restart executes restart command from service
func (s *Windows) Restart(service string) (string, error) {
	return "", windows.RestartService(s.host, service)
}
