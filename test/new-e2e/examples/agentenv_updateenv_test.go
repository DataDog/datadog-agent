// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

type agentSuiteEx4 struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestVMSuiteEx4(t *testing.T) {
	e2e.Run(t, &agentSuiteEx4{}, e2e.AgentStackDef(
		e2e.WithVMParams(ec2params.WithOS(ec2os.UbuntuOS)),
		e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: debug")),
	))
}

func (v *agentSuiteEx4) TestLogDebug() {
	assert.Contains(v.T(), v.Env().Agent.Config(), "log_level: debug")
}

func (v *agentSuiteEx4) TestLogInfo() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(
		ec2params.WithOS(ec2os.UbuntuOS)),
		e2e.WithAgentParams(agentparams.WithAgentConfig("log_level: info")),
	))
	assert.Contains(v.T(), v.Env().Agent.Config(), "log_level: info")
}
