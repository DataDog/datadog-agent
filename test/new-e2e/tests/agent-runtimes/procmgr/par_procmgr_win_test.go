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

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	parProcessName           = "datadog-agent-action"
	parLegacySCMServiceName  = "datadog-agent-action"
	parProcmgrConfigFileName = "datadog-agent-action.yaml"
)

func withPAREnabled() agentparams.Option {
	return func(p *agentparams.Params) error {
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("private_action_runner.enabled: true"))
		return nil
	}
}

type parProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestPARManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &parProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(withPAREnabled()),
			),
		),
	))
}

func (s *parProcmgrWindowsSuite) TestPARSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	parBin := filepath.Join(installRoot, "bin", "agent", "privateactionrunner.exe")
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, parBin))
	if err != nil {
		s.T().Skip("privateactionrunner.exe not installed; skipping PAR procmgr test")
	}

	cfg := filepath.Join(installRoot, "processes.d", parProcmgrConfigFileName)
	_, err = host.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, cfg))
	require.NoError(s.T(), err, "fleet PAR processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, parProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	_, err = host.Execute(
		fmt.Sprintf(`powershell -NoProfile -Command "$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0"`, parLegacySCMServiceName))
	require.NoError(s.T(), err, "%s Windows service must not be Running when PAR is managed by dd-procmgr", parLegacySCMServiceName)
}
