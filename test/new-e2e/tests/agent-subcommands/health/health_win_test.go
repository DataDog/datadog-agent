// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type windowsHealthSuite struct {
	baseHealthSuite
}

func TestWindowsHealthSuite(t *testing.T) {
	e2e.Run(t, &windowsHealthSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *windowsHealthSuite) TestDefaultInstallUnhealthy() {
	// the fakeintake says that any API key is invalid by sending a 403 code
	override := api.ResponseOverride{
		Endpoint:    "/api/v1/validate",
		StatusCode:  403,
		ContentType: "text/plain",
		Body:        []byte("invalid API key"),
	}
	v.Env().FakeIntake.Client().ConfigureOverride(override)
	// restart the agent, which validates the key using the fakeintake at startup
	v.UpdateEnv(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info\n"))))

	// agent should be unhealthy because the key is invalid

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		_, err := v.Env().Agent.Client.Health()
		assert.ErrorContains(t, err, "Agent health: FAIL")
	}, 1*time.Minute, 10*time.Second)
}
