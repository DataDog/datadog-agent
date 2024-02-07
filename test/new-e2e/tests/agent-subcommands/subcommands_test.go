// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
)

type subcommandWithFakeIntakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSubcommandSuite(t *testing.T) {
	e2e.Run(t, &subcommandWithFakeIntakeSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// section contains the content status of a specific section (e.g. Forwarder)

func (v *subcommandWithFakeIntakeSuite) TestDefaultInstallHealthy() {
	interval := 1 * time.Second

	var output string
	var err error
	err = backoff.Retry(func() error {
		output, err = v.Env().Agent.Client.Health()
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(15)))

	assert.NoError(v.T(), err)
	assert.Contains(v.T(), output, "Agent health: PASS")
}

func (v *subcommandWithFakeIntakeSuite) TestDefaultInstallUnhealthy() {
	// the fakeintake says that any API key is invalid by sending a 403 code
	override := api.ResponseOverride{
		Endpoint:    "/api/v1/validate",
		StatusCode:  403,
		ContentType: "text/plain",
		Body:        []byte("invalid API key"),
	}
	v.Env().FakeIntake.Client().ConfigureOverride(override)

	// restart the agent, which validates the key using the fakeintake at startup
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info\n")),
	))

	// agent should be unhealthy because the key is invalid
	_, err := v.Env().Agent.Client.Health()
	if err == nil {
		assert.Fail(v.T(), "agent expected to be unhealthy, but no error found!")
		return
	}
	assert.Contains(v.T(), err.Error(), "Agent health: FAIL")
}
