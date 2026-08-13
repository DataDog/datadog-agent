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

// driverCollector collects installed kernel-mode drivers via WMI.
//
// Known limitations, both accepted:
//   - user-mode (UMDF) drivers register no kernel service and are not reported;
//   - a driver whose service entry is created, loaded and then deleted is invisible, so this
//     is an inventory of registered drivers rather than of code currently in the kernel.
type driverCollector struct {
	// queryFn returns the raw driver records. It is a field so tests can supply fixtures
	// instead of reaching WMI; nil means use the real query.
	queryFn func() ([]win32SystemDriver, error)
	// versionInfoFn reads the version resource of a driver binary. It is a field so tests
	// stay off the filesystem; nil means read the real file.
	versionInfoFn func(path string) (winutil.FileVersionInfo, error)
	// loadIndirectFn resolves an indirect string reference in a display name. It is a field
	// so tests stay off the filesystem; nil means resolve against the real module.
	loadIndirectFn func(source string) (string, error)
}

// querySystemDrivers runs the WMI query against the local machine.
func querySystemDrivers() ([]win32SystemDriver, error) {
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

	var drivers []win32SystemDriver
	if err := wmiClient.SWbemServicesClient.Query(
		"SELECT Name, DisplayName, PathName FROM Win32_SystemDriver",
		&drivers,
	); err != nil {
		return nil, fmt.Errorf("failed to query Win32_SystemDriver: %w", err)
	}

	return drivers, nil
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

// Collect returns one entry per registered kernel-mode driver.
// A failure to enumerate is fatal; individual unusable records are reported as warnings.
func (c *driverCollector) Collect() ([]*Entry, []*Warning, error) {
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
		return nil, nil, err
	}

	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to locate the Windows directory: %w", err)
	}

	var warnings []*Warning
	// Group by service name. It is both the identity and the grouping key: Windows keys the
	// Services registry on it, so it is unique on the host, and unlike the image path or the
	// display name it does not change when the driver is updated. Keeping the version out of
	// the identity is what lets a version bump read as an update rather than as an uninstall
	// followed by a fresh install.
	byProductCode := make(map[string]*Entry, len(drivers))

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

	entries := make([]*Entry, 0, len(byProductCode))
	for _, entry := range byProductCode {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ProductCode < entries[j].ProductCode })

	return entries, warnings, nil
}
