// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	ec2vm "github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/os"
	"github.com/stretchr/testify/assert"
)

type vmSuiteEx3 struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestVMSuiteEx3(t *testing.T) {
	e2e.Run(t, &vmSuiteEx3{}, e2e.AgentStackDef(
		[]e2e.Ec2VMOption{ec2vm.WithOS(os.UbuntuOS)},
		agent.WithAgentConfig("log_level: debug"),
	))
}

func (v *vmSuiteEx3) TestLogDebug() {
	v.Env().Agent.WaitForReady()
	assert.Contains(v.T(), v.Env().Agent.Config(), "log_level: debug")
}
