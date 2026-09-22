// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package winutil

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var windowsVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

func TestGetWindowsVersionComponents(t *testing.T) {
	version, err := GetWindowsVersionComponents()
	assert.NoError(t, err)
	assert.NotEmpty(t, version.Major)
	assert.NotEmpty(t, version.Minor)
	assert.NotEmpty(t, version.Build)
	assert.NotEmpty(t, version.Revision)
}

func TestWindowsVersionFormattersUseSharedComponents(t *testing.T) {
	components, err := GetWindowsVersionComponents()
	assert.NoError(t, err)

	version, err := GetWindowsVersion()
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s.%s.%s.%s", components.Major, components.Minor, components.Build, components.Revision), version)

	buildString, err := GetWindowsBuildString()
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s.%s Build %s", components.Major, components.Minor, components.Build), buildString)
}

func TestGetWindowsVersionMatchesCommandVersion(t *testing.T) {
	output, err := exec.Command("cmd", "/c", "ver").Output()
	require.NoError(t, err)

	commandVersion := windowsVersionPattern.FindString(string(output))
	require.NotEmpty(t, commandVersion, "cmd /c ver output: %q", output)

	version, err := GetWindowsVersion()
	require.NoError(t, err)
	assert.Equal(t, commandVersion, version)
}
