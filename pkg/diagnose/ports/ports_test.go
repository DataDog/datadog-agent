// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ports

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/port"
)

// realProcess returns this test binary's PID and its OS-reported process
// name. Tests use this so isAgentProcess can succeed on both Windows (where
// RetrieveProcessName queries the OS by PID) and non-Windows (where it
// echoes back the passed-in name). A fake PID would fail on Windows.
func realProcess(t *testing.T) (int, string) {
	t.Helper()
	pid := os.Getpid()
	name, err := RetrieveProcessName(pid, "agent")
	require.NoError(t, err)
	return pid, name
}

func TestBindSeverity_UnknownOwner(t *testing.T) {
	sev, name := bindSeverity(port.Port{Pid: 0})
	require.Equal(t, severityUnknown, sev)
	require.Empty(t, name)
}

func TestBindSeverity_Agent(t *testing.T) {
	pid, name := realProcess(t)
	swapAgentNames(t, map[string]struct{}{name: {}})
	sev, got := bindSeverity(port.Port{Pid: pid, Process: name})
	require.Equal(t, severityAgent, sev)
	require.Equal(t, name, got)
}

func TestBindSeverity_Foreign(t *testing.T) {
	pid, name := realProcess(t)
	swapAgentNames(t, map[string]struct{}{"not-the-test-binary": {}})
	sev, _ := bindSeverity(port.Port{Pid: pid, Process: name})
	require.Equal(t, severityForeign, sev)
}

// TestWorstBind is the regression test for the bug: Poll() now returns
// multiple rows for the same port on different IPs, and DiagnosePortSuite
// must pick the worst bind so a foreign conflict on one IP isn't masked
// by an agent bind on another.
func TestWorstBind(t *testing.T) {
	pid, name := realProcess(t)
	swapAgentNames(t, map[string]struct{}{name: {}})

	// Agent bind + unknown-owner bind (Pid=0): unknown (1) beats agent (0).
	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name},
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 0},
	}
	worst, sev, _ := worstBind(binds)
	require.Equal(t, severityUnknown, sev)
	require.Equal(t, 0, worst.Pid)
}

func TestWorstBind_ForeignOverAgent(t *testing.T) {
	pid, name := realProcess(t)
	swapAgentNames(t, map[string]struct{}{name: {}})

	// Pid=1 resolves to a non-agent process (Linux echoes Process="nginx";
	// Windows resolves PID 1 to something like "System Idle Process").
	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name},
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 1, Process: "nginx"},
	}
	worst, sev, _ := worstBind(binds)
	require.Equal(t, severityForeign, sev)
	require.NotEqual(t, pid, worst.Pid)
}

// TestWorstBind_OrderIndependent: selection must not depend on slice order.
func TestWorstBind_OrderIndependent(t *testing.T) {
	pid, name := realProcess(t)
	swapAgentNames(t, map[string]struct{}{name: {}})

	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 0},
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name},
	}
	worst, sev, _ := worstBind(binds)
	require.Equal(t, severityUnknown, sev)
	require.Equal(t, 0, worst.Pid)
}

// swapAgentNames replaces the package-level agentNames map for the test's
// lifetime; the original is restored via t.Cleanup.
func swapAgentNames(t *testing.T, replacement map[string]struct{}) {
	t.Helper()
	prev := agentNames
	agentNames = replacement
	t.Cleanup(func() { agentNames = prev })
}
