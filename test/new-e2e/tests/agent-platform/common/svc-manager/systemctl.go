// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package svcmanager contains svcmanager implementations
package svcmanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// SystemCtlSvcManager struct for the Systemctl service manager
type SystemCtlSvcManager struct {
	vmClient client.VM
}

// NewSystemctlSvcManager return systemctl service manager
func NewSystemctlSvcManager(vmClient client.VM) *SystemCtlSvcManager {
	return &SystemCtlSvcManager{vmClient}
}

// Status returns status from systemctl
func (s *SystemCtlSvcManager) Status(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("systemctl status --no-pager %s.service", service))
}

// Stop executes stop command from stystemctl
func (s *SystemCtlSvcManager) Stop(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo systemctl stop %s.service", service))
}

// Start executes start command from systemctl
func (s *SystemCtlSvcManager) Start(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo systemctl start %s.service", service))
}

// Restart executes restart command from systemctl
func (s *SystemCtlSvcManager) Restart(service string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo systemctl restart %s.service", service))
}
