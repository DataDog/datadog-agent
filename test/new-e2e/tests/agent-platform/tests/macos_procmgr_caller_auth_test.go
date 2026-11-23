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
	macosCallerAuthStartEntry = "e2e-caller-auth-start"
	// _dd-agent is in group daemon, so it can reach the socket, but it is not root.
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
	assert.Equal(t, name, detail.Name, "dd-procmgr describe output:\n%s", out)
	return detail
}

// Execute returns an empty string on failure and puts the command output in the error.
func assertMacosProcmgrRequiresRoot(t *testing.T, client *common.MacOSTestClient, command string) string {
	t.Helper()
	_, err := client.Execute(command)
	require.Error(t, err, command)
	// grpc_err prints "{code}: {message}", and tonic displays a code as its description.
	assert.Contains(t, err.Error(), "The caller does not have permission to execute the specified operation: operation requires root")
	return err.Error()
}

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

// TestZXProcmgrRootOnlyMutations proves the root-only rule for mutating RPCs. ZX sorts
// before the ZY tests, which must find the shipped catalog.
func (m *macosInstallSuite) TestZXProcmgrRootOnlyMutations() {
	t := m.T()
	client := common.NewMacOSTestClient(m.Env().RemoteHost)
	// Unique per run, so cleanup can match this child after a daemon crash drops the runtime entry.
	sleepArg := strconv.Itoa(100000 + int(time.Now().UnixNano()%900000))
	// Cleanup runs only after the preconditions pass. Before that the test has changed nothing,
	// and the restart would stop every managed process, including one this test did not start.
	preconditionsPassed := false

	// Registered first so it runs after diagnostics. On failure, stop and kill before the
	// restart: each child has its own process group and would outlive the restart, which
	// drops the runtime entry.
	t.Cleanup(func() {
		if !preconditionsPassed {
			return
		}
		if t.Failed() {
			if _, err := client.Execute("sudo " + macosProcmgrCLI + " stop " + macosCallerAuthEntry); err != nil {
				t.Logf("cleanup: failed to stop %s: %v", macosCallerAuthEntry, err)
			}
			if _, err := client.Execute("sudo " + macosProcmgrCLI + " stop " + macosCallerAuthStartEntry); err != nil {
				t.Logf("cleanup: failed to stop %s: %v", macosCallerAuthStartEntry, err)
			}
			// pkill returns before the process exits. Poll, then SIGKILL a survivor and poll
			// again, so a retry does not inherit it after the daemon drops the entry.
			check := "sudo sh -c 'pgrep -f \"^/bin/sleep " + sleepArg + "$\"; status=$?; if [ \"$status\" -eq 1 ]; then exit 0; fi; exit \"$status\"'"
			if _, err := client.Execute("sudo pkill -f '^/bin/sleep " + sleepArg + "$'"); err != nil {
				t.Logf("cleanup: pkill /bin/sleep %s: %v", sleepArg, err)
			}
			waitGone := func(timeout time.Duration) (string, error) {
				deadline := time.Now().Add(timeout)
				for {
					left, err := client.Execute(check)
					if err != nil || strings.TrimSpace(left) == "" || time.Now().After(deadline) {
						return left, err
					}
					time.Sleep(250 * time.Millisecond)
				}
			}
			left, err := waitGone(5 * time.Second)
			confirmedGone := err == nil && strings.TrimSpace(left) == ""
			if !confirmedGone {
				if _, killErr := client.Execute("sudo pkill -9 -f '^/bin/sleep " + sleepArg + "$'"); killErr != nil {
					t.Logf("cleanup: pkill -9 /bin/sleep %s: %v", sleepArg, killErr)
				}
				left, err = waitGone(2 * time.Second)
			}
			if err != nil {
				t.Errorf("cleanup: failed to check for /bin/sleep %s: %v", sleepArg, err)
			} else if strings.TrimSpace(left) != "" {
				t.Errorf("cleanup: /bin/sleep %s still running:\n%s", sleepArg, left)
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
	preconditionsPassed = true

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

	// A skipped sleep, not the shipped system-probe. A regression must not launch a second
	// system-probe next to the legacy LaunchDaemon, which cleanup cannot tell apart.
	_, err = client.Execute("sudo " + macosProcmgrCLI + " create --name " + macosCallerAuthStartEntry + " --command /bin/sleep --args " + sleepArg + " --no-auto-start --restart-policy never")
	require.NoError(t, err)
	deniedTarget := macosProcmgrDescribe(t, client, macosCallerAuthStartEntry)
	assert.Equal(t, "Skipped", deniedTarget.State)
	assert.Equal(t, 0, deniedTarget.Pid)

	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" create --name "+macosCallerAuthAgentEntry+" --command /bin/sleep --args 600 --no-auto-start")
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" start "+macosCallerAuthStartEntry)
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" stop "+macosCallerAuthEntry)
	denied := assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" start does-not-exist")
	deniedLower := strings.ToLower(denied)
	assert.NotContains(t, deniedLower, "notfound")
	assert.NotContains(t, deniedLower, "not found")
	assertMacosProcmgrRequiresRoot(t, client, macosCallerAuthAgentCLI+" reload")

	stillRunning := macosProcmgrDescribe(t, client, macosCallerAuthEntry)
	assert.Equal(t, "Running", stillRunning.State)
	assert.Equal(t, running.Pid, stillRunning.Pid)
	stillSkipped := macosProcmgrDescribe(t, client, macosCallerAuthStartEntry)
	assert.Equal(t, "Skipped", stillSkipped.State)
	assert.Equal(t, 0, stillSkipped.Pid)
	if names, ok := macosProcmgrListNames(t, client, "sudo "+macosProcmgrCLI+" list --json"); ok {
		assert.NotContains(t, names, macosCallerAuthAgentEntry)
	}
	// The legacy sysprobe LaunchDaemon may have its own process, so describe, not pgrep.
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
	reloadOut, err := client.Execute("sudo " + macosProcmgrCLI + " reload --json")
	if assert.NoError(t, err, "dd-procmgr reload") {
		var reload struct {
			Unchanged []string `json:"unchanged"`
		}
		if assert.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(reloadOut)), &reload), "dd-procmgr reload output:\n%s", reloadOut) {
			assert.Contains(t, reload.Unchanged, macosSysprobeProcess, "dd-procmgr reload output:\n%s", reloadOut)
		}
	}
}
