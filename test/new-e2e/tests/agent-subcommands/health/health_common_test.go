// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"net/http"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type baseHealthSuite struct {
	e2e.BaseSuite[environments.Host]
}

// section contains the content status of a specific section (e.g. Forwarder)
func (v *baseHealthSuite) TestDefaultInstallHealthy() {
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

func (v *baseHealthSuite) TestDefaultInstallUnhealthy() {
	// the fakeintake says that any API key is invalid by sending a 403 code
	override := api.ResponseOverride{
		Endpoint:   "/api/v1/validate",
		StatusCode: 403,
		Method:     http.MethodGet,
		Body:       []byte("invalid API key"),
	}
	err := v.Env().FakeIntake.Client().ConfigureOverride(override)
	require.NoError(v.T(), err)

	// restart the agent, which validates the key using the fakeintake at startup
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(v.Env().RemoteHost.Descriptor())),
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info\nforwarder_apikey_validation_interval: 1")),
	))

	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		// forwarder should be unhealthy because the key is invalid
		_, err = v.Env().Agent.Client.Health()
		assert.ErrorContains(collect, err, "Agent health: FAIL")
		assert.ErrorContains(collect, err, "=== 1 unhealthy components ===\nforwarder")
	}, time.Second*30, time.Second)

	// the fakeintake now says that the api key is valid
	override.StatusCode = 200
	override.Body = []byte("valid API key")
	err = v.Env().FakeIntake.Client().ConfigureOverride(override)
	require.NoError(v.T(), err)

	// the agent will check every minute if the key is valid, so wait at most 1m30
	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		_, err = v.Env().Agent.Client.Health()
		assert.NoError(collect, err)
	}, time.Second*90, 3*time.Second)
}
