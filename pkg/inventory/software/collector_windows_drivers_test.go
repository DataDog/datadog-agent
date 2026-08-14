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

// driverCollectorFor builds a collector backed by fixture records for the service source.
// Version information is keyed by resolved path and matched case-insensitively, the way the
// filesystem would; a path with no entry behaves like a binary carrying no version resource.
//
// The PnP source is stubbed empty, so that a test of one source cannot reach WMI for the other.
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
		// Nothing resolves by default: fixtures that care supply their own resolver.
		loadIndirectFn:   func(string) (string, error) { return "", errUnresolvedIndirectString },
		pnpDriverQueryFn: func() ([]win32PnPSignedDriver, error) { return nil, nil },
		pnpEntityQueryFn: func() ([]win32PnPEntity, error) { return nil, nil },
	}
}

// pnpDriverCollectorFor builds a collector backed by fixture records for the PnP source, with
// the service source stubbed empty. Devices absent from entities exercise the missing-device
// path, where the driver is kept.
func pnpDriverCollectorFor(records []win32PnPSignedDriver, entities []win32PnPEntity) *driverCollector {
	return &driverCollector{
		queryFn:          func() ([]win32SystemDriver, error) { return nil, nil },
		pnpDriverQueryFn: func() ([]win32PnPSignedDriver, error) { return records, nil },
		pnpEntityQueryFn: func() ([]win32PnPEntity, error) { return entities, nil },
	}
}

// errUnresolvedIndirectString stands in for a resource reference that cannot be loaded,
// which happens on real hosts when the module is missing or lacks the string.
var errUnresolvedIndirectString = errors.New("unresolved indirect string")

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
	// A third-party vendor on purpose: the Defender driver this fixture used to carry is now
	// filtered out, since only OEM and third-party drivers are reported.
	resolved := underWindowsDir(t, `system32\drivers\SentinelMonitor.sys`)

	collector := driverCollectorFor(
		[]win32SystemDriver{{
			Name:        "SentinelMonitor",
			DisplayName: "SentinelOne Monitor Driver",
			PathName:    `\SystemRoot\system32\drivers\SentinelMonitor.sys`,
		}},
		map[string]winutil.FileVersionInfo{
			resolved: {FileVersionNumeric: "23.4.2.377", CompanyName: "Sentinel Labs, Inc."},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, softwareTypeDriver, entry.Source)
	assert.Equal(t, "SentinelOne Monitor Driver", entry.DisplayName)
	assert.Equal(t, "23.4.2.377", entry.Version)
	assert.Equal(t, "Sentinel Labs, Inc.", entry.Publisher)
	assert.Equal(t, "installed", entry.Status)
	assert.Equal(t, "sentinelmonitor", entry.ProductCode, "the service name is the identity")
	assert.Equal(t, resolved, entry.InstallPath,
		"the resolved path is what the aggregator mirrors into install_paths")
	assert.Empty(t, entry.InstallDate, "nothing records when a driver was installed")
}

