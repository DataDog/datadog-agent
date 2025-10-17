// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secretbackend contains e2e tests for secret management
package secretbackend

import (
	_ "embed"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

func (v *linuxRuntimeSecretSuite) TestPullAWSSecret() {
	config := `api_key: ENC[embedded-secret;embedded_secret_key]
secret_backend_type: aws.secrets
secret_backend_config:
  aws_session:
    aws_region: us-east-1`

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "embedded_secret_key")
	}, 30*time.Second, 2*time.Second, "could not check if secretOutput contains 'embedded_secret_key' within the allotted time")
}
