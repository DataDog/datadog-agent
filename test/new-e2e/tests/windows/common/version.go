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
// VerifyVersion takes a filename and an expected version and determines
// if the file matches the expected version.
//
// expectedVersion can be empty; if so, then it uses `git` to determine the
// expected version from the most recent tag.  It also then assumes that
// git is in the path
//
// for the remote file, it uses the powershell version information command
func VerifyVersion(host *components.RemoteHost, remoteFileName, expectedVersion string) error {
	
	if expectedVersion == "" {
		
		// this needs to be executed locally
		output, err := exec.Command("git", "describe", "--tags", "--candidates=50", "--match", "[0-9]*", "--abbrev=7").Output()
		if err != nil {
			return fmt.Errorf("failed to get git version: %w", err)
		}
		// we only want the M.m.p version number, so up to the first `-` if it exists
		expectedVersion = string(output)
		expectedVersion = expectedVersion[:strings.Index(expectedVersion, "-")]
	}
	versions := strings.Split(expectedVersion, ".")
	fmt.Printf("Looking for version %v\n", versions)
	if len(versions) != 3 {
		return fmt.Errorf("expected version must be in the form M.m.p.b")
	}
	pscommand := fmt.Sprintf(`(Get-Item "%s").VersionInfo.FileVersion`, remoteFileName)
	remoteversion, err := host.Execute(pscommand)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}
	remoteversionparts := strings.Split(strings.TrimSpace(remoteversion), ".")
	fmt.Printf("Found version %v\n", remoteversionparts)
	for idx, v := range versions {
		if v != remoteversionparts[idx] {
			return fmt.Errorf("expected version %v, got %v", expectedVersion, remoteversion)
		}
	}
	return nil
}