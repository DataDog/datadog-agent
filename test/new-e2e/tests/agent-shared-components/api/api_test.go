// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
)

const (
	agentCmdPort = 5001
)

type apiSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestApiSuite(t *testing.T) {
	e2e.Run(t, &apiSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

type agentEndpointInfo struct {
	scheme   string
	port     int
	endpoint string
	method   string
	data     string
}

func (endpointInfo *agentEndpointInfo) url() *url.URL {
	return &url.URL{
		Scheme: endpointInfo.scheme,
		Host:   net.JoinHostPort("localhost", strconv.Itoa(endpointInfo.port)),
		Path:   endpointInfo.endpoint,
	}
}

func (endpointInfo *agentEndpointInfo) httpRequest(authtoken string) (*http.Request, error) {
	payload := bytes.NewBufferString(endpointInfo.data)

	req, err := http.NewRequest(endpointInfo.method, endpointInfo.url().String(), payload)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authtoken))
	return req, nil
}

func (v *apiSuite) TestDefaultAgentAPIEndpoints() {
	testcases := []struct {
		agentEndpointInfo
		name         string
		expectedCode int
		assert       func(*assert.CollectT, agentEndpointInfo, *http.Response)
	}{
		{
			name: "version",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/version",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Version struct {
					Major int
				}
				var have Version

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				want := Version{Major: 7}
				assert.Equal(ct, have, want, "%s %s returned: %s, wanted: %v", e.method, e.endpoint, body, want)
			},
		},
		{
			name: "hostname",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/hostname",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				want := v.Env().Agent.Client.Hostname()

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				assert.Contains(ct, string(body), want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, string(body), want)
			},
		},
		{
			name: "health",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/status/health",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type HealthCheck struct {
					Healthy []string
				}
				var have HealthCheck

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Healthy, "%s %s returned: %s, expected Healthy not to be empty", e.method, e.endpoint, body)
			},
		},
		{
			name: "config",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/config",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				// this returns text output
				want := `api_key: '*******`

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				assert.Contains(ct, string(body), want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, string(body), want)
			},
		},
		{
			name: "config list-runtime",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/config/list-runtime",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Config struct {
					DogstatsdCaptureDuration interface{} `json:"dogstatsd_capture_duration"`
				}
				var have Config

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.DogstatsdCaptureDuration, "%s %s returned: %s, expected dogstatsd_capture_duration key", e.method, e.endpoint, body)
			},
		},
		{
			name: "jmx configs",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/jmx/configs",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			// TODO: can we set up a jmx environment
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Config struct {
					JMXConfig interface{} `json:"configs"`
				}
				var have Config

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.Equal(ct, have.JMXConfig, make(map[string]interface{}), "%s %s returned: %s, expected jmx configs to be empty", e.method, e.endpoint, body)
			},
		},
		{
			name: "config check",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/config-check",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Config struct {
					Checks []interface{} `json:"configs"`
				}
				var have Config

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Checks, "%s %s returned: %s, expected \"configs\" checks to be present", e.method, e.endpoint, body)
			},
		},
		{
			name: "flare",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/flare",
				method:   "POST",
				data:     "{}",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				want := `/tmp/datadog-agent-`

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				assert.Contains(ct, string(body), want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, string(body), want)
			},
		},
		{
			name: "secrets",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/secrets",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				want := `secrets feature is not enabled`

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				assert.Contains(ct, string(body), want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, string(body), want)
			},
		},
		{
			name: "tagger",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/tagger-list",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			// TODO: there isn't a configuration to enable this, it needs a dedicated environment setup
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Tagger struct {
					Entities interface{} `json:"entities"`
				}
				var have Tagger

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.Equal(ct, have.Entities, make(map[string]interface{}), "%s %s returned: %s, expected entities to be empty", e.method, e.endpoint, body)
			},
		},
		{
			name: "workloadmeta",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/workload-list",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Workload struct {
					Entities interface{} `json:"entities"`
				}
				var have Workload

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.Equal(ct, have.Entities, make(map[string]interface{}), "%s %s returned: %s, expected entities to be empty", e.method, e.endpoint, body)
			},
		},
		{
			name: "metadata gohai",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/gohai",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Metadata struct {
					Gohai interface{} `json:"gohai"`
				}
				var have Metadata

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Gohai, "%s %s returned: %s, expected \"gohai\" fields to be present", e.method, e.endpoint, body)
			},
		},
		{
			name: "metadata v5",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/v5",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Metadata struct {
					Version string `json:"agentVersion"`
				}
				var have Metadata

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Version, "%s %s returned: %s, expected \"agentVersion\" to be present", e.method, e.endpoint, body)
			},
		},
		{
			name: "metadata inventory-check",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-checks",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Metadata struct {
					Check interface{} `json:"check_metadata"`
				}
				var have Metadata

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Check, "%s %s returned: %s, expected \"check_metadata\" fields to be present", e.method, e.endpoint, body)
			},
		},
		{
			name: "metadata inventory-agent",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-agent",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Metadata struct {
					Agent interface{} `json:"agent_metadata"`
				}
				var have Metadata

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Agent, "%s %s returned: %s, expected \"agent_metadata\" fields to be present", e.method, e.endpoint, body)
			},
		},
		{
			name: "metadata inventory-host",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-host",
				method:   "GET",
				data:     "",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Metadata struct {
					Host interface{} `json:"host_metadata"`
				}
				var have Metadata

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotEmpty(ct, have.Host, "%s %s returned: %s, expected \"host_metadata\" fields to be present", e.method, e.endpoint, body)
			},
		},
		// TODO: figure out how to make this work
		// {
		// 	name: "dogstatsd context dump",
		// 	agentEndpointInfo: agentEndpointInfo{
		// 		scheme:   "https",
		// 		port:     agentCmdPort,
		// 		endpoint: "/agent/dogstatsd-context-dump",
		// 		method:   "POST",
		// 		data:     "{}",
		// 	},
		// 	assert: `dogstatsd_contexts.json.zstd`,
		// },
		{
			name: "diagnose",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/diagnose",
				method:   "POST",
				data:     "{}",
			},
			expectedCode: 200,
			assert: func(ct *assert.CollectT, e agentEndpointInfo, resp *http.Response) {
				type Diagnoses struct {
					SuiteName string `json:"suite_name"`
				}

				// DiagnoseResult contains the results of the diagnose command
				type DiagnoseResult struct {
					Diagnoses []Diagnoses `json:"runs"`
				}

				var have DiagnoseResult

				body, err := io.ReadAll(resp.Body)
				assert.NoError(ct, err, "failed to read body from request")

				err = json.Unmarshal(body, &have)
				assert.NoError(ct, err)
				assert.NotNil(ct, have)
				assert.NotZero(ct, len(have.Diagnoses), "%s %s returned: %s, expected diagnose suites to be present", e.method, e.endpoint, body)
			},
		},
	}

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	hostHTTPClient := v.Env().RemoteHost.NewHTTPClient()
	for _, testcase := range testcases {
		v.T().Run(fmt.Sprintf("API - %s test", testcase.name), func(t *testing.T) {
			req, err := testcase.httpRequest(authtoken)
			assert.NoError(t, err, "failed to create request")

			require.EventuallyWithT(t, func(ct *assert.CollectT) {
				resp, err := hostHTTPClient.Do(req)
				assert.NoError(ct, err, "failed to send request")
				defer resp.Body.Close()

				endpoint := testcase.agentEndpointInfo
				assert.Equal(ct, testcase.expectedCode, resp.StatusCode, "%s %s returned: %s, expected %s", endpoint.method, endpoint.endpoint, resp.StatusCode, testcase.expectedCode)
				testcase.assert(ct, endpoint, resp)
			}, 2*time.Minute, 10*time.Second)
		})
	}
}

