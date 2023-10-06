// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/stretchr/testify/require"
)

// mutex protecting build process
var mux sync.Mutex

func OpenFromAnotherProcess(t *testing.T, paths ...string) *exec.Cmd {
	programExecutable := getPrebuiltExecutable(t)

	if programExecutable == "" {
		// This can happen when we're not running in CI context, in which case we build the testing program
		programExecutable = build(t)
	}

	cmd := exec.Command(programExecutable, paths...)
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
	})

	return cmd
}

// getPrebuiltExecutable returns the path of the prebuilt fmapper program when applicable.
//
// When running tests via CI, the fmapper program is prebuilt by running `inv -e system-probe.kitchen-prepare`
// in which case we return the path of the executable. In case we're not running in
// CI context an empty string is returned.
func getPrebuiltExecutable(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	prebuiltPath := filepath.Join(cur, "fmapper/fmapper")
	_, err = os.Stat(prebuiltPath)
	if err != nil {
		return ""
	}

	return prebuiltPath
}

// build only gets executed when running tests locally
func build(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	sourcePath := filepath.Join(cur, "fmapper/fmapper.go")
	// Note that t.TempDir() gets cleaned up automatically by the Go runtime
	targetPath := filepath.Join(t.TempDir(), "fmapper")

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", targetPath, sourcePath)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build fmapper test binary: %s\noutput: %s", err, string(out))
	return targetPath
}
