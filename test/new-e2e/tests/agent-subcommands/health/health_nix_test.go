// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type linuxHealthSuite struct {
	baseHealthSuite
}

func TestLinuxHealthSuite(t *testing.T) {
	e2e.Run(t, &linuxHealthSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *linuxHealthSuite) TestDefaultInstallUnhealthy() {
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
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info\n")),
	))

	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		// forwarder should be unhealthy because the key is invalid
		_, err = v.Env().Agent.Client.Health()
		assert.ErrorContains(collect, err, "Agent health: FAIL")
		assert.ErrorContains(collect, err, "=== 1 unhealthy components ===\nforwarder")
	}, time.Second*30, time.Second)
}
