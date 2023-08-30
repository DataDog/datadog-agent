// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
)

type subcommandSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestStatusSuite(t *testing.T) {
	e2e.Run(t, &subcommandSuite{}, e2e.AgentStackDef(nil))
}

// XXX: this test is expected to fail until 7.48 as a known status render errors has been fixed in #18123
func (v *subcommandSuite) TestNoStatusRenderError() {
	status := v.Env().Agent.Status()
	assert.NotContains(v.T(), status.Content, "Status render errors")
}

func (v *subcommandSuite) TestDefaultInstallHealthy() {
	interval := 1 * time.Second

	var output string
	var err error
	err = backoff.Retry(func() error {
		output, err = v.Env().Agent.Health()
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(15)))

	assert.NoError(v.T(), err)
	assert.Contains(v.T(), output, "Agent health: PASS")
}
