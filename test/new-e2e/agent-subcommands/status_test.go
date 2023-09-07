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

type commandStatusSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestStatusSuite(t *testing.T) {
	e2e.Run(t, &commandStatusSuite{}, e2e.AgentStackDef(nil))
}

// XXX: this test is expected to fail until 7.48 as a known status render errors has been fixed in #18123
func (v *commandStatusSuite) TestNoRenderError() {
	status := v.Env().Agent.Status()
	assert.NotContains(v.T(), status.Content, "Status render errors")
}
