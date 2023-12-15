// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package svcmanager

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// ServiceSvcManager struct for Service service manager
type ServiceSvcManager struct {
	vmClient client.VM
}

// NewServiceSvcManager return service service manager
func NewServiceSvcManager(vmClient client.VM) *ServiceSvcManager {
	return &ServiceSvcManager{vmClient}
}

// Status returns status from service
func (s *ServiceSvcManager) Status(service string) (string, error) {
	status, err := s.vmClient.ExecuteWithError(fmt.Sprintf("service %s status", service))

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
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo service %s stop", service))
}

// Start executes start command from service
func (s *ServiceSvcManager) Start(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo service %s start", service))
}

// Restart executes restart command from service
func (s *ServiceSvcManager) Restart(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo service %s restart", service))
}
