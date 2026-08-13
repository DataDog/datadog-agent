// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// errNoVersionResource stands in for a driver binary that carries no version resource at all.
var errNoVersionResource = errors.New("no version information")

// driverCollectorFor builds a collector backed by fixture records. Version information is
// keyed by resolved path and matched case-insensitively, the way the filesystem would; a path
// with no entry behaves like a binary carrying no version resource.
func driverCollectorFor(records []win32SystemDriver, versions map[string]winutil.FileVersionInfo) *driverCollector {
	return &driverCollector{
		queryFn: func() ([]win32SystemDriver, error) { return records, nil },
		versionInfoFn: func(path string) (winutil.FileVersionInfo, error) {
			for candidate, info := range versions {
				if strings.EqualFold(candidate, path) {
					return info, nil
				}
			}
			return winutil.FileVersionInfo{}, errNoVersionResource
		},
	}
}

// underWindowsDir builds a path below the real Windows directory. Collect resolves
// \SystemRoot\ against it, and it is not spelled the same way on every host ("C:\Windows",
// "C:\WINDOWS", or another volume entirely), so expectations must be derived rather than
// hardcoded.
func underWindowsDir(t *testing.T, rel string) string {
	t.Helper()

	windowsDir, err := windows.GetSystemWindowsDirectory()
	require.NoError(t, err)
	return filepath.Join(windowsDir, rel)
}

