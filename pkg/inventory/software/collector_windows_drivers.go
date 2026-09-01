// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// errEmptyImagePath reports a driver service that names no binary.
var errEmptyImagePath = errors.New("empty image path")

// zeroVersion is what the fixed version information reports when a driver ships a version
// resource but never set FILEVERSION. It carries no information, so it is treated as absent.
const zeroVersion = "0.0.0.0"

// driverCollector collects installed kernel-mode drivers.
//
// Only OEM and third-party drivers are reported. Microsoft's inbox drivers number in the
// hundreds on any host and say nothing about it, so they would swamp the drivers that do.
// The two sources need different tests for this, because they expose different fields:
// the service source is filtered on the vendor of the binary, the device source on whether
// the INF was published as an OEM package.
//
// Known limitations, both accepted:
//   - user-mode (UMDF) drivers register no kernel service and are not reported;
//   - a driver whose service entry is created, loaded and then deleted is invisible, so this
//     is an inventory of registered drivers rather than of code currently in the kernel.
type driverCollector struct {
	// serviceQueryFn returns the raw driver service records. It is a field so tests can
	// supply fixtures instead of reaching the SCM; nil means use the real query.
	serviceQueryFn func() ([]winutil.DriverService, error)
	// deviceQueryFn returns the raw device driver records. It is a field so tests can supply
	// fixtures instead of reaching SetupAPI; nil means use the real query.
	deviceQueryFn func() ([]winutil.DeviceDriver, error)
	// versionInfoFn reads the version resource of a driver binary. It is a field so tests
	// stay off the filesystem; nil means read the real file.
	versionInfoFn func(path string) (winutil.FileVersionInfo, error)
	// loadIndirectFn resolves an indirect string reference in a display name. It is a field
	// so tests stay off the filesystem; nil means resolve against the real module.
	loadIndirectFn func(source string) (string, error)
}

