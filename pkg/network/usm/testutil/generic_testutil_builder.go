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

const (
	baseLDFlags = "-ldflags=-extldflags '-static'"
)

// buildGoBinary builds a Go binary and returns the path to it.
// If the binary is already built (meanly in the CI), it returns the
// path to the binary.
func buildGoBinary(srcDir, outPath, buildFlags string) (string, error) {
	serverSrcDir := srcDir
	cachedServerBinaryPath := outPath

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err := os.Stat(cachedServerBinaryPath); err == nil {
		return cachedServerBinaryPath, nil
	}

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-tags=test,netgo", buildFlags, "-o", cachedServerBinaryPath, serverSrcDir)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not build unix transparent proxy server test binary: %s\noutput: %s", err, string(out))
	}

	return cachedServerBinaryPath, nil
}

// BuildGoBinaryWrapper builds a Go binary and returns the path to it.
// If the binary is already built (meanly in the CI), it returns the
// path to the binary.
func BuildGoBinaryWrapper(curDir, binaryDir string) (string, error) {
	srcDir := path.Join(curDir, binaryDir)
	outPath := path.Join(srcDir, binaryDir)
	return buildGoBinary(srcDir, outPath, baseLDFlags)
}

// BuildGoBinaryWrapperWithoutSymbols builds a Go binary without symbols and returns the path to it.
// If the binary is already built (meanly in the CI), it returns the
// path to the binary.
func BuildGoBinaryWrapperWithoutSymbols(curDir, binaryDir string) (string, error) {
	srcDir := path.Join(curDir, binaryDir)
	outPath := path.Join(srcDir, binaryDir+"-nosymbols")
	return buildGoBinary(srcDir, outPath, baseLDFlags+" -s -w")
}
