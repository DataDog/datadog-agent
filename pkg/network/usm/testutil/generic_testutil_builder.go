// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides utilities for testing USM.
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path"
)

// BuildUnixTransparentProxyServer builds the unix transparent proxy server binary and returns the path to the binary.
// If the binary is already built (meanly in the CI), it returns the path to the binary.
func BuildUnixTransparentProxyServer(curDir, binaryDir string) (string, error) {
	serverSrcDir := path.Join(curDir, binaryDir)
	cachedServerBinaryPath := path.Join(serverSrcDir, binaryDir)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err := os.Stat(cachedServerBinaryPath); err == nil {
		return cachedServerBinaryPath, nil
	}

	c := exec.Command("/home/vagrant/.gimme/versions/go1.21.5.linux.arm64/bin/go", "build", "-buildvcs=false", "-a", "-tags=test", "-ldflags=-extldflags '-static'", "-o", cachedServerBinaryPath, serverSrcDir)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not build unix transparent proxy server test binary: %s\noutput: %s", err, string(out))
	}

	return cachedServerBinaryPath, nil
}
