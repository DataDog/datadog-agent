// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package sgcbackend contains e2e tests for secret management
package sgcbackend

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type linuxRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

//go:embed fixtures/secrets.yaml
var embeddedSecretFile string

func TestLinuxRuntimeSecretSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *linuxRuntimeSecretSuite) TestPullSecret() {
	config := `api_key: ENC[fake_yaml_key]
secret_backend_type: file.yaml
secret_backend_config:
  file_path: /tmp/secrets.yaml`

	unixPermission := perms.NewUnixPermissions(perms.WithPermissions("0400"), perms.WithOwner("dd-agent"), perms.WithGroup("dd-agent"))
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithFileWithPermissions("/tmp/secrets.yaml", embeddedSecretFile, true, unixPermission),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		)),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "fake_yaml_key")
	}, 30*time.Second, 2*time.Second, "could not check if secretOutput contains 'fake_yaml_key' within the allotted time")
}
