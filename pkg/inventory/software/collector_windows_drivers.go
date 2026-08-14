// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// driveLetterPath matches a path that is already rooted at a drive letter, which needs no
// further resolution.
var driveLetterPath = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// errEmptyImagePath reports a driver service that names no binary.
var errEmptyImagePath = errors.New("empty image path")

// zeroVersion is what the fixed version information reports when a driver ships a version
// resource but never set FILEVERSION. It carries no information, so it is treated as absent.
const zeroVersion = "0.0.0.0"

// win32SystemDriver is the narrow projection of the Win32_SystemDriver WMI class that the
// collector needs. The class enumerates every registered kernel-mode driver service, which
// is a superset of the drivers bound to a PnP device node: software-only drivers such as EDR
// minifilters and network filters have no device node and appear only here.
//
// The class carries no version or vendor of its own — both come from the version resource of
// the binary named by PathName.
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-systemdriver
type win32SystemDriver struct {
	// Name is the service name, e.g. "WdFilter". Windows guarantees it is unique on the
	// host, since it is the key name under HKLM\SYSTEM\CurrentControlSet\Services.
	Name string
	// DisplayName is the human-readable name, e.g. "Microsoft Defender Antivirus
	// Mini-Filter Driver". It may be empty or identical to Name.
	DisplayName string
	// PathName is the image path. It is not necessarily a Win32 path: it may be prefixed
	// with \??\ or \SystemRoot\, or be relative to the Windows directory.
	PathName string
}

// win32PnPSignedDriver is the narrow projection of the Win32_PnPSignedDriver WMI class that
// the collector needs. The class enumerates the driver bound to each PnP device node, which
// covers the drivers Win32_SystemDriver cannot see: a PnP driver package can be installed
// and bound to a device without registering a kernel service of its own.
//
// Unlike Win32_SystemDriver the class carries its own version and vendor, both taken from
// the INF, so no version resource has to be read.
//
// DriverName is deliberately absent from this projection: it is NULL on most hosts.
//
// https://learn.microsoft.com/en-us/previous-versions/windows/desktop/legacy/aa394354(v=vs.85)
type win32PnPSignedDriver struct {
	// DeviceID is the PnP instance path of the device the driver is bound to. It is what
	// joins a record to win32PnPEntity.
	DeviceID string
	// DeviceName is the device description from the INF, e.g. "NVIDIA GeForce RTX 4060".
	DeviceName string
	// DriverVersion comes from the DriverVer directive of the INF.
	DriverVersion string
	// Manufacturer is the INF's manufacturer, e.g. "NVIDIA". It is reported as the
	// publisher and is deliberately not used to filter: see collectPnPDrivers.
	Manufacturer string
	// InfName is the published name of the INF. Windows renames a package that did not
	// ship inside Windows to "oemNN.inf" when it publishes it, which is what makes this
	// the third-party signal.
	InfName string
}

// win32PnPEntity is the narrow projection of the Win32_PnPEntity WMI class that the collector
// needs. It is queried only to answer, for each device, whether a kernel service drives it.
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-pnpentity
type win32PnPEntity struct {
	// DeviceID is the PnP instance path, matching win32PnPSignedDriver.DeviceID.
	DeviceID string
	// Service is the name of the service driving the device, empty when it has none.
	Service string
}

