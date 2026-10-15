// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

const defaultExecutablePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

func platformEnvironment(session *Session, executableDirectories []string) (map[string]string, error) {
	executablePath, err := buildExecutablePath(executableDirectories)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"HOME":   session.HomeDirectory,
		"PATH":   executablePath,
		"TMPDIR": session.TempDirectory,
	}, nil
}

func platformDefaultExecutablePath() (string, error) {
	return defaultExecutablePath, nil
}

func normalizeEnvironmentVariableName(name string) string {
	return name
}