func TestResolveDriverPath(t *testing.T) {
	const windowsDir = `C:\Windows`

	t.Setenv("SystemRoot", windowsDir)

	tests := []struct {
		name     string
		pathName string
		expected string
	}{
		{
			name:     "already rooted at a drive letter is returned untouched",
			pathName: `C:\Program Files\Vendor\driver.sys`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "a UNC path is returned untouched",
			pathName: `\\fileserver\share\driver.sys`,
			expected: `\\fileserver\share\driver.sys`,
		},
		{
			name:     "forward slashes after a drive letter are left alone",
			pathName: `C:/Windows/System32/drivers/driver.sys`,
			expected: `C:/Windows/System32/drivers/driver.sys`,
		},
		{
			name:     "the NT object-manager prefix is stripped",
			pathName: `\??\C:\Program Files\Vendor\driver.sys`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "SystemRoot resolves against the Windows directory",
			pathName: `\SystemRoot\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "SystemRoot is matched case-insensitively",
			pathName: `\systemroot\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "a bare relative path resolves against the Windows directory",
			pathName: `System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "a leading separator is not a root",
			pathName: `\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "environment variables are expanded",
			pathName: `%SystemRoot%\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "surrounding whitespace is ignored",
			pathName: "  " + `C:\Windows\System32\drivers\driver.sys` + "  ",
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := resolveDriverPath(test.pathName, windowsDir)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveDriverPathRejectsEmptyPaths(t *testing.T) {
	for _, pathName := range []string{"", "   ", "\t"} {
		_, err := resolveDriverPath(pathName, `C:\Windows`)
		assert.Error(t, err, "an empty image path is not resolvable")
	}
}

func TestExpandWinEnvLeavesUnknownVariablesInPlace(t *testing.T) {
	// Dropping an unresolved variable would silently produce a path pointing somewhere else,
	// which is worse than reporting the original text and failing to read it.
	assert.Equal(t, `%NotASetVariable%\driver.sys`, expandWinEnv(`%NotASetVariable%\driver.sys`))
}

func TestDriverCollectorReportsRegisteredDrivers(t *testing.T) {
	resolved := underWindowsDir(t, `system32\drivers\wd\WdFilter.sys`)

	collector := driverCollectorFor(
		[]win32SystemDriver{{
			Name:        "WdFilter",
			DisplayName: "Microsoft Defender Antivirus Mini-Filter Driver",
			PathName:    `\SystemRoot\system32\drivers\wd\WdFilter.sys`,
		}},
		map[string]winutil.FileVersionInfo{
			resolved: {FileVersionNumeric: "4.18.24090.11", CompanyName: "Microsoft Corporation"},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, softwareTypeDriver, entry.Source)
	assert.Equal(t, "Microsoft Defender Antivirus Mini-Filter Driver", entry.DisplayName)
	assert.Equal(t, "4.18.24090.11", entry.Version)
	assert.Equal(t, "Microsoft Corporation", entry.Publisher)
	assert.Equal(t, "installed", entry.Status)
	assert.Equal(t, "wdfilter", entry.ProductCode, "the service name is the identity")
	assert.Equal(t, resolved, entry.InstallPath,
		"the resolved path is what the aggregator mirrors into install_paths")
	assert.Empty(t, entry.InstallDate, "nothing records when a driver was installed")
}

func TestDriverCollectorReportsSoftwareOnlyDrivers(t *testing.T) {
	// The reason this collector uses Win32_SystemDriver at all: a minifilter installed
	// outside the driver store has no PnP device node and no OEM INF, so the PnP class
	// cannot see it.
	collector := driverCollectorFor(
		[]win32SystemDriver{{
			Name:        "CSAgent",
			DisplayName: "CrowdStrike Falcon Sensor Driver",
			PathName:    `\??\C:\Program Files\CrowdStrike\csagent.sys`,
		}},
		map[string]winutil.FileVersionInfo{
			`C:\Program Files\CrowdStrike\csagent.sys`: {
				FileVersionNumeric: "7.20.19507.0",
				CompanyName:        "CrowdStrike, Inc.",
			},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)
	assert.Equal(t, "csagent", entries[0].ProductCode)
	assert.Equal(t, "7.20.19507.0", entries[0].Version)
}

func TestDriverIdentitySurvivesVersionBump(t *testing.T) {
	// The backend keys a software item on name, publisher, product_code, architecture and
	// software type, deliberately excluding the version, so that a version bump reads as an
	// update. Neither the version nor a relocated binary may change the identity.
	const displayName = "Vendor Filter Driver"

	before, _, err := driverCollectorFor(
		[]win32SystemDriver{{Name: "VendorFlt", DisplayName: displayName, PathName: `C:\Vendor\1.0\vendorflt.sys`}},
		map[string]winutil.FileVersionInfo{
			`C:\Vendor\1.0\vendorflt.sys`: {FileVersionNumeric: "1.0.0.1", CompanyName: "Vendor"},
		},
	).Collect()
	require.NoError(t, err)
	require.Len(t, before, 1)

	after, _, err := driverCollectorFor(
		[]win32SystemDriver{{Name: "VendorFlt", DisplayName: displayName, PathName: `C:\Vendor\2.0\vendorflt.sys`}},
		map[string]winutil.FileVersionInfo{
			`C:\Vendor\2.0\vendorflt.sys`: {FileVersionNumeric: "2.0.0.0", CompanyName: "Vendor"},
		},
	).Collect()
	require.NoError(t, err)
	require.Len(t, after, 1)

	assert.Equal(t, before[0].ProductCode, after[0].ProductCode)
	assert.Equal(t, before[0].DisplayName, after[0].DisplayName)
	assert.NotEqual(t, before[0].Version, after[0].Version)
}

func TestDriverCollectorRequiresANumericVersion(t *testing.T) {
	// The FileVersion string is not accepted as a substitute: it is routinely decorated or
	// comma-separated, and guessing at those formats is worse than reporting nothing.
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "NoResource", PathName: `C:\Windows\System32\drivers\noresource.sys`},
			{Name: "ZeroVersion", PathName: `C:\Windows\System32\drivers\zero.sys`},
			{Name: "Good", PathName: `C:\Windows\System32\drivers\good.sys`},
		},
		map[string]winutil.FileVersionInfo{
			// A resource that was never given a FILEVERSION. The decorated string must not
			// rescue it.
			`C:\Windows\System32\drivers\zero.sys`: {
				FileVersionNumeric: "0.0.0.0",
				FileVersion:        "6.3.9600.17415 built by: WinBlue",
				CompanyName:        "Vendor",
			},
			`C:\Windows\System32\drivers\good.sys`: {FileVersionNumeric: "1.2.3.4"},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err, "unusable records degrade to warnings, they do not fail the snapshot")
	assert.Len(t, warnings, 2)
	require.Len(t, entries, 1)
	assert.Equal(t, "good", entries[0].ProductCode)
}

func TestDriverCollectorSkipsUnusableRecords(t *testing.T) {
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "", DisplayName: "No Service Name", PathName: `C:\Windows\System32\drivers\a.sys`},
			{Name: "NoPath", DisplayName: "No Path", PathName: ""},
			{Name: "Good", DisplayName: "Good Driver", PathName: `C:\Windows\System32\drivers\good.sys`},
		},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\a.sys`:    {FileVersionNumeric: "1.0.0.0"},
			`C:\Windows\System32\drivers\good.sys`: {FileVersionNumeric: "1.0.0.0"},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Len(t, warnings, 2)
	require.Len(t, entries, 1)
	assert.Equal(t, "Good Driver", entries[0].DisplayName)
}

func TestDriverCollectorFallsBackToTheServiceName(t *testing.T) {
	collector := driverCollectorFor(
		[]win32SystemDriver{{Name: "ACPI", DisplayName: "", PathName: `C:\Windows\System32\drivers\acpi.sys`}},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\acpi.sys`: {FileVersionNumeric: "10.0.19041.1"},
		},
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "ACPI", entries[0].DisplayName, "an empty display name falls back to the service name")
}

func TestDriverCollectorEmitsAnEmptyPublisher(t *testing.T) {
	// A driver with no CompanyName is still worth reporting: an unattributed kernel driver is
	// exactly the kind of thing an operator wants to see.
	collector := driverCollectorFor(
		[]win32SystemDriver{{Name: "Unattributed", DisplayName: "Unattributed Driver", PathName: `C:\Windows\System32\drivers\u.sys`}},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\u.sys`: {FileVersionNumeric: "1.0.0.0", CompanyName: ""},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].Publisher)
}

func TestDriverCollectorKeepsDriversSharingADisplayName(t *testing.T) {
	// Vendors ship several drivers under one product name. The service name keeps them apart.
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "VendorFlt", DisplayName: "Vendor Endpoint Protection", PathName: `C:\Vendor\flt.sys`},
			{Name: "VendorNet", DisplayName: "Vendor Endpoint Protection", PathName: `C:\Vendor\net.sys`},
		},
		map[string]winutil.FileVersionInfo{
			`C:\Vendor\flt.sys`: {FileVersionNumeric: "1.0.0.0"},
			`C:\Vendor\net.sys`: {FileVersionNumeric: "1.0.0.0"},
		},
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 2)

	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.GetID()] = struct{}{}
	}
	assert.Len(t, ids, 2, "entry IDs must stay unique")
}

func TestDriverCollectorQueryFailureIsFatal(t *testing.T) {
	collector := &driverCollector{
		queryFn: func() ([]win32SystemDriver, error) { return nil, errors.New("WMI unavailable") },
	}

	entries, _, err := collector.Collect()

	assert.Error(t, err, "a failed enumeration is an unknown state, not an empty one")
	assert.Empty(t, entries)
}

func TestDriverCollectorEmptyResultIsNotAnError(t *testing.T) {
	entries, warnings, err := driverCollectorFor(nil, nil).Collect()

	assert.NoError(t, err)
	assert.Empty(t, entries)
	assert.Empty(t, warnings)
}
