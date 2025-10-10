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
	perms "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/filepermissions"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type windowsRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

func TestWindowsRuntimeSecretSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
	)))
}

func (v *windowsRuntimeSecretSuite) TestPullSecret() {
	config := `api_key: ENC[fake_yaml_key]
secret_backend_type: file.yaml
secret_backend_config:
  file_path: C:/TestFolder/secrets.yaml`

	windowsPermission := perms.NewWindowsPermissions(
		perms.WithIcaclsCommand(`/grant "ddagentuser:(RX)"`),
		perms.WithDisableInheritance(),
	)

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			awshost.WithAgentOptions(
				agentparams.WithFileWithPermissions("C:/TestFolder/secrets.yaml", embeddedSecretFile, true, windowsPermission),
				agentparams.WithAgentConfig(config),
				agentparams.WithSkipAPIKeyInConfig(),
			),
		),
	)

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "fake_yaml_key")
	}, 30*time.Second, 2*time.Second)
}