func TestDriverCollectorDropsMicrosoftPublishedDrivers(t *testing.T) {
	// Inbox drivers number in the hundreds on any host and say nothing about it. CompanyName
	// is the only vendor signal Win32_SystemDriver offers, and drivers spell it several ways.
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "WdFilter", DisplayName: "Defender Mini-Filter", PathName: `C:\Windows\System32\drivers\WdFilter.sys`},
			{Name: "tcpip", DisplayName: "TCP/IP Protocol Driver", PathName: `C:\Windows\System32\drivers\tcpip.sys`},
			{Name: "acpi", DisplayName: "ACPI Driver", PathName: `C:\Windows\System32\drivers\acpi.sys`},
			{Name: "Unattributed", DisplayName: "Unattributed Driver", PathName: `C:\Vendor\u.sys`},
			{Name: "VendorFlt", DisplayName: "Vendor Filter", PathName: `C:\Vendor\flt.sys`},
		},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\WdFilter.sys`: {FileVersionNumeric: "4.18.24090.11", CompanyName: "Microsoft Corporation"},
			`C:\Windows\System32\drivers\tcpip.sys`:    {FileVersionNumeric: "10.0.26100.1", CompanyName: "Microsoft Windows"},
			`C:\Windows\System32\drivers\acpi.sys`:     {FileVersionNumeric: "10.0.26100.1", CompanyName: "  microsoft  "},
			`C:\Vendor\u.sys`:                          {FileVersionNumeric: "1.0.0.0", CompanyName: ""},
			`C:\Vendor\flt.sys`:                        {FileVersionNumeric: "1.0.0.0", CompanyName: "Vendor GmbH"},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings, "an inbox driver is a deliberate exclusion, not an unusable record")

	reported := make([]string, 0, len(entries))
	for _, entry := range entries {
		reported = append(reported, entry.ProductCode)
	}
	// The unattributed driver is kept: an unattributed kernel driver is exactly the kind of
	// thing an operator wants to see.
	assert.Equal(t, []string{"unattributed", "vendorflt"}, reported)
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

func TestResolveDisplayName(t *testing.T) {
	// Windows lets a service store its display name as an indirect resource reference so that
	// it can be localized. WMI resolves most of them but not all, and both unresolved forms
	// below were observed in a real payload.
	resolves := func(string) (string, error) { return "TCP/IP Registry Compatibility", nil }
	fails := func(string) (string, error) { return "", errUnresolvedIndirectString }

	tests := []struct {
		name        string
		displayName string
		load        func(string) (string, error)
		expected    string
	}{
		{
			name:        "a plain display name is passed through",
			displayName: "Microsoft Defender Antivirus Mini-Filter Driver",
			load:        fails,
			expected:    "Microsoft Defender Antivirus Mini-Filter Driver",
		},
		{
			name:        "an empty display name falls back to the service name",
			displayName: "",
			load:        fails,
			expected:    "TheService",
		},
		{
			name:        "a resolvable reference is resolved",
			displayName: `@%SystemRoot%\System32\drivers\tcpipreg.sys,-10110,`,
			load:        resolves,
			expected:    "TCP/IP Registry Compatibility",
		},
		{
			name:        "an unresolvable reference falls back to the service name",
			displayName: `@%SystemRoot%\System32\drivers\tcpipreg.sys,-10110,`,
			load:        fails,
			expected:    "TheService",
		},
		{
			name:        "an unresolvable reference uses its comment text when it has one",
			displayName: "@todo.dll,-100;Microsoft IPv6 Protocol Driver",
			load:        fails,
			expected:    "Microsoft IPv6 Protocol Driver",
		},
		{
			name:        "a resolver that echoes the reference back is not trusted",
			displayName: "@todo.dll,-100;Microsoft IPv6 Protocol Driver",
			load:        func(s string) (string, error) { return s, nil },
			expected:    "Microsoft IPv6 Protocol Driver",
		},
		{
			name:        "a resolver returning blank text falls back",
			displayName: "@todo.dll,-100",
			load:        func(string) (string, error) { return "   ", nil },
			expected:    "TheService",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, resolveDisplayName(test.displayName, "TheService", test.load))
		})
	}
}

func TestDriverCollectorNeverReportsARawResourceReference(t *testing.T) {
	// Regression: these two shipped a file path and a module reference as the software name,
	// which is also part of the backend's identity key.
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "tcpipreg", DisplayName: `@%SystemRoot%\System32\drivers\tcpipreg.sys,-10110,`, PathName: `C:\Windows\System32\drivers\tcpipreg.sys`},
			{Name: "tcpip6", DisplayName: "@todo.dll,-100;Microsoft IPv6 Protocol Driver", PathName: `C:\Windows\System32\drivers\tcpip.sys`},
		},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\tcpipreg.sys`: {FileVersionNumeric: "6.2.26100.9168"},
			`C:\Windows\System32\drivers\tcpip.sys`:    {FileVersionNumeric: "6.2.26100.9168"},
		},
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		assert.NotContains(t, entry.DisplayName, "@", "a raw resource reference reached the payload")
	}
}

func TestDriverCollectorKeepsServicesSharingABinary(t *testing.T) {
	// tcpip and tcpip6 are both served by tcpip.sys on a real host. Keying on the image path
	// would merge them; the service name keeps them apart.
	collector := driverCollectorFor(
		[]win32SystemDriver{
			{Name: "tcpip", DisplayName: "TCP/IP Protocol Driver", PathName: `C:\Windows\System32\drivers\tcpip.sys`},
			{Name: "tcpip6", DisplayName: "Microsoft IPv6 Protocol Driver", PathName: `C:\Windows\System32\drivers\tcpip.sys`},
		},
		map[string]winutil.FileVersionInfo{
			`C:\Windows\System32\drivers\tcpip.sys`: {FileVersionNumeric: "6.2.26100.9168"},
		},
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 2, "two services sharing one binary are two drivers")
	assert.Equal(t, "tcpip", entries[0].ProductCode)
	assert.Equal(t, "tcpip6", entries[1].ProductCode)
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

func TestDriverCollectorReportsOEMPnPDriversWithoutAService(t *testing.T) {
	// The reason this collector queries Win32_PnPSignedDriver at all: a driver package can be
	// installed and bound to a device without registering a kernel service, so the service
	// source cannot see it.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{{
			DeviceID:      `PCI\VEN_1179&DEV_011A&SUBSYS_00011179\4&2A3B7C1D&0&00E4`,
			DeviceName:    "KIOXIA KXG80ZNV1T02 NVMe",
			DriverVersion: "2.2.1.14",
			Manufacturer:  "KIOXIA Corporation",
			InfName:       "oem42.inf",
		}},
		[]win32PnPEntity{{
			DeviceID: `PCI\VEN_1179&DEV_011A&SUBSYS_00011179\4&2A3B7C1D&0&00E4`,
			Service:  "",
		}},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, softwareTypeDriver, entry.Source)
	assert.Equal(t, "KIOXIA KXG80ZNV1T02 NVMe", entry.DisplayName)
	assert.Equal(t, "2.2.1.14", entry.Version)
	assert.Equal(t, "KIOXIA Corporation", entry.Publisher)
	assert.Equal(t, "installed", entry.Status)
	assert.Equal(t, "kioxia kxg80znv1t02 nvme", entry.ProductCode, "the device name is the identity")
	assert.Empty(t, entry.InstallPath, "this class names no binary")
	assert.Empty(t, entry.InstallDate)
}

