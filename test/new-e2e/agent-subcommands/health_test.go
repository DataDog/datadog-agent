// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/stretchr/testify/assert"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentHealthSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(nil))
}

func (v *agentSuite) TestDefaultInstallHealthy() {
	output := v.Env().Agent.Health()

	assert.Contains(v.T(), output, "Agent health: PASS")
}
