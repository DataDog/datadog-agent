// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/stretchr/testify/assert"
)

type commandFlareSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestFlareSuite(t *testing.T) {
	e2e.Run(t, &commandFlareSuite{}, e2e.AgentStackDef(nil), params.WithDevMode())
}

func (v *commandFlareSuite) TestFlareCreation() {
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	_ = v.Env().Agent.Flare(client.WithArgs("--email e2e@test.com --send"))

	flare, err := v.Env().Fakeintake.Client.GetLatestFlare()
	assert.NoError(v.T(), err)
	assert.Equal(v.T(), flare.GetEmail(), "e2e@test.com")
}
