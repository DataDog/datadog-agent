// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenariomodel

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/require"
)

type ec2HostSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestEC2HostScenario(t *testing.T) {
	t.Parallel()
	prov, err := ec2host.Provisioner(ec2host.NewEC2HostParams("ubuntu-22.04", "x86_64"))
	require.NoError(t, err)
	e2e.Run(t, &ec2HostSuite{}, e2e.WithProvisioner(prov))
}

func (s *ec2HostSuite) TestActionRunsAgainstEnv() {
	// The same action handler the CLI/service invoke, called against the live env.
	runCommand := ec2host.Scenario().Actions["run-command"]
	err := runCommand.Run(context.Background(), s.Env(), &ec2host.RunCommandParams{Command: "echo hello"})
	s.Require().NoError(err)
}
