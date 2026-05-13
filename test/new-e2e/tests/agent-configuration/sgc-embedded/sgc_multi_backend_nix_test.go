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

//go:embed fixtures/secrets.yaml
var embeddedSecretYAML string

//go:embed fixtures/secrets.json
var embeddedSecretJSON string

type linuxRuntimeSecretSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSGCMultiBackendLinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// TestMultiBackend verifies that multi_secret_backends routes ENC[backendID;key] handles
// to the correct named backend for resolution.
func (v *linuxRuntimeSecretSuite) TestMultiBackend() {
	config := `api_key: ENC[yaml;fake_yaml_key]
multi_secret_backends:
  yaml:
    type: file.yaml
    config:
      file_path: /tmp/secrets.yaml
  json:
    type: file.json
    config:
      file_path: /tmp/secrets.json
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[json;fake_json_key]`

	unixPermission := perms.NewUnixPermissions(
		perms.WithPermissions("0400"),
		perms.WithOwner("dd-agent"),
		perms.WithGroup("dd-agent"),
	)
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithFileWithPermissions("/tmp/secrets.yaml", embeddedSecretYAML, true, unixPermission),
			agentparams.WithFileWithPermissions("/tmp/secrets.json", embeddedSecretJSON, true, unixPermission),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		)),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		require.Contains(t, secretOutput, "yaml;fake_yaml_key")
		require.Contains(t, secretOutput, "json;fake_json_key")
	}, 30*time.Second, 2*time.Second, "could not verify all secrets are resolved within the allotted time")
}

// dummyScript is a secret backend command that resolves every handle to a fixed value.
// Used to verify that secret_backend_command takes precedence over multi_secret_backends:
// handles are sent as-is to the command without backendID routing. The multi_secret_backends
// backends reference non-existent files; if routing were incorrectly applied the backends
// would fail and the agent would not start — so agent startup proves the command won.
const dummyScript = `#!/usr/bin/env python3
import json, sys
payload = json.load(sys.stdin)
print(json.dumps({h: {"value": "00000000000000000000000000000000"} for h in payload["secrets"]}))
`

// TestSecretBackendCommandOverridesMulti verifies that when secret_backend_command is set
// alongside multi_secret_backends, the command wins: handles (including ones with the
// "backendID;" prefix) are sent as-is to the command without routing. The multi_secret_backends
// entries reference non-existent files; correct behaviour (command wins) lets the agent start
// and resolve all handles, while incorrect behaviour (routing applied) would send handles to
// the file backends which would fail, preventing agent startup.
func (v *linuxRuntimeSecretSuite) TestSecretBackendCommandOverridesMulti() {
	config := `api_key: ENC[yaml;api_key_secret]
secret_backend_command: /etc/datadog-agent/dummy_secret_script.py
multi_secret_backends:
  yaml:
    type: file.yaml
    config:
      file_path: /tmp/does-not-exist.yaml
  json:
    type: file.json
    config:
      file_path: /tmp/does-not-exist.json
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[yaml;fake_yaml_key]
  - ENC[json;fake_json_key]`

	scriptPermission := perms.NewUnixPermissions(
		perms.WithPermissions("0700"),
		perms.WithOwner("dd-agent"),
		perms.WithGroup("dd-agent"),
	)
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithFileWithPermissions("/etc/datadog-agent/dummy_secret_script.py", dummyScript, true, scriptPermission),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithAgentConfig(config),
		)),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		secretOutput := v.Env().Agent.Client.Secret()
		// Handles are forwarded as-is to the command (no routing). If routing had been
		// applied the file backends would fail (files don't exist) and the agent would
		// not have started — reaching this assertion proves the command won.
		require.Contains(t, secretOutput, "yaml;fake_yaml_key")
		require.Contains(t, secretOutput, "json;fake_json_key")
	}, 30*time.Second, 2*time.Second, "secret_backend_command should resolve prefixed handles without backendID routing")
}

// TestSecretBackendTypeOverridesMulti verifies that when secret_backend_type is set, every
// ENC[...] inner string is looked up as a single secret key on that backend — including
// substrings that look like multi-backend prefixes (e.g. "yaml;..."). multi_secret_backends
// entries are ignored for routing but may still be present in config.
func (v *linuxRuntimeSecretSuite) TestSecretBackendTypeOverridesMulti() {
	config := `api_key: ENC[api_key_secret]
secret_backend_type: file.yaml
secret_backend_config:
  file_path: /tmp/secrets_merged.yaml
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[yaml;fake_yaml_key]
  - ENC[json;fake_json_key]
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
		require.Contains(t, secretOutput, "api_key_secret")
		require.Contains(t, secretOutput, "yaml;fake_yaml_key")
		require.Contains(t, secretOutput, "json;fake_json_key")
	}, 30*time.Second, 2*time.Second, "could not verify all secrets are resolved within the allotted time")
}
