// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/stretchr/testify/assert"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(nil))
}

func (v *agentSuite) TestAgentCommandNoArg() {
	version := v.Env().Agent.Version()

	match, err := regexp.MatchString("^Agent .* - Commit: .* - Serialization version: .* - Go version: .*$", strings.TrimSuffix(version, "\n"))
	assert.NoError(v.T(), err)
	assert.True(v.T(), match)
}

func (v *agentSuite) TestAgentCommandWithArg() {
	status := v.Env().Agent.Status(client.WithArgs("-h"))
	assert.Contains(v.T(), status.Content, "Use \"datadog-agent status [command] --help\" for more information about a command.")
}
