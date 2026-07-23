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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	paridentity "github.com/DataDog/datadog-agent/test/new-e2e/tests/privateactionrunner"
)

const (
	parLinuxBinaryPath      = "/opt/datadog-agent/embedded/bin/privateactionrunner"
	parLinuxProcmgrConfig   = linuxConfigDir + "/datadog-agent-action.yaml"
	parLinuxSystemdUnitName = "datadog-agent-action.service"
	parProcmgrProcessName   = "datadog-agent-action"
)

type parProcmgrLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestPARManagedByProcmgrLinux mirrors TestPARManagedByProcmgrWindows.
func TestPARManagedByProcmgrLinux(t *testing.T) {
	t.Parallel()
	config := paridentity.GenerateTestPrivateActionRunnerConfig(t)
	e2e.Run(t, &parProcmgrLinuxSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithoutFakeIntake(),
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(config),
					agentparams.WithFile(parLinuxProcmgrConfig, embedded.PARProcessConfig, true),
				),
			),
		),
	))
}

func (s *parProcmgrLinuxSuite) cli(args string) string {
	return "sudo " + linuxCLIBin + " " + args
}

func (s *parProcmgrLinuxSuite) TestPARSupervisedByProcmgrAndSystemdInactive() {
	host := s.Env().RemoteHost

	if _, err := host.Execute("test -f " + linuxCLIBin); err != nil {
		s.T().Skip("dd-procmgr CLI not included in this agent package; skipping PAR procmgr test")
	}
	if _, err := host.Execute("test -f " + parLinuxBinaryPath); err != nil {
		s.T().Skipf("%s not installed; skipping PAR procmgr test", parLinuxBinaryPath)
	}

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := host.MustExecuteOn(ct, s.cli("describe "+parProcmgrProcessName))
		assertField(ct, out, "State", "Running")
		assertField(ct, out, "Command", parLinuxBinaryPath)
	}, 120*time.Second, 3*time.Second)

	unitState := strings.TrimSpace(
		host.MustExecute("systemctl is-active " + parLinuxSystemdUnitName + " || true"))
	assert.NotEqual(s.T(), "active", unitState,
		"%s must not be active when PAR is managed by dd-procmgr", parLinuxSystemdUnitName)
}
