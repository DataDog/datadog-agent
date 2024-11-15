// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package testutil contains helpers to build sample C binaries for testing.

//go:build linux_bpf && test

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// mutex to protect the build process
var mux sync.Mutex

func buildCBinary(srcDir, outPath string) (string, error) {
	mux.Lock()
	defer mux.Unlock()

	serverSrcDir := srcDir
	cachedServerBinaryPath := outPath

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err := os.Stat(cachedServerBinaryPath); err == nil {
		log.Debugf("Using cached test binary: %s", cachedServerBinaryPath)
		return cachedServerBinaryPath, nil
	}

	// Build statically to avoid issues with shared libraries (specially libc if we run in alpine)
	buildCmd := []string{"clang", "-static", serverSrcDir, "-o", cachedServerBinaryPath}
	log.Debugf("Building test binary: %s", buildCmd)
	c := exec.Command(buildCmd[0], buildCmd[1:]...)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not build test binary: %s\noutput: %s", err, string(out))
	}

	return cachedServerBinaryPath, nil
}
