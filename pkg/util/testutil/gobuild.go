// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"os"
	"os/exec"
)

// IsolatedGoBuildCmd creates a "go build" command with isolated build environment.
// This may help reduce timeouts and flakiness when running tests with high parallelism.
//
// cacheDir is the directory to use for GOCACHE.
// output is the path for the output binary.
// args are additional arguments to pass to "go build" (e.g., "-tags", "foo", "source.go").
//
// See: https://github.com/golang/go/issues/59657
func IsolatedGoBuildCmd(cacheDir string, output string, args ...string) *exec.Cmd {
	cmd := exec.Command("go", append([]string{"build", "-o", output}, args...)...)
	cmd.Env = os.Environ()
	for key, value := range IsolatedGoBuildEnv(cacheDir) {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd
}

// IsolatedGoBuildEnv returns environment variables for isolated Go build operations.
// This may help reduce timeouts and flakiness when running tests with high parallelism.
//
// cacheDir is the directory to use for GOCACHE.
//
// See: https://github.com/golang/go/issues/59657
func IsolatedGoBuildEnv(cacheDir string) map[string]string {
	return map[string]string{
		"GOCACHE":   cacheDir, // avoid concurrent cache access
		"GOPRIVATE": "*",      // avoid VCS queries
		"GOPROXY":   "off",    // avoid network access
	}
}
