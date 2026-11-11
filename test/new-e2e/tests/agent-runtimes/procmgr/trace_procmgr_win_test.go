// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
)

type traceProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli string
	// autoSpawnPID is the default-install PID; the cutover test requires it still Running.
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

// Both halves must hold: procmgr Running alone means two trace-agents; legacy Stopped alone means none.
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

// Trace-agent must keep the Agent spawn profile (install user), not LocalSystem.
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