func (v *apiSuite) TestSecretsAgentAPIEndpoints() {
	e := agentEndpointInfo{
		scheme:   "https",
		port:     agentCmdPort,
		endpoint: "/agent/secrets",
		method:   "GET",
		data:     "",
	}
	want := `used in 'datadog.yaml' configuration in entry 'hostname'`

	config := `secret_backend_command: /tmp/test-secret-api-endpoint/secret-resolver.py
secret_backend_arguments:
  - /tmp/test-secret-api-endpoint
secret_backend_remove_trailing_line_break: true
secret_backend_command_allow_group_exec_perm: true

log_level: debug
hostname: ENC[hostname]`

	rootDir := "/tmp/test-secret-api-endpoint"
	v.Env().RemoteHost.MkdirAll(rootDir)
	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secretsutils.NewSecretClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("hostname", "e2e.test")

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			secretsutils.WithUnixSecretSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(config),
		),
	))

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	req, err := e.httpRequest(authtoken)
	assert.NoError(v.T(), err, "failed to create request")
	host := v.Env().RemoteHost
	hostHTTPClient := host.NewHTTPClient()

	require.EventuallyWithT(v.T(), func(ct *assert.CollectT) {
		resp, err := hostHTTPClient.Do(req)
		assert.NoError(ct, err, "failed to send request")

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		assert.NoError(ct, err, "failed to read body from request")

		assert.Contains(ct, string(body), want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, body, want)
	}, 2*time.Minute, 10*time.Second)
}

func (v *apiSuite) TestWorkloadMetaAgentAPIEndpoint() {
	e := agentEndpointInfo{
		scheme:   "https",
		port:     agentCmdPort,
		endpoint: "/agent/workload-list",
		method:   "GET",
		data:     "",
	}

	config := `process_config:
  process_collection:
    enabled: true
  intervals:
    process: 1

language_detection:
  enabled: true

log_level: debug
`
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(config),
		),
	))

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	req, err := e.httpRequest(authtoken)
	assert.NoError(v.T(), err, "failed to create request")
	host := v.Env().RemoteHost
	hostHTTPClient := host.NewHTTPClient()

	require.EventuallyWithT(v.T(), func(ct *assert.CollectT) {
		type Workload struct {
			Entities interface{} `json:"entities"`
		}

		var have Workload
		resp, err := hostHTTPClient.Do(req)
		assert.NoError(ct, err, "failed to send request")

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		assert.NoError(ct, err, "failed to read body from request")

		err = json.Unmarshal(body, &have)
		assert.NoError(ct, err)

		assert.NotEmpty(ct, have.Entities, "%s %s returned: %s, expected workload entities to be present", e.method, e.endpoint, body)
	}, 2*time.Minute, 10*time.Second)
}
