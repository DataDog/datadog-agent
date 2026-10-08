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
	paridentity "github.com/DataDog/datadog-agent/test/new-e2e/tests/privateactionrunner"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	parControlProcessName     = "datadog-agent-par-control"
	parControlConfigFileName  = "datadog-agent-par-control.yaml"
	parMonolithProcessName    = "datadog-agent-action"
	parMonolithConfigFileName = "datadog-agent-action.yaml"
	parLegacySCMServiceName   = "datadog-agent-action"
)

type parSplitProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

type parProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestPARSplitManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	config := paridentity.GenerateTestPrivateActionRunnerConfig(t)
	e2e.Run(t, &parSplitProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault), ec2.WithInternetAccess()),
				ec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func TestPARManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	config := paridentity.GenerateTestMonolithicPrivateActionRunnerConfig(t)
	e2e.Run(t, &parProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault), ec2.WithInternetAccess()),
				ec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *parSplitProcmgrWindowsSuite) TestPARControlSupervisedByProcmgrAndLegacySCMStopped() {
	assertPARSupervisedByProcmgr(s.T(), s.Env().RemoteHost, parControlProcessName, parControlConfigFileName)
}

func (s *parProcmgrWindowsSuite) TestPARSupervisedByProcmgrAndLegacySCMStopped() {
	assertPARSupervisedByProcmgr(s.T(), s.Env().RemoteHost, parMonolithProcessName, parMonolithConfigFileName)
}

func assertPARSupervisedByProcmgr(t *testing.T, host *components.RemoteHost, processName, configFileName string) {
	t.Helper()
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)

	skipUnlessHostPath(t, host, agentBin(installRoot, "privateactionrunner.exe"),
		"privateactionrunner.exe not installed; skipping PAR procmgr test")
	requireHostPath(t, host, processesDConfig(installRoot, configFileName),
		"fleet PAR processes.d config should exist at %s")

	cli := agentBin(installRoot, "dd-procmgr.exe")
	_ = waitProcmgrRunning(t, host, cli, processName, 2*time.Minute)

	out, err := host.Execute(fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		parLegacySCMServiceName,
	))
	require.NoError(t, err)
	require.NotEqual(t, "Running", strings.TrimSpace(out),
		"%s Windows service must not be Running when PAR is managed by dd-procmgr", parLegacySCMServiceName)
}