func TestDriverCollectorSkipsPnPDriversWithAService(t *testing.T) {
	// A device driven by a service is already reported by the service source under that
	// service name. A device absent from Win32_PnPEntity is kept: unknown is not the same as
	// present, and the permissive choice matches keeping a driver whose vendor is unknown.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{
			{DeviceID: `PCI\HAS_SERVICE`, DeviceName: "Bound To A Service", DriverVersion: "1.0", InfName: "oem1.inf"},
			{DeviceID: `PCI\NO_SERVICE`, DeviceName: "No Service", DriverVersion: "1.0", InfName: "oem2.inf"},
			{DeviceID: `PCI\NOT_A_DEVICE`, DeviceName: "Missing From Entities", DriverVersion: "1.0", InfName: "oem3.inf"},
			// The join must survive the two classes disagreeing on case.
			{DeviceID: `pci\has_service_lowercase`, DeviceName: "Case Mismatch", DriverVersion: "1.0", InfName: "oem4.inf"},
		},
		[]win32PnPEntity{
			{DeviceID: `PCI\HAS_SERVICE`, Service: "vendorbus"},
			{DeviceID: `PCI\NO_SERVICE`, Service: ""},
			{DeviceID: `PCI\HAS_SERVICE_LOWERCASE`, Service: "vendorbus"},
		},
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)

	reported := make([]string, 0, len(entries))
	for _, entry := range entries {
		reported = append(reported, entry.DisplayName)
	}
	assert.ElementsMatch(t, []string{"No Service", "Missing From Entities"}, reported)
}

func TestDriverCollectorSkipsInboxPnPDrivers(t *testing.T) {
	// Windows renames a package that did not ship inside Windows to "oemNN.inf", so the INF
	// name is the third-party test. The manufacturer is not, and these two fixtures are why:
	// Microsoft's own inbox INFs mostly report generic strings, and Windows ships inbox INFs
	// authored by hardware vendors. Filtering on the manufacturer would keep both.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{
			{DeviceID: `PCI\A`, DeviceName: "PCI Express Root Complex", DriverVersion: "10.0.26100.1",
				Manufacturer: "(Standard system devices)", InfName: "machine.inf"},
			{DeviceID: `PCI\B`, DeviceName: "Intel(R) Ethernet Connection I219-LM", DriverVersion: "12.19.2.60",
				Manufacturer: "Intel", InfName: "e1d68x64.inf"},
			{DeviceID: `PCI\C`, DeviceName: "Vendor Widget", DriverVersion: "3.1.0.0",
				Manufacturer: "Vendor", InfName: "OEM7.INF"},
		},
		nil,
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings, "an inbox driver is a deliberate exclusion, not an unusable record")
	require.Len(t, entries, 1)
	assert.Equal(t, "vendor widget", entries[0].ProductCode, "the INF prefix is matched case-insensitively")
}

func TestDriverCollectorReportsMicrosoftPnPDriversPublishedAsOEM(t *testing.T) {
	// Deliberate asymmetry with the service source: a Microsoft package delivered out of band
	// by Windows Update is published as an OEM INF, and is genuinely not inbox, so it is
	// reported. This is not the bug it looks like.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{{
			DeviceID: `USB\MS_SURFACE`, DeviceName: "Surface Touch Controller", DriverVersion: "1.5.139.0",
			Manufacturer: "Microsoft", InfName: "oem19.inf",
		}},
		nil,
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Microsoft", entries[0].Publisher)
}

