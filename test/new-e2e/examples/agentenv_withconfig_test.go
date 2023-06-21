// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"github.com/stretchr/testify/assert"
)

type agentSuiteEx3 struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestVAgentSuiteEx3(t *testing.T) {
	e2e.Run(t, &agentSuiteEx3{}, e2e.AgentStackDef(
		[]ec2params.Option{ec2params.WithOS(ec2os.UbuntuOS)},
		agentparams.WithAgentConfig("log_level: debug"),
	))
}

func (v *agentSuiteEx3) TestLogDebug() {
	v.Env().Agent.WaitForReady()
	assert.Contains(v.T(), v.Env().Agent.Config(), "log_level: debug")
}
