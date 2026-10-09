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
	securityProcessName           = "datadog-agent-security"
	securityLegacySCMServiceName  = "datadog-security-agent"
	securityProcmgrConfigFileName = "datadog-agent-security.yaml"

	// Gate is system-probe.yaml (cws Servicedef / processes.d), not security-agent.yaml.
	securitySystemProbeConfig         = "log_level: debug\nruntime_security_config:\n  enabled: true\n"
	securityDisabledSystemProbeConfig = "log_level: debug\nruntime_security_config:\n  enabled: false\n"
)

type securityProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli string
	// autoSpawnPID is the install-time PID; the cutover test requires it still Running.
	autoSpawnPID string
}

func TestSecurityAgentManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &securityProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithSystemProbeConfig(securitySystemProbeConfig),
				),
			),
		),
	))
}

func (s *securityProcmgrWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	s.cli = agentBin(installRoot, "dd-procmgr.exe")

	s.autoSpawnPID = waitProcmgrRunning(s.T(), host, s.cli, securityProcessName, 3*time.Minute)
}

// Both halves must hold: procmgr Running alone means two security-agents; legacy Stopped alone means none.
func (s *securityProcmgrWindowsSuite) TestSecurityAgentCutoverSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	requireHostPath(s.T(), host, agentBin(installRoot, "security-agent.exe"),
		"security-agent.exe should be installed at %s")
	requireHostPath(s.T(), host, processesDConfig(installRoot, securityProcmgrConfigFileName),
		"fleet security-agent processes.d config should exist at %s")

	requireSupervisedOnlyByProcmgr(s.T(), host, s.cli, securityProcessName, s.autoSpawnPID,
		securityLegacySCMServiceName, 2*time.Minute)
}

// Security-agent must keep the Agent spawn profile (install user), not LocalSystem.
func (s *securityProcmgrWindowsSuite) TestSecurityAgentSpawnRunsAsAgentUser() {
	host := s.Env().RemoteHost

	out, err := host.Execute(procmgrCmd(s.cli, "describe "+securityProcessName))
	require.NoError(s.T(), err)
	pid := fieldValue(out, "PID")
	require.Equal(s.T(), s.autoSpawnPID, pid,
		"%s should still be the auto-spawned PID, describe returned %s", securityProcessName, out)

	owner, err := windowsProcessOwnerByPID(host, pid)
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "NT AUTHORITY/SYSTEM", owner,
		"%s PID %s must not be spawned with the privileged profile", securityProcessName, pid)

	domain, user, err := windowsagent.GetAgentUserFromRegistry(host)
	require.NoError(s.T(), err)
	want := strings.ReplaceAll(windowsCommon.MakeDownLevelLogonName(domain, user), `\`, `/`)
	require.Equal(s.T(), want, owner,
		"%s PID %s should run as the Agent install user", securityProcessName, pid)
}

type securityDisabledProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]

	cli string
}

func TestSecurityAgentGateClosedProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &securityDisabledProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithSystemProbeConfig(securityDisabledSystemProbeConfig),
				),
			),
		),
	))
}

func (s *securityDisabledProcmgrWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	s.cli = agentBin(installRoot, "dd-procmgr.exe")
}

// Gate closed: not Running under procmgr, and legacy SCM stays down.
func (s *securityDisabledProcmgrWindowsSuite) TestSecurityAgentNotRunningWhenGateClosed() {
	host := s.Env().RemoteHost

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(procmgrCmd(s.cli, "describe "+securityProcessName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.NotEqual(ct, "Running", fieldValue(out, "State"),
			"%s must not be Running when runtime_security_config.enabled is false: %s",
			securityProcessName, out)
	}, 2*time.Minute, 3*time.Second)

	out, err := host.Execute(fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		securityLegacySCMServiceName,
	))
	require.NoError(s.T(), err)
	require.Contains(s.T(), []string{"Stopped", "Absent"}, strings.TrimSpace(out),
		"%s must stay down while the gate is closed", securityLegacySCMServiceName)
}
