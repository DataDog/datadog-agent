// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sgcbackend contains e2e tests for secret-generic-connector multi-backend scenarios.
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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/secrets_merged.yaml
var embeddedSecretMergedYAML string

type linuxRuntimeSecretSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSGCMultiBackendLinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// TestMultiBackend verifies that when secret_backend_type is set, every ENC[...] inner string is
// looked up as a single secret key on that backend (including substrings that look like multi-backend
// prefixes, e.g. "yaml::..."). multi_secret_backends entries are ignored for routing but may still
// be present in config (and should trigger a warning in agent logs).
func (v *linuxRuntimeSecretSuite) TestMultiBackend() {
	config := `api_key: ENC[embedded-secret;embedded_secret_key]
secret_backend_type: file.yaml
secret_backend_config:
  file_path: /tmp/secrets_merged.yaml
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[yaml::fake_yaml_key]
  - ENC[json::fake_json_key]
multi_secret_backends:
  yaml:
    type: file.yaml
    config:
      file_path: /tmp/does-not-exist.yaml
  json:
    type: file.json
    config:
      file_path: /tmp/does-not-exist.json`

	unixPermission := perms.NewUnixPermissions(
		perms.WithPermissions("0400"),
		perms.WithOwner("dd-agent"),
		perms.WithGroup("dd-agent"),
	)
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithFileWithPermissions("/tmp/secrets_merged.yaml", embeddedSecretMergedYAML, true, unixPermission),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		)),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "embedded-secret;embedded_secret_key")
		require.Contains(t, secretOutput, "fake_yaml_key")
		require.Contains(t, secretOutput, "fake_json_key")
	}, 30*time.Second, 2*time.Second, "could not verify all secrets are resolved within the allotted time")
}