func TestDriverCollectorSkipsPnPDriversWithoutADeviceName(t *testing.T) {
	// The device name is the identity, so a record without one is unusable rather than
	// deliberately excluded, and that difference is what earns it a warning.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{
			{DeviceID: `PCI\A`, DeviceName: "   ", DriverVersion: "1.0", InfName: "oem1.inf"},
			// A version is mandatory for a package to be signable, so this should never occur;
			// it is dropped silently because an entry whose version can never advance is empty.
			{DeviceID: `PCI\B`, DeviceName: "No Version", DriverVersion: "", InfName: "oem2.inf"},
			{DeviceID: `PCI\C`, DeviceName: "Good", DriverVersion: "1.0", InfName: "oem3.inf"},
		},
		nil,
	)

	entries, warnings, err := collector.Collect()

	require.NoError(t, err, "unusable records degrade to warnings, they do not fail the snapshot")
	assert.Len(t, warnings, 1)
	require.Len(t, entries, 1)
	assert.Equal(t, "good", entries[0].ProductCode)
}

func TestPnPDriverIdentitySurvivesVersionBump(t *testing.T) {
	// The same requirement the service source has, and the reason the device name rather than
	// the INF name or the device id is the identity: oemNN.inf is renumbered when a package is
	// republished, and a device id embeds the instance path.
	const deviceName = "Vendor Widget"

	before, _, err := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{{
			DeviceID: `PCI\VEN_1AF4&DEV_1000\3&11583659&0&08`, DeviceName: deviceName,
			DriverVersion: "1.0.0.1", Manufacturer: "Vendor", InfName: "oem12.inf",
		}},
		nil,
	).Collect()
	require.NoError(t, err)
	require.Len(t, before, 1)

	after, _, err := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{{
			DeviceID: `PCI\VEN_1AF4&DEV_1000\3&11583659&0&09`, DeviceName: deviceName,
			DriverVersion: "2.0.0.0", Manufacturer: "Vendor", InfName: "oem31.inf",
		}},
		nil,
	).Collect()
	require.NoError(t, err)
	require.Len(t, after, 1)

	assert.Equal(t, before[0].ProductCode, after[0].ProductCode)
	assert.Equal(t, before[0].DisplayName, after[0].DisplayName)
	assert.NotEqual(t, before[0].Version, after[0].Version)
}

func TestDriverCollectorCollapsesPnPDevicesOfTheSameModel(t *testing.T) {
	// Two identical devices are one driver package, so one entry is the right answer.
	collector := pnpDriverCollectorFor(
		[]win32PnPSignedDriver{
			{DeviceID: `PCI\SLOT_1`, DeviceName: "Vendor 10G NIC", DriverVersion: "4.1.0.0", InfName: "oem5.inf"},
			{DeviceID: `PCI\SLOT_2`, DeviceName: "Vendor 10G NIC", DriverVersion: "4.1.0.0", InfName: "oem5.inf"},
		},
		nil,
	)

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestDriverCollectorMergesBothSources(t *testing.T) {
	collector := driverCollectorFor(
		[]win32SystemDriver{{Name: "VendorFlt", DisplayName: "Vendor Filter", PathName: `C:\Vendor\flt.sys`}},
		map[string]winutil.FileVersionInfo{
			`C:\Vendor\flt.sys`: {FileVersionNumeric: "1.0.0.0", CompanyName: "Vendor"},
		},
	)
	collector.pnpDriverQueryFn = func() ([]win32PnPSignedDriver, error) {
		return []win32PnPSignedDriver{{
			DeviceID: `PCI\A`, DeviceName: "Vendor Widget", DriverVersion: "3.1.0.0",
			Manufacturer: "Vendor", InfName: "oem7.inf",
		}}, nil
	}

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 2)

	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.GetID()] = struct{}{}
	}
	assert.Len(t, ids, 2, "entry IDs must stay unique across the two sources")
}

func TestDriverCollectorPnPQueryFailureIsFatal(t *testing.T) {
	// Same stance as the service source: a failed enumeration is an unknown state, not an
	// empty one, and reporting a truncated driver family reads as an uninstall.
	for _, test := range []struct {
		name      string
		collector *driverCollector
	}{
		{
			name: "the signed driver query fails",
			collector: &driverCollector{
				queryFn:          func() ([]win32SystemDriver, error) { return nil, nil },
				pnpDriverQueryFn: func() ([]win32PnPSignedDriver, error) { return nil, errors.New("WMI unavailable") },
			},
		},
		{
			name: "the device query fails",
			collector: &driverCollector{
				queryFn:          func() ([]win32SystemDriver, error) { return nil, nil },
				pnpDriverQueryFn: func() ([]win32PnPSignedDriver, error) { return nil, nil },
				pnpEntityQueryFn: func() ([]win32PnPEntity, error) { return nil, errors.New("WMI unavailable") },
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries, _, err := test.collector.Collect()

			assert.Error(t, err)
			assert.Empty(t, entries)
		})
	}
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
