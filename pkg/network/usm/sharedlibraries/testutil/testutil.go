// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package testutil provides utilities for testing the fmapper program
package testutil

import (
	"errors"
	"os/exec"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	protocolstestutil "github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// mutex protecting build process
var mux sync.Mutex

// BuildFmapperScanner creates a new pattern scanner for the fmapper program,
// that scans for the "awaiting signal" pattern that indicates that the program
// has started correctly.
func BuildFmapperScanner(t testing.TB) *protocolstestutil.PatternScanner {
	patternScanner, err := protocolstestutil.NewScanner(regexp.MustCompile("awaiting signal"), protocolstestutil.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")
	return patternScanner
}

// OpenFromProcess launches the specified external program which holds an active
// handle to the given paths.
func OpenFromProcess(t *testing.T, programExecutable string, paths ...string) (*exec.Cmd, error) {
	cmd := exec.Command(programExecutable, paths...)
	patternScanner := BuildFmapperScanner(t)
	cmd.Stdout = patternScanner
	cmd.Stderr = patternScanner

	require.NoError(t, cmd.Start())
	log.Infof("exec prog=%s, paths=%v | PID = %d", programExecutable, paths, cmd.Process.Pid)

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
	})

	for {
		select {
		case <-patternScanner.DoneChan:
			return cmd, nil
		case <-time.After(time.Second * 5):
			patternScanner.PrintLogs(t)
			// please don't use t.Fatalf() here as we could test if it failed later
			return nil, errors.New("couldn't launch process in time")
		}
	}
}

// OpenFromAnotherProcess launches an external program that holds an active
// handle to the given paths.
func OpenFromAnotherProcess(t *testing.T, paths ...string) (*exec.Cmd, error) {
	programExecutable := BuildFmapper(t)
	return OpenFromProcess(t, programExecutable, paths...)
}

// BuildFmapper builds the external program which is used to hold references to
// shared libraries for testing.
func BuildFmapper(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, "fmapper")
	require.NoError(t, err)
	return serverBin
}
