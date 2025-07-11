// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/cenkalti/backoff/v4"
)

const (
	// WERLocalDumpsRegistryKey is the registry key for Windows Error Reporting (WER) user-mode dumps
	WERLocalDumpsRegistryKey = `HKLM:SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps`
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

// GetWERGlobalDumpFolder returns the folder where Windows Error Reporting (WER) dumps are stored
// as configured in the registry.
func GetWERGlobalDumpFolder(host *components.RemoteHost) (string, error) {
	val, err := GetRegistryValue(host, WERLocalDumpsRegistryKey, "DumpFolder")
	if err != nil {
		return "", fmt.Errorf("error getting WER dump folder: %w", err)
	}
	return val, nil
}

// parseWERDumpFilePath parses a WER dump file name of the form <image name>.<pid>.dmp and returns
// a WERDumpFile struct
func parseWERDumpFilePath(path string) WERDumpFile {
	// Example: crash.exe.1234.dmp
	filename := FileNameFromPath(path)
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

// ListWERDumps lists WER dumps in a folder on a remote host
func ListWERDumps(host *components.RemoteHost, dumpFolder string) ([]WERDumpFile, error) {
	entries, err := host.ReadDir(dumpFolder)
	if err != nil {
		return nil, fmt.Errorf("error reading WER dump dir %s: %w", dumpFolder, err)
	}

	var dumps []WERDumpFile
	for _, entry := range entries {
		if entry.IsDir() {
			// skip directories, WER doesn't create directories in the dump folder
			continue
		}
		dump := parseWERDumpFilePath(filepath.Join(dumpFolder, entry.Name()))
		dumps = append(dumps, dump)
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

// DownloadAllWERDumps collects WER dumps from a folder on a remote host and saves them to a local folder
//
// See DownloadWERDump for the naming convention used for the output files.
//
// This function continues collecting dumps even if some of them fail to be collected, and returns
// an error with all the errors encountered.
func DownloadAllWERDumps(host *components.RemoteHost, dumpFolder string, outputPath string) ([]string, error) {
	return DownloadAllWERDumpsFunc(host, dumpFolder, outputPath,
		// collect all dumps
		func(_ WERDumpFile) bool { return true },
	)
}

// DownloadAllWERDumpsFunc is like DownloadAllWERDumps, but allows to filter the dumps to collect
func DownloadAllWERDumpsFunc(host *components.RemoteHost, dumpFolder string, outputPath string, f func(WERDumpFile) bool) ([]string, error) {
	dumps, err := ListWERDumps(host, dumpFolder)
	if err != nil {
		return nil, err
	}

	collectedDumps := []string{}
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
		collectedDumps = append(collectedDumps, outPath)
	}

	return collectedDumps, retErr
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
	var driverList string

	for _, driverName := range kernelDrivers {
		if !strings.HasSuffix(driverName, ".sys") {
			driverList += fmt.Sprintf("%s.sys ", driverName)
		} else {
			driverList += fmt.Sprintf("%s ", driverName)
		}
	}

	fmt.Println("Enabling driver verifier for: ", driverList)

	// Driver verifier returns an error code of 2.
	out, err := host.Execute(fmt.Sprintf("verifier /standard /driver %s", driverList))
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

	return backoff.Retry(func() error {
		err := host.Reconnect()
		if err != nil {
			return err
		}
		out, err = host.Execute("(Get-CimInstance Win32_OperatingSystem).LastBootUpTime")
		if err != nil {
			return err
		}
		bootTime := strings.TrimSpace(out)
		fmt.Println("current boot time:", bootTime)
		if bootTime == lastBootTime {
			return fmt.Errorf("boot time has not changed")
		}
		return nil
	}, b)
}
