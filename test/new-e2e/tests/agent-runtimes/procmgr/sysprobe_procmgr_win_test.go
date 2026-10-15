// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	sysprobeProcessName           = "datadog-agent-sysprobe"
	sysprobeLegacySCMServiceName  = "datadog-system-probe"
	sysprobeProcmgrConfigFileName = "datadog-agent-sysprobe.yaml"

	// system-probe decides it is not enabled, sleeps 5 seconds and exits 0 unless at least one
	// of the keys in the processes.d condition_config_any is set, so on a default install there
	// is no supervised process to observe. network_config is the same term
	// test/new-e2e/tests/windows/service-test/fixtures/system-probe.yaml uses.
	sysprobeSystemProbeConfig = "log_level: debug\nnetwork_config:\n  enabled: true\n"
)

type sysprobeProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli string
	// autoSpawnPID is the system-probe PID dd-procmgr reported after the install, before any
	// test could act on the process. Later tests require this same PID still be the one
	// supervised, held for procmgrHoldFor rather than checked once.
	autoSpawnPID string
}

func TestSystemProbeManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &sysprobeProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithSystemProbeConfig(sysprobeSystemProbeConfig),
				),
			),
		),
	))
}

func (s *sysprobeProcmgrWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	s.cli = agentBin(installRoot, "dd-procmgr.exe")

	// The first half of the cutover proof, recorded before any test runs so the PID can only
	// be the one dd-procmgr spawned on its own. The window is wider than the process-agent
	// suite's: system-probe loads the kernel drivers on the way up.
	s.autoSpawnPID = waitProcmgrRunning(s.T(), host, s.cli, sysprobeProcessName, 3*time.Minute)
}

// TestSystemProbeCutoverSupervisedByProcmgrAndLegacySCMStopped is the end-to-end proof of the
// system-probe cutover: dd-procmgr brings system-probe up on its own, and the core Agent
// leaves the legacy SCM service alone. Both halves have to hold at once. Either one alone is a
// bug: only the first means two system-probes over one named pipe and one set of kernel
// drivers, only the second means none at all.
//
// The PID is what makes this stronger than asserting the process is Running. system-probe is
// restart: on-failure with restart_sec 2, so a crash loop spends most of its time Running with
// a different PID each time. requireSupervisedOnlyByProcmgr requires the PID SetupSuite
// recorded, which also rules out an auto-start that failed and was later repaired by something
// else, and it holds that together with the legacy service being down over one window, since
// the claim is that both are true at once. The two minutes is the deadline for that hold.
func (s *sysprobeProcmgrWindowsSuite) TestSystemProbeCutoverSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	requireHostPath(s.T(), host, agentBin(installRoot, "system-probe.exe"),
		"system-probe.exe should be installed at %s")
	requireHostPath(s.T(), host, processesDConfig(installRoot, sysprobeProcmgrConfigFileName),
		"fleet system-probe processes.d config should exist at %s")

	requireSupervisedOnlyByProcmgr(s.T(), host, s.cli, sysprobeProcessName, s.autoSpawnPID,
		sysprobeLegacySCMServiceName, 2*time.Minute)
}

// TestSystemProbePrivilegedSpawnRunsAsLocalSystem checks the half of the cutover the state
// machine cannot show. system-probe loads kernel drivers, so the legacy SCM service ran it as
// LocalSystem rather than as ddagentuser like the rest of the Agent. dd-procmgr reproduces that
// through SpawnProfile::Privileged, which is an allowlist of two process names in
// pkg/procmgr/rust/src/spawn/profile.rs and defaults to the Agent account for everything else.
// A supervised system-probe running as ddagentuser would still be Running here, and would still
// pass the cutover test above, right up until it failed to load a driver.
func (s *sysprobeProcmgrWindowsSuite) TestSystemProbePrivilegedSpawnRunsAsLocalSystem() {
	host := s.Env().RemoteHost

	out, err := host.Execute(procmgrCmd(s.cli, "describe "+sysprobeProcessName))
	require.NoError(s.T(), err)
	pid := fieldValue(out, "PID")
	require.Equal(s.T(), s.autoSpawnPID, pid,
		"%s should still be the auto-spawned PID, describe returned %s", sysprobeProcessName, out)

	// Resolved by PID rather than by image name so the answer is about the process dd-procmgr
	// reports supervising, not some other system-probe.exe that happens to be on the host.
	owner, err := windowsProcessOwnerByPID(host, pid)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "NT AUTHORITY/SYSTEM", owner,
		"%s PID %s should be spawned with the privileged profile", sysprobeProcessName, pid)
}

// windowsProcessOwnerByPID returns the "DOMAIN/USER" that owns pid on the remote host.
func windowsProcessOwnerByPID(host *components.RemoteHost, pid string) (string, error) {
	script := fmt.Sprintf(
		`$p = Get-CimInstance Win32_Process -Filter "ProcessId=%s"; if ($null -eq $p) { throw 'no process with PID %s' }; `+
			`$o = Invoke-CimMethod -InputObject $p -MethodName GetOwner; `+
			`if ($o.ReturnValue -ne 0) { throw "GetOwner failed: $($o.ReturnValue)" }; "$($o.Domain)/$($o.User)"`,
		pid, pid,
	)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}
