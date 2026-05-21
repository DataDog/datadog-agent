// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// silentExitDumpFileRE matches dump filenames written by Windows SilentProcessExit
// monitoring, of the form "<image>-(PID-<pid>).dmp" (e.g. "datadog-installer.exe-(PID-6932).dmp").
// These live one directory level under LocalDumpFolder in a per-event subdirectory
// named "<image>-(PID-<pid>)-<seq>".
var silentExitDumpFileRE = regexp.MustCompile(`^(.+)-\(PID-(\d+)\)\.dmp$`)

const (
	// WERLocalDumpsRegistryKey is the registry key for Windows Error Reporting (WER) user-mode dumps
	WERLocalDumpsRegistryKey = `HKLM:SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps`
	// imageFileExecutionOptionsKey is the per-image IFEO key under which the
	// FLG_MONITOR_SILENT_PROCESS_EXIT GlobalFlag bit (0x200) is set to arm
	// silent-exit monitoring.
	imageFileExecutionOptionsKey = `HKLM:SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
	// silentProcessExitKey is the per-image key that holds the silent-exit
	// dump configuration (LocalDumpFolder, ReportingMode, DumpType, IgnoreSelfExits).
	silentProcessExitKey = `HKLM:SOFTWARE\Microsoft\Windows NT\CurrentVersion\SilentProcessExit`
)

// WERDumpFile represents a Windows Error Reporting (WER) dump file
type WERDumpFile struct {
	// Path to the dump file
	Path string
	// FileName part of Path
	FileName string
	// Process ID, extracted from FileName
	PID string
	// Image name, extracted from FileName
	ImageName string
}

// EnableWERGlobalDumps enables Windows Error Reporting (WER) dumps for all applications
//
// This function creates a folder to store the dumps and sets the registry keys to enable WER dumps.
// ACLs are set to allow everyone to write to the folder.
//
// https://learn.microsoft.com/en-us/windows/win32/wer/collecting-user-mode-dumps
func EnableWERGlobalDumps(host *components.RemoteHost, dumpFolder string) error {

	cmd := fmt.Sprintf(`
		mkdir '%s' -Force
		icacls.exe '%s' /grant 'Everyone:(OI)(CI)F'
		New-Item -Path '%s' -Force
		Set-ItemProperty -Path '%s' -Name "DumpFolder" -Value '%s' -Type ExpandString -Force
		Set-ItemProperty -Path '%s' -Name "DumpCount" -Value 10 -Type DWORD -Force
		Set-ItemProperty -Path '%s' -Name "DumpType" -Value 2 -Type DWORD -Force
	`,
		dumpFolder,
		dumpFolder,
		WERLocalDumpsRegistryKey,
		WERLocalDumpsRegistryKey,
		dumpFolder,
		WERLocalDumpsRegistryKey,
		WERLocalDumpsRegistryKey)
	_, err := host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("error enabling WER dumps: %w", err)
	}

	return nil
}

// EnableSilentProcessExitDump configures WerFault to capture a user-mode dump of the
// named image when it is terminated by another process (e.g. SCM killing a service
// that did not respond to start/stop within its timeout). The dump is written to
// dumpFolder using the same naming convention as WER LocalDumps (<image>.<pid>.dmp),
// so existing collection/parsing code in this package can pick it up.
//
// IgnoreSelfExits is set so normal process exits (ExitProcess from inside the
// process) do not produce dumps — only externally-initiated terminations.
//
// imageName must be the basename (e.g. "datadog-installer.exe"); matching is by
// basename, not full path.
//
// https://learn.microsoft.com/en-us/windows-hardware/drivers/debugger/registry-entries-for-silent-process-exit
func EnableSilentProcessExitDump(host *components.RemoteHost, imageName string, dumpFolder string) error {
	imageOptions := fmt.Sprintf(`%s\%s`, imageFileExecutionOptionsKey, imageName)
	silentExit := fmt.Sprintf(`%s\%s`, silentProcessExitKey, imageName)

	// 0x200 == FLG_MONITOR_SILENT_PROCESS_EXIT. ReportingMode 2 == LocalDump.
	// DumpType 2 == MiniDumpWithFullMemory (heap included; needed for Go runtime
	// state when diagnosing early-startup hangs).
	cmd := fmt.Sprintf(`
		New-Item -Path '%s' -Force | Out-Null
		Set-ItemProperty -Path '%s' -Name "GlobalFlag" -Value 0x200 -Type DWORD -Force
		New-Item -Path '%s' -Force | Out-Null
		Set-ItemProperty -Path '%s' -Name "ReportingMode" -Value 2 -Type DWORD -Force
		Set-ItemProperty -Path '%s' -Name "LocalDumpFolder" -Value '%s' -Type ExpandString -Force
		Set-ItemProperty -Path '%s' -Name "DumpType" -Value 2 -Type DWORD -Force
		Set-ItemProperty -Path '%s' -Name "IgnoreSelfExits" -Value 1 -Type DWORD -Force
	`,
		imageOptions,
		imageOptions,
		silentExit,
		silentExit,
		silentExit, dumpFolder,
		silentExit,
		silentExit)
	if _, err := host.Execute(cmd); err != nil {
		return fmt.Errorf("error enabling silent process exit dump for %s: %w", imageName, err)
	}
	return nil
}

// GetWERGlobalDumpFolder returns the folder where Windows Error Reporting (WER) dumps are stored
// as configured in the registry.
func GetWERGlobalDumpFolder(host *components.RemoteHost) (string, error) {
	val, err := GetRegistryValue(host, WERLocalDumpsRegistryKey, "DumpFolder")
	if err != nil {
		return "", fmt.Errorf("error getting WER dump folder: %w", err)
	}
	return val, nil
}

// parseWERDumpFilePath parses a dump file name and returns a WERDumpFile struct.
// Two formats are recognized:
//   - WER LocalDumps:        <image>.<pid>.dmp                 (e.g. agent.exe.1234.dmp)
//   - SilentProcessExit:     <image>-(PID-<pid>).dmp           (e.g. datadog-installer.exe-(PID-6932).dmp)
//
// In both cases the returned ImageName omits the ".exe" suffix to match the
// behavior expected by normalizeImageName / IsIgnoredCrashDump.
func parseWERDumpFilePath(path string) WERDumpFile {
	filename := FileNameFromPath(path)

	if m := silentExitDumpFileRE.FindStringSubmatch(filename); m != nil {
		// SilentProcessExit format: "<image>.exe-(PID-<pid>).dmp"
		imageName := strings.TrimSuffix(m[1], ".exe")
		return WERDumpFile{
			Path:      path,
			FileName:  filename,
			PID:       m[2],
			ImageName: imageName,
		}
	}

	// WER LocalDumps format: "<image>.<pid>.dmp" (e.g. crash.exe.1234.dmp)
	parts := strings.Split(filename, ".")
	i := len(parts) - 1
	// file extension
	_ = parts[i]
	i--
	// pid
	pid := parts[i]
	i--
	// image name
	imageName := strings.Join(parts[:i], ".")
	return WERDumpFile{
		Path:      path,
		FileName:  filename,
		PID:       pid,
		ImageName: imageName,
	}
}

// ListWERDumps lists dump files in a folder on a remote host. It picks up both
// WER LocalDumps written flat in the folder and SilentProcessExit dumps written
// one directory level deeper in per-event subdirectories.
func ListWERDumps(host *components.RemoteHost, dumpFolder string) ([]WERDumpFile, error) {
	entries, err := host.ReadDir(dumpFolder)
	if err != nil {
		return nil, fmt.Errorf("error reading WER dump dir %s: %w", dumpFolder, err)
	}

	var dumps []WERDumpFile
	for _, entry := range entries {
		entryPath := filepath.Join(dumpFolder, entry.Name())
		if entry.IsDir() {
			// SilentProcessExit writes "<image>-(PID-<pid>)-<seq>/<image>-(PID-<pid>).dmp"
			subEntries, err := host.ReadDir(entryPath)
			if err != nil {
				return nil, fmt.Errorf("error reading dump subdir %s: %w", entryPath, err)
			}
			for _, sub := range subEntries {
				if sub.IsDir() || !strings.HasSuffix(strings.ToLower(sub.Name()), ".dmp") {
					continue
				}
				dumps = append(dumps, parseWERDumpFilePath(filepath.Join(entryPath, sub.Name())))
			}
			continue
		}
		dumps = append(dumps, parseWERDumpFilePath(entryPath))
	}

	return dumps, nil
}

// DownloadWERDump downloads a WER dump from a remote host and saves it to a local folder
// with the format <host address>-<dump file name>
func DownloadWERDump(host *components.RemoteHost, dump WERDumpFile, outputDir string) (string, error) {
	outName := fmt.Sprintf("%s-%s", host.Address, dump.FileName)
	outPath := filepath.Join(outputDir, outName)
	err := host.GetFile(dump.Path, outPath)
	if err != nil {
		return "", fmt.Errorf("error getting WER dump file %s: %w", dump.Path, err)
	}
	return outPath, nil
}

// DownloadedWERDump pairs the source metadata for a WER dump with the local
// artifact path produced by downloading it. Callers get both the image name
// (for filtering against an ignore list) and the local path (for logs /
// artifact pointers) in one record.
//
// Source.Path is the *remote* path on the test VM (e.g. C:\dumps\agent.exe.1234.dmp);
// LocalPath is the path on the test runner (e.g. e2e-output/<host>-agent.exe.1234.dmp).
type DownloadedWERDump struct {
	Source    WERDumpFile
	LocalPath string
}

// DefaultIgnoredCrashDumpImages is the denylist of process image names whose
// WER crash dumps are recorded as artifacts but do NOT fail tests that use
// PartitionDownloadedWERDumps. These are third-party processes whose crashes
// have routinely been observed to be unrelated to Datadog agent behavior.
//
// Add entries here when a new noisy image is observed in CI. Comparison is
// case-insensitive and tolerates names with or without a ".exe" suffix.
var DefaultIgnoredCrashDumpImages = []string{
	"svchost.exe",
	"WmiPrvSE.exe",
	"powershell.exe",
}

// IsIgnoredCrashDump reports whether the dump's image name appears in ignore
// (case-insensitive, tolerates names with or without a ".exe" suffix).
func IsIgnoredCrashDump(dump DownloadedWERDump, ignore []string) bool {
	name := normalizeImageName(dump.Source.ImageName)
	for _, ign := range ignore {
		if normalizeImageName(ign) == name {
			return true
		}
	}
	return false
}

// PartitionDownloadedWERDumps splits dumps into (failing, ignored) using the
// given ignore list. Use it to keep test assertions focused on dumps that
// represent real agent regressions while still preserving the others as
// downloaded artifacts.
func PartitionDownloadedWERDumps(dumps []DownloadedWERDump, ignore []string) (failing, ignored []DownloadedWERDump) {
	for _, d := range dumps {
		if IsIgnoredCrashDump(d, ignore) {
			ignored = append(ignored, d)
		} else {
			failing = append(failing, d)
		}
	}
	return failing, ignored
}

func normalizeImageName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(n, ".exe") {
		n += ".exe"
	}
	return n
}

// DownloadAllWERDumps collects WER dumps from a folder on a remote host and saves them to a local folder.
//
// See DownloadWERDump for the naming convention used for the output files.
//
// This function continues collecting dumps even if some of them fail to be collected, and returns
// an error with all the errors encountered.
func DownloadAllWERDumps(host *components.RemoteHost, dumpFolder string, outputPath string) ([]DownloadedWERDump, error) {
	return DownloadAllWERDumpsFunc(host, dumpFolder, outputPath,
		// collect all dumps
		func(_ WERDumpFile) bool { return true },
	)
}

// DownloadAllWERDumpsFunc is like DownloadAllWERDumps, but allows to filter the dumps to collect.
func DownloadAllWERDumpsFunc(host *components.RemoteHost, dumpFolder string, outputPath string, f func(WERDumpFile) bool) ([]DownloadedWERDump, error) {
	dumps, err := ListWERDumps(host, dumpFolder)
	if err != nil {
		return nil, err
	}

	collected := []DownloadedWERDump{}
	var retErr error
	for _, dump := range dumps {
		if !f(dump) {
			continue
		}
		outPath, err := DownloadWERDump(host, dump, outputPath)
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("error getting WER dump file %s: %w", dump.Path, err))
			continue
		}
		collected = append(collected, DownloadedWERDump{Source: dump, LocalPath: outPath})
	}

	return collected, retErr
}

// DownloadSystemCrashDump downloads a system crash dump from a remote host.
func DownloadSystemCrashDump(host *components.RemoteHost, systemCrashDumpFile string, outputFile string) (string, error) {
	if exists, _ := host.FileExists(systemCrashDumpFile); !exists {
		return "", nil
	}

	// We cannot directly download the system crash dump file since it is under a protected directory.
	// The dump needs to be copied to a temporary location first.
	// Go's os.MkdirTemp is not suitable because it does not yield
	// a proper local path for Powershell (e.g. /tmp/2173892461/SystemCrash.DMP)

	tmpDir, err := host.GetTmpFolder()
	if err != nil {
		return "", fmt.Errorf("failed to get TMP folder: %w", err)
	}

	outName := filepath.Base(outputFile)
	tmpPath := host.JoinPath(tmpDir, outName)

	_, err = host.Execute(fmt.Sprintf("Copy-Item -Path \"%s\" -Destination \"%s\"", systemCrashDumpFile, tmpPath))
	if err != nil {
		return "", fmt.Errorf("error copying system crash dump file %s to %s: %w", systemCrashDumpFile, tmpPath, err)
	}

	// The framework may timeout trying to download the dump.
	err = host.GetFile(tmpPath, outputFile)

	if err != nil {
		return "", fmt.Errorf("error getting system crash dump file %s: %w", tmpPath, err)
	}

	return outputFile, nil
}

// EnableDriverVerifier enables standard verifier checks on the specified kernel drivers. Requires a reboot.
func EnableDriverVerifier(host *components.RemoteHost, kernelDrivers []string) (string, error) {
	var driverListBuilder strings.Builder

	for _, driverName := range kernelDrivers {
		if !strings.HasSuffix(driverName, ".sys") {
			driverListBuilder.WriteString(driverName + ".sys ")
		} else {
			driverListBuilder.WriteString(driverName + " ")
		}
	}
	driverList := driverListBuilder.String()

	fmt.Println("Enabling driver verifier for: ", driverList)

	// Driver verifier returns an error code of 2.
	out, err := host.Execute("verifier /standard /driver " + driverList)
	out = strings.TrimSpace(out)

	return out, err
}

// RebootAndWait reboots the host and waits for it to boot
func RebootAndWait(host *components.RemoteHost, b backoff.BackOff) error {
	return waitForRebootFunc(host, b, func() error {
		_, err := host.Execute("Restart-Computer -Force")
		return err
	})
}

// WaitForRebootFunc waits for the host to reboot
func waitForRebootFunc(host *components.RemoteHost, b backoff.BackOff, rebootFunc func() error) error {
	// get last boot time
	out, err := host.Execute("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
	if err != nil {
		return fmt.Errorf("failed to get last boot time: %w", err)
	}
	lastBootTime := strings.TrimSpace(out)
	fmt.Println("last boot time:", lastBootTime)

	// run the reboot function
	err = rebootFunc()
	if err != nil {
		return fmt.Errorf("failed to reboot: %w", err)
	}

	_, err = backoff.Retry(context.Background(), func() (any, error) {
		err := host.Reconnect()
		if err != nil {
			return nil, err
		}
		out, err = host.Execute("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
		if err != nil {
			return nil, err
		}
		bootTime := strings.TrimSpace(out)
		fmt.Println("current boot time:", bootTime)
		if bootTime == lastBootTime {
			return nil, errors.New("boot time has not changed")
		}
		return nil, nil
	}, backoff.WithBackOff(b))
	return err
}
