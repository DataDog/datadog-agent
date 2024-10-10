// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// GetFileVersion gets the file version information from the versioninfo resource
// of the specified file.  It returns as a slice of strings, each element being part
// of the version (major, minor, patch, build)
//
// // for the remote file, it uses the powershell version information command
func GetFileVersion(host *components.RemoteHost, remoteFileName string) ([]string, error) {
	pscommand := fmt.Sprintf(`(Get-Item "%s").VersionInfo.FileVersion`, remoteFileName)
	remoteversion, err := host.Execute(pscommand)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	return strings.Split(strings.TrimSpace(remoteversion), "."), nil
}

// GetExpectedVersion gets the expected version from the most recent tag
// returns in the form of slice of strings, each element being a part of the version
// (major, minor, patch)
//
// expectedVersion can be empty; if so, then it uses `git` to determine the
// expected version from the most recent tag.  It also then assumes that
// git is in the path
func GetExpectedVersion(expectedVersion string) ([]string, error) {
	if expectedVersion == "" {
		// this needs to be executed locally
		output, err := exec.Command("git", "describe", "--tags", "--candidates=50", "--match", "[0-9]*", "--abbrev=7").Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get git version: %w", err)
		}
		// we only want the M.m.p version number, so up to the first `-` if it exists
		expectedVersion = strings.TrimSpace(string(output))
	}
	dashindex := strings.Index(expectedVersion, "-")
	if dashindex > 1 {
		expectedVersion = expectedVersion[:dashindex]
	}

	expectedparts := strings.Split(expectedVersion, ".")
	fmt.Printf("Looking for version %v\n", expectedparts)
	if len(expectedparts) != 3 {
		return nil, fmt.Errorf("expected version must be in the form M.m.p.")
	}
	return expectedparts, nil
}

// VerifyVersion takes a filename and an expected version and determines
// if the file matches the expected version.

func VerifyVersion(host *components.RemoteHost, remoteFileName, expectedVersion string) error {

	expectedparts, err := GetExpectedVersion(expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to get expected version: %w", err)
	}

	remoteversionparts, err := GetFileVersion(host, remoteFileName)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}
	fmt.Printf("Found version %v\n", remoteversionparts)

	for idx, v := range expectedparts {
		if v != remoteversionparts[idx] {
			return fmt.Errorf("expected version %v, got %v", expectedVersion, remoteversionparts)
		}
	}
	return nil
}
