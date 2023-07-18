// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debugtest

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/stretchr/testify/assert"
)

type commandStatusSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestStatusSuite(t *testing.T) {
	e2e.Run(t, &commandStatusSuite{}, e2e.AgentStackDef(nil))
}

func (v *commandStatusSuite) TestStatusNotEmpty() {
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	status := v.Env().Agent.Status()
	assert.NotEmpty(v.T(), status.Content)
}

func (v *commandStatusSuite) TestNoRenderError() {
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	status := v.Env().Agent.Status()
	assert.NotContains(v.T(), status.Content, "Status render errors")
}
