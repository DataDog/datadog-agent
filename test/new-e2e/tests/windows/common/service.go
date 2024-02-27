// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// GetServiceStatus returns the status of the service
func GetServiceStatus(host *components.RemoteHost, service string) (string, error) {
	cmd := fmt.Sprintf("(Get-Service -Name '%s').Status", service)
	out, err := host.Execute(cmd)
	return strings.TrimSpace(out), err
}

// StopService stops the service
func StopService(host *components.RemoteHost, service string) error {
	cmd := fmt.Sprintf("Stop-Service -Force -Name '%s'", service)
	_, err := host.Execute(cmd)
	return err
}

// StartService starts the service
func StartService(host *components.RemoteHost, service string) error {
	cmd := fmt.Sprintf("Start-Service -Name '%s'", service)
	_, err := host.Execute(cmd)
	return err
}

// RestartService restarts the service
func RestartService(host *components.RemoteHost, service string) error {
	cmd := fmt.Sprintf("Restart-Service -Force -Name '%s'", service)
	_, err := host.Execute(cmd)
	return err
}
