// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package testutil provides utilities for testing the fmapper program
package testutil

import (
	"fmt"
	"os/exec"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

// mutex protecting build process
var mux sync.Mutex

// OpenFromAnotherProcess launches an external file that holds active handler to the given paths.
func OpenFromAnotherProcess(t *testing.T, paths ...string) (*exec.Cmd, error) {
	programExecutable := build(t)

	cmd := exec.Command(programExecutable, paths...)
	patternScanner := protocolstestutil.NewScanner(regexp.MustCompile("awaiting signal"), make(chan struct{}, 1))
	cmd.Stdout = patternScanner
	cmd.Stderr = patternScanner

	require.NoError(t, cmd.Start())

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
			return nil, fmt.Errorf("couldn't luanch process in time")
		}
	}
}

// build only gets executed when running tests locally
func build(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, "fmapper")
	require.NoError(t, err)
	return serverBin
}
