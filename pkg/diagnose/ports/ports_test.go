// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// TestBindSeverity_UnknownOwner: Pid == 0 means the agent can't retrieve the
// process (different user). Severity 1 — above agent (0), below foreign (2).
func TestBindSeverity_UnknownOwner(t *testing.T) {
	require.Equal(t, 1, bindSeverity(port.Port{Pid: 0}))
}

// TestBindSeverity_Agent: a bind whose process name is in agentNames
// ranks as 0 (agent-owned).
func TestBindSeverity_Agent(t *testing.T) {
	pid, name := realProcess(t)
	restore := swapAgentNames(t, map[string]struct{}{name: {}})
	defer restore()
	require.Equal(t, 0, bindSeverity(port.Port{Pid: pid, Process: name}))
}

// TestBindSeverity_Foreign: a bind with a known PID whose process name is
// NOT in agentNames ranks as 2 — the worst severity. This is the case that
// used to be silently collapsed into the same-port agent bind.
func TestBindSeverity_Foreign(t *testing.T) {
	pid, name := realProcess(t)
	// Replace agentNames with something that deliberately excludes `name`.
	restore := swapAgentNames(t, map[string]struct{}{"not-the-test-binary": {}})
	defer restore()
	require.Equal(t, 2, bindSeverity(port.Port{Pid: pid, Process: name}))
}

// TestWorstBindSelection is the regression test for the bug:
// Poll() now returns multiple rows for the same port on different IPs,
// and DiagnosePortSuite must pick the worst bind so a foreign conflict
// on one IP isn't masked by an agent bind on another.
func TestWorstBindSelection(t *testing.T) {
	pid, name := realProcess(t)
	// Treat the current process as "agent". The other bind uses Pid=0 which
	// is always "unknown owner" (severity 1). Agent (0) + unknown (1) must
	// select unknown — worse than agent.
	restore := swapAgentNames(t, map[string]struct{}{name: {}})
	defer restore()

	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name}, // agent
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 0},                // unknown owner
	}

	worst := pickWorst(binds)
	require.Equal(t, 0, worst.Pid, "worst-bind selection must pick the unknown-owner bind over the agent bind")
	require.Equal(t, "127.0.0.1", worst.IP)
}

// TestWorstBindSelection_ForeignOverAgent: the core scenario from the bug —
// agent on one IP, foreign-with-known-PID on another. Must pick foreign.
func TestWorstBindSelection_ForeignOverAgent(t *testing.T) {
	pid, name := realProcess(t)
	// agentNames contains only `name`, so a bind with a different process
	// name is foreign. But on Windows RetrieveProcessName ignores the passed
	// Process field and queries by PID — so for a foreign bind we must use a
	// PID that resolves to a name NOT equal to `name`. We create two entries
	// for the same PID (the test binary) but toggle agentNames between them
	// via a second assertion to simulate both classifications.
	//
	// Instead: use Pid=1 (or any non-zero PID that RetrieveProcessName can't
	// resolve to an agent). On Windows Pid=1 is usually "System Idle Process"
	// or fails; on Linux the passed-in Process ("nginx") is echoed back.
	restore := swapAgentNames(t, map[string]struct{}{name: {}})
	defer restore()

	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name}, // agent (severity 0)
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 1, Process: "nginx"},
	}

	worst := pickWorst(binds)
	// foreign (severity 2) must win over agent (0). We don't assert the exact
	// Pid because on Windows Pid=1 resolution is OS-dependent, but we do
	// assert it's not the agent bind.
	require.NotEqual(t, pid, worst.Pid, "foreign bind must beat agent bind")
}

// TestWorstBindSelection_OrderIndependent: same scenario with agent listed
// last. The selection must still land on the non-agent bind.
func TestWorstBindSelection_OrderIndependent(t *testing.T) {
	pid, name := realProcess(t)
	restore := swapAgentNames(t, map[string]struct{}{name: {}})
	defer restore()

	binds := []port.Port{
		{Proto: "tcp", Port: 8126, IP: "127.0.0.1", Pid: 0},                // unknown
		{Proto: "tcp", Port: 8126, IP: "0.0.0.0", Pid: pid, Process: name}, // agent
	}

	worst := pickWorst(binds)
	require.Equal(t, 0, worst.Pid, "unknown-owner bind must beat agent bind regardless of order")
}

// pickWorst replicates the selection loop in DiagnosePortSuite so the test
// exercises identical logic without depending on pkgconfigsetup.
func pickWorst(binds []port.Port) port.Port {
	worst := binds[0]
	sev := bindSeverity(worst)
	for _, b := range binds[1:] {
		if s := bindSeverity(b); s > sev {
			worst, sev = b, s
		}
	}
	return worst
}

// swapAgentNames replaces the package-level agentNames map for the duration
// of a test. Returns a function that restores the original value.
func swapAgentNames(t *testing.T, replacement map[string]struct{}) func() {
	t.Helper()
	prev := agentNames
	agentNames = replacement
	return func() { agentNames = prev }
}
