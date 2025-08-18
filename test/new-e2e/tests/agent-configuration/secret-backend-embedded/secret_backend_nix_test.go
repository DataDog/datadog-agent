// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secretbackend contains e2e tests for secret management
package secretbackend

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type linuxRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

//go:embed fixtures/secrets.yaml
var secretScript string

func TestLinuxRuntimeSecretSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *linuxRuntimeSecretSuite) TestPullSecret() {
	config := `api_key: ENC[fake_api_key]
secret_backend_type: file.yaml
secret_backend_config:
  file_path: /tmp/secrets.yaml`

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			secretsutils.WithUnixSetupCustomScript("/tmp/secrets.yaml", secretScript, false),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret("fake_api_key")
		assert.Equal(t, "fake-api-key", secretOutput)
	}, 30*time.Second, 2*time.Second)
}
