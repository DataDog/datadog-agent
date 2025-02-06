// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type linuxAPIKeyRefreshSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor os.Descriptor
}

func TestLinuxAPIKeyFreshSuite(t *testing.T) {
	suite := &linuxAPIKeyRefreshSuite{descriptor: os.UbuntuDefault}
	fakeIntakeURL := "http://public.ecr.aws/datadog/fakeintake:v6aaaa439"
	e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(suite.descriptor)),
		awshost.WithFakeIntakeOptions(
			fakeintake.WithImageURL(fakeIntakeURL),
		),
	)))
}

func (v *linuxAPIKeyRefreshSuite) TestIntakeRefreshAPIKey() {
	// the fakeintake says that any API key is invalid by sending a 403 code
	override := api.ResponseOverride{
		Endpoint:   "/api/v1/validate",
		StatusCode: 403,
		Method:     http.MethodGet,
		Body:       []byte("invalid API key"),
	}
	err := v.Env().FakeIntake.Client().ConfigureOverride(override)
	require.NoError(v.T(), err)

	config := `secret_backend_command: /tmp/secret.py
secret_backend_arguments:
  - /tmp
api_key: ENC[api_key]
`

	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, "/tmp")
	secretClient.SetSecret("api_key", "abcdefghijklmnopqrstuvwxyz123456")

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				secrets.WithUnixSecretSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
		),
	)

	status := v.Env().Agent.Client.Status()
	assert.Contains(v.T(), status.Content, "API key ending with 23456")

	secretClient.SetSecret("api_key", "123456abcdefghijklmnopqrstuvwxyz")

	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	status = v.Env().Agent.Client.Status()
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assert.Contains(t, status.Content, "API key ending with vwxyz")
	}, 1*time.Minute, 10*time.Second)

	lastAPIKey, err := v.Env().FakeIntake.Client().GetLastAPIKey()
	fmt.Printf("did we get an error: %v\n", err)
	fmt.Printf("did we get api Key: %v\n", lastAPIKey)
	v.T().Logf("%s", fmt.Sprintf("did we get an error: %v\n", err))
	v.T().Logf("%s", fmt.Sprintf("did we get api Key: %v\n", lastAPIKey))
	v.T().Logf("Test Completed!!!!")
}
