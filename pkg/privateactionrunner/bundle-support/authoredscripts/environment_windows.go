// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultWindowsPathExt = ".COM;.EXE;.BAT;.CMD"

func platformEnvironment(session *Session, executablePath string) (map[string]string, error) {
	appDataDirectory := filepath.Join(session.HomeDirectory, "AppData", "Roaming")
	localAppDataDirectory := filepath.Join(session.HomeDirectory, "AppData", "Local")
	for _, directory := range []string{appDataDirectory, localAppDataDirectory} {
		if err := os.MkdirAll(directory, sessionDirectoryMode); err != nil {
			return nil, fmt.Errorf("could not create authored-script session directory %q: %w", directory, err)
		}
	}

	envVars := map[string]string{
		"APPDATA":      appDataDirectory,
		"HOME":         session.HomeDirectory,
		"LOCALAPPDATA": localAppDataDirectory,
		"PATH":         executablePath,
		"TEMP":         session.TempDirectory,
		"TMP":          session.TempDirectory,
		"TMPDIR":       session.TempDirectory,
		"USERPROFILE":  session.HomeDirectory,
	}

	systemRoot, err := windowsSystemRoot()
	if err != nil {
		return nil, err
	}
	// These values normally come from the PAR service environment. Use Windows
	// defaults when the service environment is more restricted, as it is in some
	// test and service launchers.
	setEnvironmentEntry(envVars, "COMSPEC", windowsEnvironmentVariable("COMSPEC", filepath.Join(systemRoot, "System32", "cmd.exe")))
	setEnvironmentEntry(envVars, "PATHEXT", windowsEnvironmentVariable("PATHEXT", defaultWindowsPathExt))
	setEnvironmentEntry(envVars, "SYSTEMROOT", windowsEnvironmentVariable("SYSTEMROOT", systemRoot))
	setEnvironmentEntry(envVars, "WINDIR", windowsEnvironmentVariable("WINDIR", systemRoot))
	return envVars, nil
}

func platformDefaultExecutablePath() (string, error) {
	systemRoot, err := windowsSystemRoot()
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(systemRoot, "System32"),
		systemRoot,
	}, string(os.PathListSeparator)), nil
}

func windowsSystemRoot() (string, error) {
	if value := os.Getenv("SYSTEMROOT"); value != "" {
		return value, nil
	}
	value, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return "", fmt.Errorf("could not locate the Windows system directory: %w", err)
	}
	return value, nil
}

func windowsEnvironmentVariable(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	log.Warnf("Windows environment variable %q is not set; using %q", name, fallback)
	return fallback
}

func normalizeEnvironmentVariableName(name string) string {
	return strings.ToUpper(name)
}
