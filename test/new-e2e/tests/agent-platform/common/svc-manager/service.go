// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package svcmanager

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// ServiceSvcManager struct for Service service manager
type ServiceSvcManager struct {
	host *components.RemoteHost
}

// NewServiceSvcManager return service service manager
func NewServiceSvcManager(host *components.RemoteHost) *ServiceSvcManager {
	return &ServiceSvcManager{host: host}
}

// Status returns status from service
func (s *ServiceSvcManager) Status(service string) (string, error) {
	status, err := s.host.Execute(fmt.Sprintf("service %s status", service))
	if err != nil {
		return status, err
	}

	// systemctl status returns 0 even if the service is not running
	if strings.Contains(status, fmt.Sprintf("%s stop", service)) {
		return status, fmt.Errorf("service %s is not running", service)
	}
	return status, nil
}

// Stop executes stop command from service
func (s *ServiceSvcManager) Stop(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s stop", service))
}

// Start executes start command from service
func (s *ServiceSvcManager) Start(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s start", service))
}

// Restart executes restart command from service
func (s *ServiceSvcManager) Restart(service string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo service %s restart", service))
}
