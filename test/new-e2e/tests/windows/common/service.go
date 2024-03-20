// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Service API constants
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-query_service_configa
const (
	//revive:disable:var-naming match windows API names

	// dwServiceType
	SERVICE_KERNEL_DRIVER       = 0x00000001
	SERVICE_FILE_SYSTEM_DRIVER  = 0x00000002
	SERVICE_WIN32_OWN_PROCESS   = 0x00000010
	SERVICE_WIN32_SHARE_PROCESS = 0x00000020

	// dwStartType
	SERVICE_SYSTEM_START = 1
	SERVICE_AUTO_START   = 2
	SERVICE_DEMAND_START = 3
	SERVICE_DISABLED     = 4

	//revive:enable:var-naming
)

// ServiceConfig contains information about a Windows service
type ServiceConfig struct {
	ServiceName        string
	DisplayName        string
	ImagePath          string
	StartType          int
	ServiceType        int
	Status             int
	UserName           string
	UserSID            string
	ServicesDependedOn []string `json:"-"`
}

// ServiceConfigMap maps a service name to a ServiceConfig
type ServiceConfigMap map[string]*ServiceConfig

// UnmarshalJSON implements the yaml.Unmarshaler interface
func (s *ServiceConfig) UnmarshalJSON(b []byte) error {
	// unmarshal basic types, use type alias to avoid infinite recursion
	type serviceConfig ServiceConfig
	if err := json.Unmarshal(b, (*serviceConfig)(s)); err != nil {
		return err
	}
	// flatten some types so they are easier to use
	// ServicesDependedOn is returned as an object list, but we just want the service names
	type expandedServiceConfig struct {
		ServicesDependedOn []struct {
			ServiceName string
		}
	}
	var expanded expandedServiceConfig
	if err := json.Unmarshal(b, &expanded); err != nil {
		return err
	}
	s.ServicesDependedOn = make([]string, len(expanded.ServicesDependedOn))
	for i, d := range expanded.ServicesDependedOn {
		s.ServicesDependedOn[i] = d.ServiceName
	}
	return nil
}

// FetchUserSID fetches the SID for the service user
func (s *ServiceConfig) FetchUserSID(host *components.RemoteHost) error {
	if s.UserName == "" {
		return fmt.Errorf("UserName is not set")
	}
	var err error
	sid, err := GetServiceAliasSID(s.UserName)
	if err == nil {
		s.UserSID = sid
		return nil
	}

	s.UserSID, err = GetSIDForUser(host, s.UserName)
	if err != nil {
		return err
	}

	return nil
}

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

// GetServiceConfig returns the configuration of the service
//
// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/get-service?view=powershell-7.4
func GetServiceConfig(host *components.RemoteHost, service string) (*ServiceConfig, error) {
	cmd := fmt.Sprintf("Get-Service -Name '%s' | ConvertTo-Json", service)
	output, err := host.Execute(cmd)
	if err != nil {
		fmt.Println(output)
		return nil, err
	}

	var result ServiceConfig
	err = json.Unmarshal([]byte(output), &result)
	if err != nil {
		return nil, err
	}

	// if it's a usermode service, get the username and SID
	if IsUserModeServiceType(result.ServiceType) {
		// UserName was not added to Get-Service until PowerShell 6.0
		if result.UserName == "" {
			result.UserName, err = GetServiceAccountName(host, service)
			if err != nil {
				return nil, err
			}
		}

		err = result.FetchUserSID(host)
		if err != nil {
			return nil, err
		}
	}

	// Get-Service does not return the command line for the service
	imagePath, err := GetServiceImagePath(host, service)
	if err != nil {
		return nil, err
	}
	result.ImagePath = imagePath

	return &result, nil
}

// GetServiceAliasSID returns the SID for a special SCM account alias
//
// https://learn.microsoft.com/en-us/windows/win32/services/service-user-accounts
func GetServiceAliasSID(alias string) (string, error) {
	switch alias {
	case "LocalSystem":
		return "S-1-5-18", nil
	case "LocalService":
		return "S-1-5-19", nil
	case "NetworkService":
		return "S-1-5-20", nil
	}
	return "", fmt.Errorf("unknown alias %s", alias)
}

// GetServiceConfigMap returns a map of service names to service configuration
func GetServiceConfigMap(host *components.RemoteHost, services []string) (ServiceConfigMap, error) {
	result := make(ServiceConfigMap)
	for _, service := range services {
		config, err := GetServiceConfig(host, service)
		if err != nil {
			return nil, err
		}
		result[service] = config
	}
	return result, nil
}

// GetEmptyServiceConfigMap returns a ServiceConfigMap with only the ServiceName set
func GetEmptyServiceConfigMap(services []string) ServiceConfigMap {
	result := make(ServiceConfigMap)
	for _, service := range services {
		result[service] = &ServiceConfig{
			ServiceName: service,
		}
	}
	return result
}

// GetServiceAccountName returns the account name that the service runs as
func GetServiceAccountName(host *components.RemoteHost, service string) (string, error) {
	cmd := fmt.Sprintf("(Get-WmiObject Win32_Service -Filter \"Name=`'%s`'\").StartName", service)
	out, err := host.Execute(cmd)
	return strings.TrimSpace(out), err
}

// GetServicePID returns the PID of the service
func GetServicePID(host *components.RemoteHost, service string) (int, error) {
	cmd := fmt.Sprintf("(Get-WmiObject Win32_Service -Filter \"Name=`'%s`'\").ProcessId", service)
	out, err := host.Execute(cmd)
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	return strconv.Atoi(out)
}

// GetServiceImagePath returns the image path (command line) of the service
func GetServiceImagePath(host *components.RemoteHost, service string) (string, error) {
	return GetRegistryValue(host, fmt.Sprintf("HKLM:\\SYSTEM\\CurrentControlSet\\Services\\%s", service), "ImagePath")
}

// IsUserModeServiceType returns true if the service is a user mode service
func IsUserModeServiceType(serviceType int) bool {
	return serviceType == SERVICE_WIN32_OWN_PROCESS || serviceType == SERVICE_WIN32_SHARE_PROCESS
}

// IsKernelModeServiceType returns true if the service is a kernel mode service
func IsKernelModeServiceType(serviceType int) bool {
	return serviceType == SERVICE_KERNEL_DRIVER || serviceType == SERVICE_FILE_SYSTEM_DRIVER
}
