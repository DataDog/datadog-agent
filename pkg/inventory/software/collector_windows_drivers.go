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

// driverEntry builds a single entry from every device binding of one INF package.
// records must be non-empty and already validated.
//
// The product code deliberately excludes the INF name, because Windows reassigns the
// oemNN number when a package is republished and every driver update would otherwise look
// like an uninstall followed by a fresh install. It is instead derived from provider,
// device class and a representative description, all of which survive a version bump.
// The representative is the lowest description in the group so that the identity does not
// depend on the order WMI happens to return bindings in.
func driverEntry(records []win32PnPSignedDriver) *Entry {
	// Provider and class are declared once in the INF, so every binding of a package
	// reports the same pair and any record can supply them. Only the description and the
	// version vary between bindings.
	provider := strings.TrimSpace(records[0].DriverProviderName)
	deviceClass := strings.TrimSpace(records[0].DeviceClass)

	description := strings.TrimSpace(records[0].Description)
	version := trimVersion(strings.TrimSpace(records[0].DriverVersion))
	for _, record := range records[1:] {
		if candidate := strings.TrimSpace(record.Description); candidate < description {
			description = candidate
		}
		if candidate := trimVersion(strings.TrimSpace(record.DriverVersion)); compareVersions(candidate, version) > 0 {
			version = candidate
		}
	}

	return &Entry{
		Source:      softwareTypeDriver,
		DisplayName: description,
		Version:     version,
		Publisher:   provider,
		Status:      "installed",
		ProductCode: strings.ToLower(provider + "|" + deviceClass + "|" + description),
		Is64Bit:     runtime.GOARCH != "386",
		// InstallDate is left empty on purpose: Win32_PnPSignedDriver only exposes
		// DriverDate, which is the vendor's build date rather than an install time.
	}
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
	// Win32_PnPSignedDriver returns one record per device binding, so a package bound to
	// several devices shows up repeatedly — and those records can carry different
	// descriptions when one INF supports multiple device models. Group by INF name, which
	// is the real package boundary in this snapshot.
	byInf := make(map[string][]win32PnPSignedDriver)

	for _, driver := range drivers {
		if !oemInfPattern.MatchString(driver.InfName) {
			// Inbox driver, or a record with no published INF: out of scope.
			continue
		}

		if strings.TrimSpace(driver.DriverProviderName) == "" ||
			strings.TrimSpace(driver.Description) == "" ||
			strings.TrimSpace(driver.DriverVersion) == "" {
			warnings = append(warnings, warnf("skipping driver %q: missing provider, description or version", driver.InfName))
			continue
		}

		infName := strings.ToLower(driver.InfName)
		byInf[infName] = append(byInf[infName], driver)
	}

	// Key the result by product code rather than INF name. The two are not
	// interchangeable: the INF name identifies the package only within this snapshot,
	// while the product code has to survive updates. Two INF packages that agree on
	// provider, class and description collapse here, which keeps entry IDs unique.
	byProductCode := make(map[string]*Entry, len(byInf))

	for _, records := range byInf {
		entry := driverEntry(records)
		if existing, ok := byProductCode[entry.ProductCode]; ok {
			if compareVersions(entry.Version, existing.Version) > 0 {
				existing.Version = entry.Version
			}
			continue
		}
		byProductCode[entry.ProductCode] = entry
	}

	entries := make([]*Entry, 0, len(byProductCode))
	for _, entry := range byProductCode {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings, nil
}