// resolveDriverPath turns the image path reported for a driver service into a Win32 path.
//
// windowsDir is passed in rather than looked up so that the resolution is deterministic and
// testable. The rules are ordered so that the prefix strip and the variable expansion, both
// of which can produce an already-rooted path, run before the checks that recognise one.
func resolveDriverPath(pathName string, windowsDir string) (string, error) {
	path := strings.TrimSpace(pathName)
	if path == "" {
		return "", errEmptyImagePath
	}

	// A service binary path containing spaces may be quoted and followed by arguments.
	// Extract the quoted executable before applying path normalization.
	if strings.HasPrefix(path, `"`) {
		closingQuote := strings.Index(path[1:], `"`)
		if closingQuote < 0 {
			return "", fmt.Errorf("unterminated quoted image path %q", pathName)
		}
		path = path[1 : closingQuote+1]
		if path == "" {
			return "", errEmptyImagePath
		}
	}

	// "\??\C:\Program Files\Vendor\driver.sys" is the NT object-manager form of a Win32 path.
	path = strings.TrimPrefix(path, `\??\`)

	// A few drivers store their path with variables, e.g. "%SystemRoot%\System32\drivers".
	if strings.Contains(path, "%") {
		expanded, err := winutil.ExpandEnvironmentStrings(path)
		if err != nil {
			return "", fmt.Errorf("failed to expand environment variables in %q: %w", pathName, err)
		}
		path = expanded
	}

	// Already rooted: a drive letter, or a UNC share.
	if filepath.IsAbs(path) {
		return path, nil
	}

	// "\SystemRoot\System32\drivers\driver.sys" is relative to the Windows directory.
	if rest, ok := cutPrefixFold(path, `\SystemRoot\`); ok {
		return filepath.Join(windowsDir, rest), nil
	}

	// Anything left is relative to the Windows directory, e.g. "System32\drivers\driver.sys".
	// A leading separator here is not a root: Windows resolves it against %SystemRoot%.
	return filepath.Join(windowsDir, strings.TrimLeft(path, `\/`)), nil
}

// resolveDisplayName turns the DisplayName of a driver service into text fit for a human.
//
// Windows lets a service store its display name as an indirect resource reference so that it
// can be localized, e.g. "@%SystemRoot%\System32\drivers\tcpipreg.sys,-10110". WMI resolves
// most of them, but not all, and shipping the raw reference would put a file path in the
// software name. The comment form "@todo.dll,-100;Microsoft IPv6 Protocol Driver" carries its
// own fallback text after the semicolon, which is used when the resource cannot be loaded.
//
// The service name is the last resort: it is always present and always meaningful.
func resolveDisplayName(displayName string, serviceName string, load func(string) (string, error)) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return serviceName
	}
	if !strings.HasPrefix(name, "@") {
		return name
	}

	if resolved, err := load(name); err == nil {
		if resolved = strings.TrimSpace(resolved); resolved != "" && !strings.HasPrefix(resolved, "@") {
			return resolved
		}
	}

	// The reference did not resolve. Anything after the first semicolon is fallback text the
	// author supplied for exactly this case.
	if _, comment, found := strings.Cut(name, ";"); found {
		if comment = strings.TrimSpace(comment); comment != "" {
			return comment
		}
	}

	return serviceName
}

// cutPrefixFold reports whether s starts with prefix, ignoring case, and returns the
// remainder when it does.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// isMicrosoftPublisher reports whether the CompanyName of a driver binary names Microsoft.
// Drivers spell it several ways — "Microsoft Corporation", "Microsoft Windows", plain
// "Microsoft" — so the prefix is what is matched.
//
// This filters the service source only. The device source carries an INF name, which is a
// far better signal; see collectDeviceDrivers.
func isMicrosoftPublisher(publisher string) bool {
	_, ok := cutPrefixFold(strings.TrimSpace(publisher), "microsoft")
	return ok
}

// isOEMInfName reports whether an INF was published as an OEM package.
//
// When Windows publishes a driver package that did not ship inside Windows, it renames the
// INF to "oemNN.inf". Inbox packages keep their original names, so the prefix separates
// third-party drivers from Microsoft's own.
func isOEMInfName(infName string) bool {
	_, ok := cutPrefixFold(strings.TrimSpace(infName), "oem")
	return ok
}

// Collect returns one entry per OEM or third-party kernel-mode driver, from both sources.
// A failure to enumerate either source is fatal; individual unusable records are reported as
// warnings.
func (c *driverCollector) Collect() ([]*Entry, []*Warning, error) {
	// Both sources group into one map, keyed on the identity described by each of them. The
	// service source is collected first, so where the two could name the same driver the
	// service entry wins — it is the richer of the two, carrying the image path.
	byProductCode := make(map[string]*Entry)

	warnings, err := c.collectServiceDrivers(byProductCode)
	if err != nil {
		return nil, nil, err
	}

	deviceWarnings, err := c.collectDeviceDrivers(byProductCode)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, deviceWarnings...)

	entries := make([]*Entry, 0, len(byProductCode))
	for _, entry := range byProductCode {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings, nil
}

// collectServiceDrivers adds the drivers registered as kernel services to byProductCode. The
// source is the SCM (winutil.EnumDriverServices), not Win32_SystemDriver.
//
// The identity is the service name. It is both the identity and the grouping key: Windows keys
// the Services registry on it, so it is unique on the host, and unlike the image path or the
// display name it does not change when the driver is updated. Keeping the version out of the
// identity is what lets a version bump read as an update rather than as an uninstall followed
// by a fresh install.
func (c *driverCollector) collectServiceDrivers(byProductCode map[string]*Entry) ([]*Warning, error) {
	query := c.serviceQueryFn
	if query == nil {
		query = winutil.EnumDriverServices
	}
	versionInfo := c.versionInfoFn
	if versionInfo == nil {
		versionInfo = winutil.GetFileVersionInfoStrings
	}
	loadIndirect := c.loadIndirectFn
	if loadIndirect == nil {
		loadIndirect = winutil.LoadIndirectString
	}

	drivers, err := query()
	if err != nil {
		return nil, err
	}

	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to locate the Windows directory: %w", err)
	}

	var warnings []*Warning

	for _, driver := range drivers {
		name := strings.TrimSpace(driver.Name)
		if name == "" {
			warnings = append(warnings, warnf("skipping driver with no service name (path %q)", driver.ImagePath))
			continue
		}

		path, err := resolveDriverPath(driver.ImagePath, windowsDir)
		if err != nil {
			warnings = append(warnings, warnf("skipping driver %q: %v", name, err))
			continue
		}

		info, err := versionInfo(path)
		if err != nil {
			warnings = append(warnings, warnf("skipping driver %q: no version information for %q: %v", name, path, err))
			continue
		}
		// Only the numeric file version is accepted. The FileVersion string carries the same
		// value but is routinely decorated ("10.0.19041.1 (WinBuild.160101.0800)") or
		// comma-separated, and guessing at those formats is worse than reporting nothing.
		if info.FileVersionNumeric == "" || info.FileVersionNumeric == zeroVersion {
			warnings = append(warnings, warnf("skipping driver %q: %q reports no file version", name, path))
			continue
		}

		// Only OEM and third-party drivers are reported, and the vendor of the binary is the
		// only signal this class offers. This is a deliberate exclusion rather than an
		// unusable record, so it earns no warning: a host has hundreds of inbox drivers, and
		// a warning for each would swamp the payload.
		//
		// An absent CompanyName is kept. An unattributed kernel driver is exactly the kind of
		// thing an operator wants to see.
		if isMicrosoftPublisher(info.CompanyName) {
			continue
		}

		displayName := resolveDisplayName(driver.DisplayName, name, loadIndirect)

		productCode := strings.ToLower(name)
		if _, ok := byProductCode[productCode]; ok {
			// Service names are unique, so this only guards against a duplicated record.
			continue
		}

		byProductCode[productCode] = &Entry{
			Source:      softwareTypeDriver,
			DisplayName: displayName,
			Version:     info.FileVersionNumeric,
			Publisher:   strings.TrimSpace(info.CompanyName),
			Status:      "installed",
			ProductCode: productCode,
			// The aggregator mirrors InstallPath into the backend-facing InstallPaths, which
			// is how the driver file name reaches the payload.
			InstallPath: path,
			Is64Bit:     runtime.GOARCH != "386",
			// InstallDate is left empty on purpose: neither the service registration nor the
			// binary records when the driver was installed.
		}
	}

	return warnings, nil
}

// collectDeviceDrivers adds the OEM driver packages that register no kernel service to
// byProductCode. Those are invisible to the service source, which is the gap this closes.
//
// The identity is the device's most specific hardware ID. It survives driver updates and,
// unlike the description, distinguishes devices that share a generic name. Manufacturer and
// description are used as a fallback for the unusual device that exposes no hardware ID.
func (c *driverCollector) collectDeviceDrivers(byProductCode map[string]*Entry) ([]*Warning, error) {
	deviceQuery := c.deviceQueryFn
	if deviceQuery == nil {
		deviceQuery = winutil.EnumDeviceDrivers
	}

	devices, err := deviceQuery()
	if err != nil {
		return nil, err
	}

	var warnings []*Warning

	for _, driver := range devices {
		// A device driven by a service is already reported by collectServiceDrivers under that
		// service name. This is where the old WMI join between Win32_PnPSignedDriver and
		// Win32_PnPEntity disappears: winutil already resolved it per device.
		if strings.TrimSpace(driver.Service) != "" {
			continue
		}

		// Only OEM and third-party drivers are reported. Windows renames a package that did
		// not ship inside Windows to "oemNN.inf" when it publishes it, so the INF name is the
		// test. The manufacturer is not: Microsoft's own inbox INFs mostly report vendor-shaped
		// or generic strings such as "(Standard system devices)", and Windows also ships inbox
		// INFs authored by hardware vendors, so filtering on it would let hundreds of inbox
		// drivers through. The converse is accepted: a Microsoft package delivered out of band
		// by Windows Update is published as an OEM INF and is reported.
		//
		// Dropped silently, like the inbox drivers of the service source and for the same
		// reason: they are the majority and their exclusion is deliberate.
		if !isOEMInfName(driver.InfName) {
			continue
		}

		name := strings.TrimSpace(driver.Description)
		if name == "" {
			warnings = append(warnings, warnf("skipping device driver with no description (INF %q, device %q)",
				driver.InfName, driver.InstanceID))
			continue
		}

		// DriverVer is mandatory for a package to be signable, so this should never fire. It
		// is dropped silently for that reason, and because an entry whose version can never
		// advance carries nothing. zeroVersion deliberately does not apply: it stands for a
		// binary that shipped a version resource without setting FILEVERSION, which is not
		// something an INF text field can express.
		version := strings.TrimSpace(driver.DriverVersion)
		if version == "" {
			continue
		}

		manufacturer := strings.TrimSpace(driver.Manufacturer)
		identity := strings.TrimSpace(driver.HardwareID)
		if identity == "" {
			identity = name
			if manufacturer != "" {
				identity = manufacturer + "|" + name
			}
		}
		productCode := strings.ToLower(identity)
		if _, ok := byProductCode[productCode]; ok {
			// Two devices with the same hardware ID are the same model.
			continue
		}

		byProductCode[productCode] = &Entry{
			Source:      softwareTypeDriver,
			DisplayName: name,
			Version:     version,
			Publisher:   manufacturer,
			Status:      "installed",
			ProductCode: productCode,
			// InstallPath is left empty: this source names no binary. It is also what tells
			// the two sources apart in the integration test.
			InstallPath: "",
			Is64Bit:     runtime.GOARCH != "386",
			// InstallDate is left empty for the same reason as the service source: nothing
			// records when the driver was installed.
		}
	}

	return warnings, nil
}
