// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ntp

//revive:disable:var-naming  All names in this file match Windows API identifiers
import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	netapi32        = windows.NewLazySystemDLL("netapi32.dll")
	procDsGetDcName = netapi32.NewProc("DsGetDcNameW")
	// Name is intended to match the Windows API name see MSDN: https://learn.microsoft.com/en-us/windows/win32/api/lmapibuf/nf-lmapibuf-netapibufferfree
	procNetApiBufferFree = netapi32.NewProc("NetApiBufferFree")
	// These variables set up:
	// Lazy loading of Windows DLL functions from netapi32.dll
	// Function pointers for dependency injection (allows mocking in tests)
	// Windows API calls for domain controller discovery
	// Function variables for testing - can be overridden in tests
	registryOpenKey = registry.OpenKey
	dsGetDcNameCall = procDsGetDcName.Call
)

// Constants for DsGetDcName
// Name is intended to match the Windows API name see MSDN: https://learn.microsoft.com/en-us/windows/win32/api/dsgetdc/nf-dsgetdc-dsgetdcnamew
const (
	DS_PDC_REQUIRED    = 0x00000080
	DS_RETURN_DNS_NAME = 0x40000000
	DS_AVOID_SELF      = 0x00004000
)

// DOMAIN_CONTROLLER_INFO structure
// Name is intended to match the Windows API name see MSDN: https://learn.microsoft.com/en-us/windows/win32/api/dsgetdc/ns-dsgetdc-domain_controller_infow
type DOMAIN_CONTROLLER_INFO struct {
	DomainControllerName        *uint16
	DomainControllerAddress     *uint16
	DomainControllerAddressType uint32
	DomainGuid                  syscall.GUID
	DomainName                  *uint16
	DnsForestName               *uint16
	Flags                       uint32
	DcSiteName                  *uint16
	ClientSiteName              *uint16
}

func getLocalDefinedNTPServers() ([]string, error) {
	// Use native Windows registry API to get the Type value
	// https://learn.microsoft.com/en-us/windows-server/networking/windows-time-service/windows-time-service-tools-and-settings?tabs=parameters
	regKeyPath := `SYSTEM\CurrentControlSet\Services\W32Time\Parameters`
	k, err := registryOpenKey(registry.LOCAL_MACHINE, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("Cannot open registry key %s: %s", regKeyPath, err)
	}
	defer k.Close()

	// Read the Type value
	typeValue, _, err := k.GetStringValue("Type")
	if err != nil {
		return nil, fmt.Errorf("Cannot get Type value from registry: %s", err)
	}

	// Based on Type value, decide how to get NTP servers
	switch strings.ToUpper(typeValue) {
	case "NT5DS":
		// AD Domain Hierarchy - prioritize PDC discovery
		log.Debug("NT5DS detected, attempting PDC discovery")

		// First, try proactive PDC discovery
		servers, err := discoverPDC()
		if err == nil && len(servers) > 0 {
			log.Infof("Successfully discovered PDC: %s", strings.Join(servers, ", "))
			return servers, nil
		}
		// If PDC discovery fails, log a warning and fall through to the registry method.
		// This handles cases where a machine is configured for NT5DS but is off-domain.
		log.Warnf("PDC discovery failed (%v), falling back to registry NTP servers.", err)

	case "NOSYNC":
		// Go directly to registry method
		// Fall through to registry method

	case "NTP":
		// Standard NTP - use registry method
		// Fall through to registry method

	default:
		// Unknown type, try registry method
		log.Debugf("Unknown W32Time Type value: %s, using registry method", typeValue)
		// Fall through to registry method
	}

	// Standard registry method for NTP servers
	return getNTPServersFromRegistry(k)
}

func discoverPDC() ([]string, error) {
	// Use DsGetDcName to find the PDC Emulator
	var dcInfo *DOMAIN_CONTROLLER_INFO

	// Call DsGetDcName with DS_PDC_REQUIRED flag to get the PDC
	ret, _, err := dsGetDcNameCall(
		0, // ComputerName (NULL = local computer)
		0, // DomainName (NULL = primary domain)
		0, // DomainGuid (NULL)
		0, // SiteName (NULL)
		uintptr(DS_PDC_REQUIRED|DS_RETURN_DNS_NAME|DS_AVOID_SELF),
		uintptr(unsafe.Pointer(&dcInfo)),
	)

	if ret != 0 {
		return nil, fmt.Errorf("DsGetDcName failed with error code: %d (%v)", ret, err)
	}

	// Ensure we free the buffer when done, error is ignored as it is not critical
	defer procNetApiBufferFree.Call(uintptr(unsafe.Pointer(dcInfo))) //nolint:errcheck

	// Convert the PDC name to a Go string
	if dcInfo.DomainControllerName != nil {
		pdcName := windows.UTF16PtrToString(dcInfo.DomainControllerName)
		// Remove leading backslashes if present
		pdcName = strings.TrimPrefix(pdcName, "\\\\")

		if pdcName != "" {
			return []string{pdcName}, nil
		}
	}

	return nil, errors.New("PDC discovery returned empty controller name")
}

func getNTPServersFromRegistry(k registry.Key) ([]string, error) {
	// Read NtpServer value from already open registry key
	ntpServerValue, _, err := k.GetStringValue("NtpServer")
	if err != nil {
		return nil, fmt.Errorf("Cannot get NtpServer value from registry: %s", err)
	}

	servers, err := getNptServersFromRegKeyValue(ntpServerValue)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse NTP servers from registry value: %s", err)
	}
	return servers, nil
}

func getNptServersFromRegKeyValue(regKeyValue string) ([]string, error) {
	// Possible formats:
	// time.windows.com,0x9
	// pool.ntp.org time.windows.com time.apple.com time.google.com
	fields := strings.Fields(regKeyValue)
	var servers []string
	for _, f := range fields {
		server := strings.Split(f, ",")[0]
		if server != "" {
			servers = append(servers, server)
		}
	}

	if len(servers) == 0 {
		return nil, errors.New("No NTP server found in registry value")
	}

	return servers, nil
}
