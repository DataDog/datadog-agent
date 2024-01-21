// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// HostInfo contains information about a Windows host, such as the hostname and version
type HostInfo struct {
	Hostname string
	Domain   string
	OSInfo   *OSInfo
}

// OSInfo contains a selection of values from: Get-WmiObject Win32_OperatingSystem
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-operatingsystem
type OSInfo struct {
	WindowsDirectory string `json:"WindowsDirectory"`
	Version          string `json:"Version"`
	SystemDrive      string `json:"SystemDrive"`
	SystemDirectory  string `json:"SystemDirectory"`
	ProductType      int    `json:"ProductType"`
	OSType           int    `json:"OSType"`
	OSProductSuite   int    `json:"OSProductSuite"`
	OSLanguage       int    `json:"OSLanguage"`
	Locale           string `json:"Locale"`
	BuildNumber      string `json:"BuildNumber"`
	Caption          string `json:"Caption"`
}

// GetHostInfo returns HostInfo for the given VM
func GetHostInfo(host *components.RemoteHost) (*HostInfo, error) {
	osinfo, err := GetOSInfo(host)
	if err != nil {
		return nil, err
	}
	hostname, err := GetHostname(host)
	if err != nil {
		return nil, err
	}
	domain, err := GetJoinedDomain(host)
	if err != nil {
		return nil, err
	}

	var h HostInfo
	h.Hostname = hostname
	h.Domain = domain
	h.OSInfo = osinfo

	return &h, nil
}

// IsDomainController returns true if the host is a domain controller
func (h *HostInfo) IsDomainController() bool {
	return h.OSInfo.ProductType == 2
}

// GetHostname returns the hostname of the VM
func GetHostname(host *components.RemoteHost) (string, error) {
	hostname, err := host.Execute("[Environment]::MachineName")
	if err != nil {
		return "", fmt.Errorf("GetHostname failed: %v", err)
	}
	return hostname, nil
}

// GetJoinedDomain returns the domain that the host is joined to
func GetJoinedDomain(host *components.RemoteHost) (string, error) {
	domain, err := host.Execute("(Get-WMIObject Win32_ComputerSystem).Domain")
	if err != nil {
		return "", fmt.Errorf("GetJoinedDomain failed: %v", err)
	}
	return domain, nil
}

// GetOSInfo returns OSInfo for the given VM
func GetOSInfo(host *components.RemoteHost) (*OSInfo, error) {
	cmd := "Get-WmiObject Win32_OperatingSystem | ConvertTo-Json"
	output, err := host.Execute(cmd)
	if err != nil {
		fmt.Println(output)
		return nil, fmt.Errorf("GetOSInfo failed: %v", err)
	}

	var result OSInfo
	err = json.Unmarshal([]byte(output), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// NameToNetBIOSName converts a given host or DNS name into a NetBIOS formatted name
//
// Warning: This is not necessarily the actual NetBIOS name of the host, as it can
// be configured separately from the DNS name.
//
// https://learn.microsoft.com/en-us/troubleshoot/windows-server/identity/naming-conventions-for-computer-domain-site-ou
func NameToNetBIOSName(name string) string {
	parts := strings.Split(name, ".")
	upper := strings.ToUpper(parts[0])
	maxlen := int(math.Min(float64(len(upper)), 15))
	return upper[:maxlen]
}
