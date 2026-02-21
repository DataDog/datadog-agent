// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package testutil contains helpers to build sample C binaries for testing.

//go:build linux && test

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

// BuildOptions configures how to build a sample binary
type BuildOptions struct {
	// UseCUDA indicates whether to build with CUDA support using nvcc
	UseCUDA bool
}

// DefaultBuildOptions returns the default build options (no CUDA)
func DefaultBuildOptions() BuildOptions {
	return BuildOptions{
		UseCUDA: false,
	}
}

func buildCBinary(srcDir, outPath string, opts BuildOptions) (string, error) {
	mux.Lock()
	defer mux.Unlock()

	serverSrcDir := srcDir
	cachedServerBinaryPath := outPath
	forceBuild := os.Getenv("FORCE_BINARY_REBUILD") != ""

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err := os.Stat(cachedServerBinaryPath); err == nil && !forceBuild {
		log.Debugf("Using cached test binary: %s", cachedServerBinaryPath)
		return cachedServerBinaryPath, nil
	}

	var buildCmd []string
	if opts.UseCUDA {
		// Building CUDA binaries doesn't really support static linking well
		buildCmd = []string{"nvcc", serverSrcDir, "-o", cachedServerBinaryPath}
	} else {
		// Build statically to avoid issues with shared libraries (specially libc if we run in alpine)
		buildCmd = []string{"clang", "-static", serverSrcDir, "-o", cachedServerBinaryPath}
	}

	log.Debugf("Building test binary: %s", buildCmd)
	c := exec.Command(buildCmd[0], buildCmd[1:]...)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not build test binary: %s\noutput: %s", err, string(out))
	}

	return cachedServerBinaryPath, nil
}
