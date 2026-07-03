// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"path/filepath"
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
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	processAgentProcessName          = "datadog-agent-process"
	processAgentLegacySCMServiceName = "datadog-process-agent"
	processAgentProcmgrConfigFile    = "datadog-agent-process.yaml"
)

type processProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestProcessAgentManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &processProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(),
			),
		),
	))
}

func (s *processProcmgrWindowsSuite) TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	processBin := filepath.Join(installRoot, "bin", "agent", "process-agent.exe")
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, processBin))
	require.NoError(s.T(), err, "process-agent.exe should be installed at %s", processBin)

	cfg := filepath.Join(installRoot, "processes.d", processAgentProcmgrConfigFile)
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, cfg))
	require.NoError(s.T(), err, "fleet process-agent processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processAgentProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	_, err = host.Execute(
		fmt.Sprintf(`powershell -NoProfile -Command "$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0"`, processAgentLegacySCMServiceName))
	require.NoError(s.T(), err, "%s Windows service must not be Running when process-agent is managed by dd-procmgr", processAgentLegacySCMServiceName)
}
