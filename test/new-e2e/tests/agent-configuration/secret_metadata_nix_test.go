// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"
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

//go:embed secret/fixtures/metadata-configs.json
var metadataConfigsJSON []byte

//go:embed secret/fixtures/metadata-secrets.py
var metadataBackendScript string

type metadataSecretCase struct {
	file string
	path []any
}

var metadataSecretCases = []metadataSecretCase{
	{"datadog.yaml", []any{"api_key"}},
	{"datadog.yaml", []any{"apm_config", "debugger_api_key"}},
	{"datadog.yaml", []any{"logs_config", "api_key"}},
	{"datadog.yaml", []any{"proxy", "http"}},
	{"system-probe.yaml", []any{"system_probe_config", "internal_profiling", "api_key"}},
	{"security-agent.yaml", []any{"security_agent", "internal_profiling", "api_key"}},
	{"conf.d/http_check.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/http_check.d/fa-secrets.yaml", []any{"instances", 0, "headers", "Authorization"}},
	{"conf.d/postgres.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/mysql.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/redisdb.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/rabbitmq.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/mongo.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/elastic.d/fa-secrets.yaml", []any{"instances", 0, "password"}},
	{"conf.d/openmetrics.d/fa-secrets.yaml", []any{"instances", 0, "headers", "Authorization"}},
	{"conf.d/http_check.d/fa-secrets.yaml", []any{"instances", 1, "password"}},
	{"conf.d/http_check.d/fa-secrets-second.yaml", []any{"instances", 0, "password"}},
}

// TestSecretMetadataFiles exercises file loading, the assembled metadata providers,
// scrubbing and forwarding. Assertions use actual intake requests, not a CLI-built payload.
func (v *secretLinuxRuntimeSuite) TestSecretMetadataFiles() {
	// Restore the provisioner's normal config and remove unmanaged integration
	// files so this method can be retried on the same host.
	v.T().Cleanup(func() {
		for _, integration := range []string{"http_check", "postgres", "mysql", "redisdb", "rabbitmq", "mongo", "elastic", "openmetrics"} {
			_, err := v.Env().RemoteHost.Execute("sudo rm -f /etc/datadog-agent/conf.d/" + integration + ".d/fa-secrets.yaml /etc/datadog-agent/conf.d/" + integration + ".d/fa-secrets-second.yaml")
			assert.NoError(v.T(), err)
		}
		v.UpdateEnv(awshost.Provisioner())
	})
	for _, kind := range []string{"raw", "enc"} {
		v.T().Run(kind, func(t *testing.T) {
			var configs map[string]map[string]any
			require.NoError(t, json.Unmarshal(metadataConfigsJSON, &configs))
			values := map[string]string{}
			plaintexts := make([]string, 0, len(metadataSecretCases))
			for i, entry := range metadataSecretCases {
				handle := fmt.Sprintf("metadata_%d", i)
				value := fmt.Sprintf("%032d", i+1)
				plaintexts = append(plaintexts, value)
				if i == 3 {
					value = "http://fixture:" + value + "@127.0.0.1:9"
				}
				if i == 7 {
					value = "Bearer " + value
				}
				values[handle] = value
				configured := value
				if kind == "enc" {
					configured = "ENC[" + handle + "]"
				}
				setMetadataFixtureValue(configs["/etc/datadog-agent/"+entry.file], entry.path, configured)
			}
			core := configs["/etc/datadog-agent/datadog.yaml"]
			core["secret_backend_command"] = "/tmp/fa-metadata-secrets.py"
			opts := []agentparams.Option{agentparams.WithSkipAPIKeyInConfig(), secretsutils.WithUnixSetupCustomScript("/tmp/fa-metadata-secrets.py", metadataBackendScript, false)}
			valueJSON, err := json.Marshal(values)
			require.NoError(t, err)
			opts = append(opts, agentparams.WithFile("/tmp/fa-metadata-values.json", string(valueJSON), true))
			for file, config := range configs {
				encoded, err := yaml.Marshal(config)
				require.NoError(t, err)
				switch file {
				case "/etc/datadog-agent/datadog.yaml":
					opts = append(opts, agentparams.WithAgentConfig(string(encoded)))
				case "/etc/datadog-agent/system-probe.yaml":
					opts = append(opts, agentparams.WithSystemProbeConfig(string(encoded)))
				case "/etc/datadog-agent/security-agent.yaml":
					opts = append(opts, agentparams.WithSecurityAgentConfig(string(encoded)))
				default:
					opts = append(opts, agentparams.WithFile(file, string(encoded), true))
				}
			}
			v.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(opts...))))
			require.NoError(t, v.Env().FakeIntake.Client().FlushServerAndResetAggregators())
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				requests, err := v.Env().FakeIntake.Client().GetRawPayloads("/api/v1/metadata")
				require.NoError(c, err)
				seen := map[string]bool{}
				for _, request := range requests {
					data, err := aggregator.Inflate(request.Data, request.Encoding)
					require.NoError(c, err)
					var payload struct {
						Files map[string]struct {
							Secrets []struct {
								Path   []any  `json:"path"`
								Kind   string `json:"kind"`
								Handle string `json:"handle"`
							} `json:"secrets"`
						} `json:"files_metadata"`
					}
					require.NoError(c, json.Unmarshal(data, &payload))
					for _, value := range plaintexts {
						assert.NotContains(c, string(data), value)
					}
					for i, entry := range metadataSecretCases {
						expectedPath, err := json.Marshal(entry.path)
						require.NoError(c, err)
						for _, secret := range payload.Files["/etc/datadog-agent/"+entry.file].Secrets {
							actualPath, err := json.Marshal(secret.Path)
							require.NoError(c, err)
							if string(actualPath) == string(expectedPath) && secret.Kind == kind {
								if kind == "enc" {
									assert.Equal(c, fmt.Sprintf("metadata_%d", i), secret.Handle)
								}
								seen[entry.file+string(expectedPath)] = true
							}
						}
					}
				}
				assert.Len(c, seen, len(metadataSecretCases), "not all file-secret declarations reached intake")
			}, 2*time.Minute, 5*time.Second)
		})
	}
}

func setMetadataFixtureValue(config map[string]any, path []any, value string) {
	var current any = config
	for i, segment := range path {
		switch node := current.(type) {
		case map[string]any:
			key := segment.(string)
			if i == len(path)-1 {
				node[key] = value
				return
			}
			current = node[key]
		case []any:
			current = node[segment.(int)]
		}
	}
	panic("invalid fixture path: " + fmt.Sprint(path))
}
