// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"path/filepath"
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
	paridentity "github.com/DataDog/datadog-agent/test/new-e2e/tests/privateactionrunner"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	parProcessName           = "datadog-agent-action"
	parLegacySCMServiceName  = "datadog-agent-action"
	parProcmgrConfigFileName = "datadog-agent-action.yaml"
)

type parProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestPARManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	config := paridentity.GenerateTestPrivateActionRunnerConfig(t)
	e2e.Run(t, &parProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *parProcmgrWindowsSuite) TestPARSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	parBin := filepath.Join(installRoot, "bin", "agent", "privateactionrunner.exe")
	exists, err := host.FileExists(parBin)
	require.NoError(s.T(), err)
	if !exists {
		s.T().Skip("privateactionrunner.exe not installed; skipping PAR procmgr test")
	}

	cfg := filepath.Join(installRoot, "processes.d", parProcmgrConfigFileName)
	exists, err = host.FileExists(cfg)
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "fleet PAR processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, parProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	out, err := host.Execute(fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		parLegacySCMServiceName,
	))
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "Running", strings.TrimSpace(out),
		"%s Windows service must not be Running when PAR is managed by dd-procmgr", parLegacySCMServiceName)
}
