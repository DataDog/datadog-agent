// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Component Based Servicing (CBS) store, the authoritative record of which
// servicing packages are installed on the machine.
const (
	cbsPackagesKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\Packages`
	// cbsStateInstalled (0x70) is the CurrentState value for a fully installed package.
	cbsStateInstalled = 112
	// CBS package registry values
	cbsCurrentState    = "CurrentState"
	cbsInstallTimeHigh = "InstallTimeHigh"
	cbsInstallTimeLow  = "InstallTimeLow"
)

// cbsPackagePattern matches an installed servicing package and captures its family,
// e.g. "RollupFix" out of "Package_for_RollupFix~31bf3856ad364e35~amd64~~19041.2075.1.1".
// The family is the update's identity: a KB number for individually shipped updates, or
// a rolling name for updates whose registry key carries no KB number at all — most
// importantly "RollupFix", which is how monthly cumulative updates appear on Windows 10
// and 11. Matching only "Package_for_KB…" would drop those silently and report a
// plausible but materially incomplete set of updates.
//
// Component manifests such as "Microsoft-Windows-Foo-Package~…" do not use the
// "Package_for_" prefix and are excluded by it.
var cbsPackagePattern = regexp.MustCompile(`(?i)^Package_for_([A-Za-z0-9_.]+?)~`)

// cbsKBPattern matches a package family that is a KB article id
var cbsKBPattern = regexp.MustCompile(`(?i)^KB\d{6,7}$`)

// osUpdateCollector collects installed OS updates from the CBS servicing store.
type osUpdateCollector struct{}

// cbsPackage is one installed servicing package.
type cbsPackage struct {
	// identity is the package family: a KB id, or a rolling name such as "RollupFix"
	identity string
	// version is the trailing component of the package key name
	version string
	// installDate is RFC3339-formatted, empty when the package records no install time
	installDate string
	// installTime is the raw FILETIME in nanoseconds, used to pick the newest package
	installTime int64
}

// parseCBSPackageKey extracts the update identity and package version from a CBS package
// key name. ok is false when the key is not a servicing package.
func parseCBSPackageKey(name string) (identity string, version string, ok bool) {
	match := cbsPackagePattern.FindStringSubmatch(name)
	if match == nil {
		return "", "", false
	}

	identity = match[1]
	if cbsKBPattern.MatchString(identity) {
		// Normalize so "kb5061234" and "KB5061234" are the same update
		identity = strings.ToUpper(identity)
	}

	// The version is the last "~"-delimited component, e.g. "10.0.1.5"
	parts := strings.Split(name, "~")
	return identity, parts[len(parts)-1], true
}

// cbsInstallTime converts the split FILETIME stored by CBS into an RFC3339 timestamp
// and its nanosecond value. A zero FILETIME means the package records no install time.
func cbsInstallTime(high, low uint32) (string, int64) {
	if high == 0 && low == 0 {
		return "", 0
	}

	ft := windows.Filetime{HighDateTime: high, LowDateTime: low}
	ns := ft.Nanoseconds()
	return time.Unix(0, ns).UTC().Format(time.RFC3339Nano), ns
}

// buildOSUpdateEntries collapses servicing packages into one entry per update identity.
// Several packages can service the same update, and rolling families such as RollupFix
// keep the same identity across months, so the newest install time wins with the higher
// version breaking ties. A rolling family therefore reads as a version change on one
// entry rather than a removal followed by a new install.
func buildOSUpdateEntries(packages []cbsPackage) []*Entry {
	newest := make(map[string]cbsPackage, len(packages))
	for _, pkg := range packages {
		existing, ok := newest[pkg.identity]
		if !ok {
			newest[pkg.identity] = pkg
			continue
		}
		if pkg.installTime > existing.installTime ||
			(pkg.installTime == existing.installTime && compareVersions(pkg.version, existing.version) > 0) {
			newest[pkg.identity] = pkg
		}
	}

	entries := make([]*Entry, 0, len(newest))
	for _, pkg := range newest {
		entries = append(entries, &Entry{
			Source:      softwareTypeOSUpdate,
			DisplayName: "Update " + pkg.identity,
			Version:     pkg.version,
			InstallDate: pkg.installDate,
			Publisher:   "Microsoft Corporation",
			Status:      "installed",
			ProductCode: pkg.identity,
			Is64Bit:     true,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries
}

// Collect returns one entry per installed update, identified by KB number where the
// servicing store records one and by package family otherwise.
// Failing to enumerate the servicing store is fatal: reporting no updates would make
// the whole family look uninstalled.
func (c *osUpdateCollector) Collect() ([]*Entry, []*Warning, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, cbsPackagesKey,
		registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open CBS packages key: %w", err)
	}
	defer func() { _ = key.Close() }()

	names, err := key.ReadSubKeyNames(wantAll)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CBS package subkeys: %w", err)
	}

	var warnings []*Warning
	var packages []cbsPackage

	for _, name := range names {
		identity, version, ok := parseCBSPackageKey(name)
		if !ok {
			continue
		}

		subkey, err := registry.OpenKey(key, name, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			warnings = append(warnings, warnf("failed to open CBS package %s: %v", name, err))
			continue
		}

		state, _, err := subkey.GetIntegerValue(cbsCurrentState)
		if err != nil || state != cbsStateInstalled {
			// Superseded, staged or partially installed packages are not reported.
			_ = subkey.Close()
			continue
		}

		high, _, _ := subkey.GetIntegerValue(cbsInstallTimeHigh)
		low, _, _ := subkey.GetIntegerValue(cbsInstallTimeLow)
		_ = subkey.Close()

		if version == "" {
			warnings = append(warnings, warnf("skipping CBS package %s: no version in key name", name))
			continue
		}

		installDate, installTime := cbsInstallTime(uint32(high), uint32(low))
		packages = append(packages, cbsPackage{
			identity:    identity,
			version:     version,
			installDate: installDate,
			installTime: installTime,
		})
	}

	return buildOSUpdateEntries(packages), warnings, nil
}
