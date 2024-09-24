// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package testutil provides utilities for testing the dynamic instrumentation sample service
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var mux sync.Mutex

// BuildSampleService builds the external program which is used for testing
// Go dynamic instrumentation
func BuildSampleService(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	curDir, err := pwd()
	require.NoError(t, err)
	serverBin, err := BuildGoBinaryWrapper(curDir, "sample/sample-service")
	require.NoError(t, err)
	return serverBin
}

// BuildGoBinaryWrapper builds a Go binary and returns the path to it.
// If the binary is already built, it returns the path to the binary.
func BuildGoBinaryWrapper(curDir, binaryDir string) (string, error) {
	sampleServiceSource := path.Join(curDir, binaryDir)
	sampleServiceBinaryPath := path.Join(sampleServiceSource, binaryDir)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err := os.Stat(sampleServiceBinaryPath); err == nil {
		return sampleServiceBinaryPath, nil
	}

	c := exec.Command("go", "build", "-C", sampleServiceSource, "-o", sampleServiceBinaryPath)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not build sample service test binary: %s\noutput: %s", err, string(out))
	}

	return sampleServiceBinaryPath, nil
}

// pwd returns the current directory of the caller.
func pwd() (string, error) {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
