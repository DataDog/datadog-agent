// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	_ "embed"
	"encoding/json"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

//go:embed secret/fixtures/metadata-configs.yaml
var metadataConfigsYAML []byte

//go:embed secret/fixtures/metadata-secrets.py
var metadataBackendScript string

type metadataSecretReference struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Handle string `json:"handle,omitempty"`
}

func (v *secretLinuxRuntimeSuite) TestSecretMetadataFiles() {
	t := v.T()
	const configDir = "/etc/datadog-agent/"
	expected := map[string][]metadataSecretReference{
		"datadog.yaml": {
			{"api_key", "enc", "metadata_core"},
			{"apm_config.debugger_api_key", "raw", ""},
		},
		"system-probe.yaml":           {{"system_probe_config.internal_profiling.api_key", "raw", ""}},
		"security-agent.yaml":         {{"security_agent.internal_profiling.api_key", "enc", "metadata_security"}},
		"application_monitoring.yaml": {{"api_key", "raw", ""}},
		"conf.d/http_check.d/fa-secrets.yaml": {
			{"instances.0.password", "enc", "metadata_http_shared"},
			{"instances.0.headers.Authorization", "raw", ""},
		},
		"conf.d/http_check.d/fa-secrets-second.yaml": {{"instances.0.password", "enc", "metadata_http_shared"}},
	}
	// Restore provisioned configuration and remove unmanaged files for suite retries.
	t.Cleanup(func() {
		_, err := v.Env().RemoteHost.Execute("sudo rm -f /etc/datadog-agent/conf.d/http_check.d/fa-secrets.yaml /etc/datadog-agent/conf.d/http_check.d/fa-secrets-second.yaml /etc/datadog-agent/application_monitoring.yaml")
		assert.NoError(t, err)
		v.UpdateEnv(awshost.Provisioner())
	})

	var configs map[string]string
	require.NoError(t, yaml.Unmarshal(metadataConfigsYAML, &configs))
	values := map[string]string{
		"metadata_core":        "0123456789abcdef0123456789abcdef",
		"metadata_security":    "fedcba9876543210fedcba9876543210",
		"metadata_http_shared": "fixture-resolved-http-secret",
	}
	// Non-hex raw values exercise key-based scrubbing, not just API-key detection.
	plaintexts := []string{"fixture-raw-secret-value"}
	for _, value := range values {
		plaintexts = append(plaintexts, value)
	}
	valueJSON, err := json.Marshal(values)
	require.NoError(t, err)
	opts := []agentparams.Option{
		agentparams.WithSkipAPIKeyInConfig(),
		secretsutils.WithUnixSetupCustomScript("/tmp/fa-metadata-secrets.py", metadataBackendScript, false),
		agentparams.WithFile("/tmp/fa-metadata-values.json", string(valueJSON), true),
	}
	for file, config := range configs {
		switch file {
		case "datadog.yaml":
			opts = append(opts, agentparams.WithAgentConfig(config))
		case "system-probe.yaml":
			opts = append(opts, agentparams.WithSystemProbeConfig(config))
		case "security-agent.yaml":
			opts = append(opts, agentparams.WithSecurityAgentConfig(config))
		default:
			opts = append(opts, agentparams.WithFile(configDir+file, config, true))
		}
	}
	v.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(opts...))))
	require.NoError(t, v.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	v.EventuallyWithT(func(c *assert.CollectT) {
		requests, err := v.Env().FakeIntake.Client().GetRawPayloads("/api/v1/metadata")
		require.NoError(c, err)
		seen := map[string]bool{}
		for _, request := range requests {
			data, err := aggregator.Inflate(request.Data, request.Encoding)
			require.NoError(c, err)
			var payload struct {
				Files map[string]struct {
					Secrets []metadataSecretReference `json:"secrets"`
				} `json:"files_metadata"`
			}
			require.NoError(c, json.Unmarshal(data, &payload))
			for _, value := range plaintexts {
				assert.NotContains(c, string(data), value)
			}
			for file, references := range expected {
				if metadata, found := payload.Files[configDir+file]; found {
					assert.ElementsMatch(c, references, metadata.Secrets, "secret references in %s", file)
					seen[file] = true
				}
			}
		}
		assert.Len(c, seen, len(expected), "not all configuration files reached intake")
	}, 2*time.Minute, 5*time.Second)
}
