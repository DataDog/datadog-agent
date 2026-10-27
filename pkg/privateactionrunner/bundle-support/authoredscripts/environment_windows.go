// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import "errors"

func platformEnvironment(_ *Session, _ string) (map[string]string, error) {
	return nil, errors.New("authored-script environments are not supported on Windows")
}

func platformDefaultExecutablePath() (string, error) {
	return "", errors.New("authored-script executable paths are not supported on Windows")
}

func normalizeEnvironmentVariableName(name string) string {
	return name
}