// driverCollector collects installed kernel-mode drivers via WMI.
//
// Only OEM and third-party drivers are reported. Microsoft's inbox drivers number in the
// hundreds on any host and say nothing about it, so they would swamp the drivers that do.
// The two sources need different tests for this, because they expose different fields:
// Win32_SystemDriver is filtered on the vendor of the binary, Win32_PnPSignedDriver on
// whether the INF was published as an OEM package.
//
// Known limitations, both accepted:
//   - user-mode (UMDF) drivers register no kernel service and are not reported;
//   - a driver whose service entry is created, loaded and then deleted is invisible, so this
//     is an inventory of registered drivers rather than of code currently in the kernel.
type driverCollector struct {
	// queryFn returns the raw driver records. It is a field so tests can supply fixtures
	// instead of reaching WMI; nil means use the real query.
	queryFn func() ([]win32SystemDriver, error)
	// pnpDriverQueryFn returns the raw PnP driver records. It is a field so tests can supply
	// fixtures instead of reaching WMI; nil means use the real query.
	pnpDriverQueryFn func() ([]win32PnPSignedDriver, error)
	// pnpEntityQueryFn returns the raw PnP device records. It is a field so tests can supply
	// fixtures instead of reaching WMI; nil means use the real query.
	pnpEntityQueryFn func() ([]win32PnPEntity, error)
	// versionInfoFn reads the version resource of a driver binary. It is a field so tests
	// stay off the filesystem; nil means read the real file.
	versionInfoFn func(path string) (winutil.FileVersionInfo, error)
	// loadIndirectFn resolves an indirect string reference in a display name. It is a field
	// so tests stay off the filesystem; nil means resolve against the real module.
	loadIndirectFn func(source string) (string, error)
}

// queryWMI runs a query against the local machine and decodes the rows into dst, which must
// be a pointer to a slice of the projection to decode into.
func queryWMI(query string, dst any) error {
	wmiClient := &wmi.Client{}
	swbemServices, err := wmi.InitializeSWbemServices(wmiClient)
	if err != nil {
		return fmt.Errorf("failed to initialize WMI services: %w", err)
	}
	defer func() {
		if closeErr := swbemServices.Close(); closeErr != nil {
			log.Errorf("error closing SWbemServicesClient: %v", closeErr)
		}
	}()
	wmiClient.SWbemServicesClient = swbemServices

	if err := wmiClient.SWbemServicesClient.Query(query, dst); err != nil {
		return fmt.Errorf("failed to run %q: %w", query, err)
	}

	return nil
}

// querySystemDrivers runs the WMI query against the local machine.
func querySystemDrivers() ([]win32SystemDriver, error) {
	var drivers []win32SystemDriver
	err := queryWMI("SELECT Name, DisplayName, PathName FROM Win32_SystemDriver", &drivers)
	return drivers, err
}

// queryPnPSignedDrivers runs the WMI query against the local machine.
func queryPnPSignedDrivers() ([]win32PnPSignedDriver, error) {
	var drivers []win32PnPSignedDriver
	err := queryWMI(
		"SELECT DeviceID, DeviceName, DriverVersion, Manufacturer, InfName FROM Win32_PnPSignedDriver",
		&drivers,
	)
	return drivers, err
}

