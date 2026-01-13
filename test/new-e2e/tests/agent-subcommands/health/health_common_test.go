// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"context"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type baseHealthSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor os.Descriptor
}

// section contains the content status of a specific section (e.g. Forwarder)
func (v *baseHealthSuite) TestDefaultInstallHealthy() {
	interval := 1 * time.Second

	var output string
	var err error
	output, err = backoff.Retry(context.Background(), func() (string, error) {
		out, err := v.Env().Agent.Client.Health()
		if err != nil {
			return "", err
		}
		return out, nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(interval)), backoff.WithMaxTries(15))

	assert.NoError(v.T(), err)
	assert.Contains(v.T(), output, "Agent health: PASS")
}

func (v *baseHealthSuite) TestDefaultInstallUnhealthy() {
	// restart the agent, which validates the key using the fakeintake at startup
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
			ec2.WithAgentOptions(agentparams.WithAgentConfig("log_level: info\nforwarder_apikey_validation_interval: 1")),
		),
	))

	// the fakeintake says that any API key is invalid by sending a 403 code
	override := api.ResponseOverride{
		Endpoint:   "/api/v1/validate",
		StatusCode: 403,
		Method:     http.MethodGet,
		Body:       []byte("invalid API key"),
	}

	err := v.Env().FakeIntake.Client().ConfigureOverride(override)
	require.NoError(v.T(), err)

	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		// forwarder should become unhealthy when next checking because the fakeintake will return 403
		_, err = v.Env().Agent.Client.Health()
		assert.ErrorContains(collect, err, "Agent health: FAIL")
		assert.ErrorContains(collect, err, "=== 1 unhealthy components ===\nforwarder")
	}, 2*time.Minute, 10*time.Second)

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
