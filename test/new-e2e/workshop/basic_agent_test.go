// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workshop

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

type basicAgentSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestBasicAgentSuite(t *testing.T) {
	t.Logf("Starting e2e.Run at %q", time.Now().String())
	e2e.Run[e2e.AgentEnv](t, &basicAgentSuite{}, e2e.AgentStackDef(
		[]ec2params.Option{
			ec2params.WithOS(ec2os.AmazonLinuxDockerOS),
			ec2params.WithName("sopell-instance-woo"),
		},
		agentparams.WithAgentConfig("log_level: debug"),
		agentparams.WithTelemetry(),
	), params.WithDevMode(), params.WithStackName("cache-busteeeeer"))
}

func (v *basicAgentSuite) TestBasicVM() {
	require.True(v.T(), v.Env().Agent.IsReady())
	config := v.Env().Agent.Config()
	require.Contains(v.T(), config, "log_level: debug")

	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("log_level: trace")))
	require.Contains(v.T(), v.Env().Agent.Config(), "log_level: trace")
}