// queryPnPEntities runs the WMI query against the local machine.
//
// Every device is fetched in one query and joined in memory rather than looked up per device
// id: both PnP classes are slow to enumerate, and one round trip per device would exhaust
// the collector's whole time budget on the join.
func queryPnPEntities() ([]win32PnPEntity, error) {
	var entities []win32PnPEntity
	err := queryWMI("SELECT DeviceID, Service FROM Win32_PnPEntity", &entities)
	return entities, err
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

	// "\??\C:\Program Files\Vendor\driver.sys" is the NT object-manager form of a Win32 path.
	path = strings.TrimPrefix(path, `\??\`)

	// A few drivers store their path with variables, e.g. "%SystemRoot%\System32\drivers".
	if strings.Contains(path, "%") {
		path = expandWinEnv(path)
	}

	// Already rooted: a drive letter, or a UNC share.
	if driveLetterPath.MatchString(path) || strings.HasPrefix(path, `\\`) {
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

// winEnvVar matches a Windows-style environment variable reference, e.g. "%SystemRoot%".
var winEnvVar = regexp.MustCompile(`%([^%\\/]+)%`)

// expandWinEnv expands %VAR% references. References that do not resolve are left in place,
// so the caller sees the original text rather than a silently truncated path.
func expandWinEnv(path string) string {
	return winEnvVar.ReplaceAllStringFunc(path, func(match string) string {
		if value := os.Getenv(strings.Trim(match, "%")); value != "" {
			return value
		}
		return match
	})
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
// This filters the Win32_SystemDriver source only. Win32_PnPSignedDriver carries an INF name,
// which is a far better signal; see collectPnPDrivers.
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

	pnpWarnings, err := c.collectPnPDrivers(byProductCode)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, pnpWarnings...)

	entries := make([]*Entry, 0, len(byProductCode))
	for _, entry := range byProductCode {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings, nil
}

// collectServiceDrivers adds the drivers registered as kernel services to byProductCode.
//
// The identity is the service name. It is both the identity and the grouping key: Windows keys
// the Services registry on it, so it is unique on the host, and unlike the image path or the
// display name it does not change when the driver is updated. Keeping the version out of the
// identity is what lets a version bump read as an update rather than as an uninstall followed
// by a fresh install.
func (c *driverCollector) collectServiceDrivers(byProductCode map[string]*Entry) ([]*Warning, error) {
	query := c.queryFn
	if query == nil {
		query = querySystemDrivers
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
			warnings = append(warnings, warnf("skipping driver with no service name (path %q)", driver.PathName))
			continue
		}

		path, err := resolveDriverPath(driver.PathName, windowsDir)
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

// collectPnPDrivers adds the OEM driver packages that register no kernel service to
// byProductCode. Those are invisible to the service source, which is the gap this closes.
//
// The identity is the device name. The service name is not available here — its absence is
// what selects these records in the first place — and of what the class does offer it is the
// field that survives a driver update, which is the property the identity needs. Its one
// failure mode is a vendor rewording the device description between package versions, which
// costs one spurious uninstall and reinstall and then settles.
func (c *driverCollector) collectPnPDrivers(byProductCode map[string]*Entry) ([]*Warning, error) {
	driverQuery := c.pnpDriverQueryFn
	if driverQuery == nil {
		driverQuery = queryPnPSignedDrivers
	}
	entityQuery := c.pnpEntityQueryFn
	if entityQuery == nil {
		entityQuery = queryPnPEntities
	}

	drivers, err := driverQuery()
	if err != nil {
		return nil, err
	}
	entities, err := entityQuery()
	if err != nil {
		return nil, err
	}

	// Instance paths are conventionally upper case, but nothing guarantees the two classes
	// spell them the same way, so both sides of the join are normalised.
	serviceByDeviceID := make(map[string]string, len(entities))
	for _, entity := range entities {
		serviceByDeviceID[strings.ToUpper(strings.TrimSpace(entity.DeviceID))] = strings.TrimSpace(entity.Service)
	}

	var warnings []*Warning

	for _, driver := range drivers {
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

		// A device driven by a service is already reported by collectServiceDrivers under that
		// service name. A device id absent from the map is treated as having no service, which
		// is the permissive choice, consistent with keeping a driver whose vendor is unknown.
		if service := serviceByDeviceID[strings.ToUpper(strings.TrimSpace(driver.DeviceID))]; service != "" {
			continue
		}

		name := strings.TrimSpace(driver.DeviceName)
		if name == "" {
			warnings = append(warnings, warnf("skipping PnP driver with no device name (INF %q, device %q)",
				driver.InfName, driver.DeviceID))
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

		productCode := strings.ToLower(name)
		if _, ok := byProductCode[productCode]; ok {
			// Two devices of the same model share a device name, and are one driver package.
			continue
		}

		byProductCode[productCode] = &Entry{
			Source:      softwareTypeDriver,
			DisplayName: name,
			Version:     version,
			Publisher:   strings.TrimSpace(driver.Manufacturer),
			Status:      "installed",
			ProductCode: productCode,
			// InstallPath is left empty: this class names no binary. It is also what tells the
			// two sources apart in the integration test.
			InstallPath: "",
			Is64Bit:     runtime.GOARCH != "386",
			// InstallDate is left empty for the same reason as the service source: nothing
			// records when the driver was installed.
		}
	}

	return warnings, nil
}
