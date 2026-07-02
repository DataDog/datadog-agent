// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenariomodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/scenariotest"
)

type ec2HostSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestEC2HostScenario(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ec2HostSuite{},
		scenariotest.WithScenario(ec2host.Scenario(), ec2host.NewParams()))
}

func (s *ec2HostSuite) TestRunCommandAction() {
	// exercise the real action decode + dispatch against the live suite env
	err := scenariotest.RunAction(s.Env(), ec2host.Scenario(), "run-command",
		map[string]string{"command": "echo hello"})
	s.Require().NoError(err)
}
