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
)

var requiredWindowsEnvironmentVariables = []string{
	"COMSPEC",
	"PATHEXT",
	"SYSTEMROOT",
	"WINDIR",
}

func platformEnvironment(session *Session, executableDirectories []string) (map[string]string, error) {
	executablePath, err := buildExecutablePath(executableDirectories)
	if err != nil {
		return nil, err
	}

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
	// PowerShell and Windows require these values from the PAR service environment.
	// No other service environment variables are inherited unless the manifest allows them.
	for _, name := range requiredWindowsEnvironmentVariables {
		value, err := requiredWindowsEnvironmentVariable(name)
		if err != nil {
			return nil, err
		}
		setEnvironmentEntry(envVars, name, value)
	}
	return envVars, nil
}

func platformDefaultExecutablePath() (string, error) {
	systemRoot, err := requiredWindowsEnvironmentVariable("SYSTEMROOT")
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(systemRoot, "System32"),
		systemRoot,
	}, string(os.PathListSeparator)), nil
}

func requiredWindowsEnvironmentVariable(name string) (string, error) {
	value, found := os.LookupEnv(name)
	if !found || value == "" {
		return "", fmt.Errorf("required Windows environment variable %q is not set", name)
	}
	return value, nil
}

func normalizeEnvironmentVariableName(name string) string {
	return strings.ToUpper(name)
}
