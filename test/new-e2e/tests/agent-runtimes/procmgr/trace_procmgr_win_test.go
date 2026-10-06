// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	traceProcessName           = "datadog-agent-trace"
	traceLegacySCMServiceName  = "datadog-trace-agent"
	traceProcmgrConfigFileName = "datadog-agent-trace.yaml"

	// Both IsAPMEnabled keys must be false. apm_config.enabled defaults to true.
	traceAPMDisabledConfig = "apm_config:\n  enabled: false\n  error_tracking_standalone:\n    enabled: false\n"
)

type traceProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli string
	// autoSpawnPID is the trace-agent PID dd-procmgr reported after a default install, before
	// any test could start it. The cutover test requires this same PID still be Running.
	autoSpawnPID string
}

func TestTraceAgentManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &traceProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(),
			),
		),
	))
}

func (s *traceProcmgrWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	s.cli = agentBin(installRoot, "dd-procmgr.exe")

	s.autoSpawnPID = waitProcmgrRunning(s.T(), host, s.cli, traceProcessName, 2*time.Minute)
}

// TestTraceAgentCutoverSupervisedByProcmgrAndLegacySCMStopped is the end-to-end proof of the
// Windows cutover: on a default install dd-procmgr brings trace-agent up on its own, and the
// core Agent leaves the legacy SCM service alone. Both halves have to hold at once. Either
// one alone is a bug: only the first means two trace-agents, only the second means none at all
// (silent APM loss on every default Windows install).
func (s *traceProcmgrWindowsSuite) TestTraceAgentCutoverSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	requireHostPath(s.T(), host, agentBin(installRoot, "trace-agent.exe"),
		"trace-agent.exe should be installed at %s")
	requireHostPath(s.T(), host, processesDConfig(installRoot, traceProcmgrConfigFileName),
		"fleet trace-agent processes.d config should exist at %s")

	requireSupervisedOnlyByProcmgr(s.T(), host, s.cli, traceProcessName, s.autoSpawnPID,
		traceLegacySCMServiceName, 2*time.Minute)
}

// TestTraceAgentSpawnRunsAsAgentUser checks the half of the cutover the state machine cannot
// show. The legacy SCM service ran trace-agent as the install user (ddagentuser), not as
// LocalSystem. SpawnProfile::profile_for already returns Agent for datadog-agent-trace.
// A supervised trace-agent running as SYSTEM would still be Running here, and would still
// pass the cutover test above.
func (s *traceProcmgrWindowsSuite) TestTraceAgentSpawnRunsAsAgentUser() {
	host := s.Env().RemoteHost

	out, err := host.Execute(procmgrCmd(s.cli, "describe "+traceProcessName))
	require.NoError(s.T(), err)
	pid := fieldValue(out, "PID")
	require.Equal(s.T(), s.autoSpawnPID, pid,
		"%s should still be the auto-spawned PID, describe returned %s", traceProcessName, out)

	owner, err := windowsProcessOwnerByPID(host, pid)
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "NT AUTHORITY/SYSTEM", owner,
		"%s PID %s must not be spawned with the privileged profile", traceProcessName, pid)

	domain, user, err := windowsagent.GetAgentUserFromRegistry(host)
	require.NoError(s.T(), err)
	want := strings.ReplaceAll(windowsCommon.MakeDownLevelLogonName(domain, user), `\`, `/`)
	require.Equal(s.T(), want, owner,
		"%s PID %s should run as the Agent install user", traceProcessName, pid)
}

type traceProcmgrDisabledWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestTraceAgentNotRunningWhenAPMDisabledWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &traceProcmgrDisabledWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(traceAPMDisabledConfig),
				),
			),
		),
	))
}

func (s *traceProcmgrDisabledWindowsSuite) TestTraceAgentStaysUnspawnedAndLegacySCMDown() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	cli := agentBin(installRoot, "dd-procmgr.exe")

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(cli, "describe "+traceProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, "Created", fieldValue(out, "State"),
			"dd-procmgr should leave trace-agent unspawned when APM is off: %s", out)
	}, 2*time.Minute, 5*time.Second)

	status, err := host.Execute(
		`$s = Get-Service -Name 'datadog-trace-agent' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`)
	require.NoError(s.T(), err)
	serviceState := strings.TrimSpace(status)
	require.True(s.T(), serviceState == "Stopped" || serviceState == "Absent",
		"legacy %s should be Stopped or Absent when APM is off, got %s", traceLegacySCMServiceName, serviceState)

	out, err := host.Execute(
		`$p = Get-Process -Name 'trace-agent' -ErrorAction SilentlyContinue; if ($null -eq $p) { 'Absent' } else { 'Present' }`)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "Absent", strings.TrimSpace(out),
		"trace-agent.exe must not run when APM and Error Tracking standalone are off")
}
