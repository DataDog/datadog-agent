// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
)

const (
	macosCallerAuthEntry      = "e2e-caller-auth"
	macosCallerAuthAgentEntry = "e2e-caller-auth-agent"
	// macosCallerAuthAgentCLI is the dd-procmgr CLI as the Agent account. _dd-agent is in
	// group daemon, which can connect to the socket, and is not root.
	macosCallerAuthAgentCLI = "sudo -u _dd-agent " + macosProcmgrCLI

	macosProcmgrSocket = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
)

type macosProcmgrDetail struct {
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Pid         int      `json:"pid"`
	SkipReasons []string `json:"skip_reasons"`
}

func macosProcmgrDescribe(t *testing.T, client *common.MacOSTestClient, name string) macosProcmgrDetail {
	t.Helper()
	out, err := client.Execute("sudo " + macosProcmgrCLI + " describe " + name + " --json")
	require.NoError(t, err, "dd-procmgr describe %s failed:\n%s", name, out)
	var detail macosProcmgrDetail
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &detail), "dd-procmgr describe output:\n%s", out)
	return detail
}

// assertMacosProcmgrRequiresRoot checks a mutating CLI command is denied. Execute returns
// an empty string on failure and puts the command output in the error.
func assertMacosProcmgrRequiresRoot(t *testing.T, client *common.MacOSTestClient, command string) string {
	t.Helper()
	_, err := client.Execute(command)
	require.Error(t, err, command)
	assert.Contains(t, err.Error(), "operation requires root")
	return err.Error()
}

// macosProcmgrListNames runs list --json and returns the process names. A failed
// list is reported and returns false so the rest of the test still runs.
func macosProcmgrListNames(t *testing.T, client *common.MacOSTestClient, command string) ([]string, bool) {
	t.Helper()
	out, err := client.Execute(command)
	if !assert.NoError(t, err, "%s failed:\n%s", command, out) {
		return nil, false
	}
	var list []struct {
		Name string `json:"name"`
	}
	if !assert.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &list), "dd-procmgr list output:\n%s", out) {
		return nil, false
	}
	names := make([]string, 0, len(list))
	for _, process := range list {
		names = append(names, process.Name)
	}
	return names, true
}

