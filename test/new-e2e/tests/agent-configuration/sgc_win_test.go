// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type sgcWindowsRuntimeSuite struct {
	sgcBaseRuntimeSuite
}

func TestSGCWindowsRuntimeSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &sgcWindowsRuntimeSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))),
	)))
}

func (v *sgcWindowsRuntimeSuite) TestPullSecret() {
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
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithFileWithPermissions("C:/TestFolder/secrets.yaml", embeddedSecretFile, true, windowsPermission),
					agentparams.WithAgentConfig(config),
					agentparams.WithSkipAPIKeyInConfig(),
				),
			),
		),
	)

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "fake_yaml_key")
	}, 60*time.Second, 2*time.Second, "could not check if secretOutput contains 'fake_yaml_key' within the allotted time")
}
