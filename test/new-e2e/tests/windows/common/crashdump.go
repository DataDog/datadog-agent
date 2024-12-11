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