// TestZXProcmgrRootOnlyMutations proves the installed daemon's root-only rule: root may run
// every mutating RPC, _dd-agent may run none, and reads stay open. ZX sorts after the
// read-only tests and before the ZY tests, which must find the shipped catalog.
func (m *macosInstallSuite) TestZXProcmgrRootOnlyMutations() {
	t := m.T()
	client := common.NewMacOSTestClient(m.Env().RemoteHost)
	// The duration is unique per run, so cleanup can match this child's command line after
	// a daemon crash drops the runtime entry.
	sleepArg := strconv.Itoa(100000 + int(time.Now().UnixNano()%900000))

	// Registered first so it runs after diagnostics. The stop is only on failure: the
	// child has its own process group and can outlive the daemon restart, which drops
	// the runtime entry.
	t.Cleanup(func() {
		if t.Failed() {
			if _, err := client.Execute("sudo " + macosProcmgrCLI + " stop " + macosCallerAuthEntry); err != nil {
				t.Logf("cleanup: failed to stop %s: %v", macosCallerAuthEntry, err)
			}
			// The shipped entry's child has its own process group. A restarted daemon
			// reloads that entry as skipped and does not see a child a failed denial started.
			if _, err := client.Execute("sudo " + macosProcmgrCLI + " stop " + macosSysprobeProcess); err != nil {
				t.Logf("cleanup: failed to stop %s: %v", macosSysprobeProcess, err)
			}
			if _, err := client.Execute("sudo pkill -f '^/bin/sleep " + sleepArg + "$'"); err != nil {
				t.Logf("cleanup: no leftover child for /bin/sleep %s or pkill failed: %v", sleepArg, err)
			}
		}
		if _, err := client.Execute("sudo launchctl kickstart -k system/com.datadoghq.procmgr"); err != nil {
			t.Logf("cleanup: failed to restart dd-procmgrd: %v", err)
		}
		assertMacosProcmgrAnswers(t, client, 1)
	})
	macosLogDiagnosticsOnFailure(t, client)

	assertMacosProcmgrAnswers(t, client, 1)
	statOut, statErr := client.Execute("sudo stat -f '%Su:%Sg %Lp' " + macosProcmgrSocket)
	require.False(t, t.Failed(), "preconditions failed")
	require.NoError(t, statErr, "stat %s failed:\n%s", macosProcmgrSocket, statOut)
	require.Equal(t, "root:daemon 660", strings.TrimSpace(statOut), "socket owner or mode")

	_, err := client.Execute("sudo " + macosProcmgrCLI + " create --name " + macosCallerAuthEntry + " --command /bin/sleep --args " + sleepArg + " --no-auto-start --restart-policy never")
	require.NoError(t, err)
	created := macosProcmgrDescribe(t, client, macosCallerAuthEntry)
	assert.Equal(t, "Skipped", created.State)
	assert.Equal(t, 0, created.Pid)
	assert.Equal(t, []string{"auto_start_false"}, created.SkipReasons)

	_, err = client.Execute("sudo " + macosProcmgrCLI + " start " + macosCallerAuthEntry)
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := client.Execute("sudo " + macosProcmgrCLI + " describe " + macosCallerAuthEntry + " --json")
		if !assert.NoError(c, err, "dd-procmgr describe failed:\n%s", out) {
			return
		}
		var detail macosProcmgrDetail
		if !assert.NoError(c, json.Unmarshal([]byte(strings.TrimSpace(out)), &detail), "dd-procmgr describe output:\n%s", out) {
			return
		}
		assert.Equal(c, "Running", detail.State)
		assert.NotZero(c, detail.Pid)
	}, 30*time.Second, time.Second)
	running := macosProcmgrDescribe(t, client, macosCallerAuthEntry)
	require.Equal(t, "Running", running.State)
	require.NotZero(t, running.Pid)

	if names, ok := macosProcmgrListNames(t, client, macosCallerAuthAgentCLI+" list --json"); ok {
		assert.Contains(t, names, macosSysprobeProcess)
		assert.Contains(t, names, macosCallerAuthEntry)
	}
	for _, args := range []string{
		"status --json",
		"describe " + macosCallerAuthEntry + " --json",
		"config --json",
	} {
		_, err := client.Execute(macosCallerAuthAgentCLI + " " + args)
		assert.NoError(t, err, "dd-procmgr %s as _dd-agent", args)
	}

	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" create --name "+macosCallerAuthAgentEntry+" --command /bin/sleep --args 600 --no-auto-start")
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" start "+macosSysprobeProcess)
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" stop "+macosCallerAuthEntry)
	denied := assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" start does-not-exist")
	deniedLower := strings.ToLower(denied)
	assert.NotContains(t, deniedLower, "notfound")
	assert.NotContains(t, deniedLower, "not found")
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" reload")

	stillRunning := macosProcmgrDescribe(t, client, macosCallerAuthEntry)
	assert.Equal(t, "Running", stillRunning.State)
	assert.Equal(t, running.Pid, stillRunning.Pid)
	if names, ok := macosProcmgrListNames(t, client, "sudo "+macosProcmgrCLI+" list --json"); ok {
		assert.NotContains(t, names, macosCallerAuthAgentEntry)
	}
	// describe, not pgrep: the legacy sysprobe LaunchDaemon may have its own process.
	sysprobe := macosProcmgrDescribe(t, client, macosSysprobeProcess)
	assert.Equal(t, "Skipped", sysprobe.State)
	assert.Equal(t, []string{"auto_start_false"}, sysprobe.SkipReasons)
	assert.Equal(t, 0, sysprobe.Pid)

	_, err = client.Execute("sudo " + macosProcmgrCLI + " stop " + macosCallerAuthEntry)
	if assert.NoError(t, err, "dd-procmgr stop %s", macosCallerAuthEntry) {
		stopped := macosProcmgrDescribe(t, client, macosCallerAuthEntry)
		assert.Equal(t, "Stopped", stopped.State)
		assert.Equal(t, 0, stopped.Pid)
	}
	_, err = client.Execute("sudo " + macosProcmgrCLI + " reload")
	assert.NoError(t, err, "dd-procmgr reload")
}
