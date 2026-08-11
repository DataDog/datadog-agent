// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/yusufpapurcu/wmi"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// oemInfPattern matches the name Windows assigns to an out-of-box driver package
// when it is published to the driver store (e.g. "oem12.inf"). Inbox drivers keep
// their original INF name, so this is what scopes collection to third-party packages.
var oemInfPattern = regexp.MustCompile(`(?i)^oem\d+\.inf$`)

// win32PnPSignedDriver is the narrow projection of the Win32_PnPSignedDriver WMI class
// that the collector needs. Signature columns (Signer, IsSigned) are deliberately not
// selected: they are the expensive part of this query.
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-pnpsigneddriver
type win32PnPSignedDriver struct {
	DeviceClass        string
	Description        string
	DriverProviderName string
	DriverVersion      string
	InfName            string
}

// driverCollector collects third-party (OEM) driver packages via WMI.
type driverCollector struct {
	// queryFn returns the raw driver records. It is a field so tests can supply
	// fixtures instead of reaching WMI; nil means use the real query.
	queryFn func() ([]win32PnPSignedDriver, error)
}

// queryPnPSignedDrivers runs the WMI query against the local machine.
func queryPnPSignedDrivers() ([]win32PnPSignedDriver, error) {
	wmiClient := &wmi.Client{}
	swbemServices, err := wmi.InitializeSWbemServices(wmiClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize WMI services: %w", err)
	}
	defer func() {
		if closeErr := swbemServices.Close(); closeErr != nil {
			log.Errorf("error closing SWbemServicesClient: %v", closeErr)
		}
	}()
	wmiClient.SWbemServicesClient = swbemServices

	var drivers []win32PnPSignedDriver
	if err := wmiClient.SWbemServicesClient.Query(
		"SELECT DeviceClass, Description, DriverProviderName, DriverVersion, InfName FROM Win32_PnPSignedDriver",
		&drivers,
	); err != nil {
		return nil, fmt.Errorf("failed to query Win32_PnPSignedDriver: %w", err)
	}

	return drivers, nil
}

// Collect returns one entry per installed OEM driver package.
// A failure to enumerate is fatal; individual unusable records are reported as warnings.
func (c *driverCollector) Collect() ([]*Entry, []*Warning, error) {
	query := c.queryFn
	if query == nil {
		query = queryPnPSignedDrivers
	}

	drivers, err := query()
	if err != nil {
		return nil, nil, err
	}

	var warnings []*Warning
	// Win32_PnPSignedDriver returns one record per device binding, so a package
	// installed against several devices shows up repeatedly. Collapse to one entry
	// per package, keeping the highest version seen.
	byPackage := make(map[string]*Entry)

	for _, driver := range drivers {
		if !oemInfPattern.MatchString(driver.InfName) {
			// Inbox driver, or a record with no published INF: out of scope.
			continue
		}

		provider := strings.TrimSpace(driver.DriverProviderName)
		description := strings.TrimSpace(driver.Description)
		version := trimVersion(strings.TrimSpace(driver.DriverVersion))
		if provider == "" || description == "" || strings.TrimSpace(driver.DriverVersion) == "" {
			warnings = append(warnings, warnf("skipping driver %q: missing provider, description or version", driver.InfName))
			continue
		}

		// Identity intentionally excludes InfName: the oemNN number is reassigned when
		// a package is republished, which would make every driver update look like an
		// uninstall followed by a fresh install.
		productCode := strings.ToLower(provider + "|" + strings.TrimSpace(driver.DeviceClass) + "|" + description)

		if existing, ok := byPackage[productCode]; ok {
			if compareVersions(version, existing.Version) > 0 {
				existing.Version = version
			}
			continue
		}

		byPackage[productCode] = &Entry{
			Source:      softwareTypeDriver,
			DisplayName: description,
			Version:     version,
			Publisher:   provider,
			Status:      "installed",
			ProductCode: productCode,
			Is64Bit:     runtime.GOARCH != "386",
			// InstallDate is left empty on purpose: Win32_PnPSignedDriver only exposes
			// DriverDate, which is the vendor's build date rather than an install time.
		}
	}

	entries := make([]*Entry, 0, len(byPackage))
	for _, entry := range byPackage {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings, nil
}
