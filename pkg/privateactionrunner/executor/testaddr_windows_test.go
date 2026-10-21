// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package executor

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// testPipeCounter uniquifies named-pipe addresses across tests in the same process.
var testPipeCounter atomic.Uint64

// testListenAddr returns a unique named-pipe address; Windows has no Unix-socket equivalent,
// so the executor's transport listens on a named pipe rather than a filesystem socket.
//
// Test hosts have no installed Agent, so the pipe's Agent account is the test
// user, standing in for the shared identity of par-control and the executor.
func testListenAddr(t *testing.T) string {
	t.Helper()
	useAgentAccount(t, winutil.GetSidFromUser)
	return fmt.Sprintf(`\\.\pipe\dd-par-test-%d-%d`, os.Getpid(), testPipeCounter.Add(1))
}

// useAgentAccount replaces the Agent account resolution for the duration of the test.
func useAgentAccount(t *testing.T, resolve func() (*windows.SID, error)) {
	t.Helper()
	original := agentAccountSID
	agentAccountSID = resolve
	t.Cleanup(func() { agentAccountSID = original })
}
