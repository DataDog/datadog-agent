// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/secrets"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	agentCmdPort = 5001
)

type apiSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestApiSuite(t *testing.T) {
	e2e.Run(t, &apiSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()), e2e.WithDevMode())
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

func (endpointInfo *agentEndpointInfo) fetchCommand(authtoken string) string {
	data := endpointInfo.data
	if len(endpointInfo.data) == 0 {
		data = "{}"
	}

	// -s: silent so we don't show auth token in output
	// -k: allow insecure server connections since we self-sign the TLS cert
	// -H: add a header with the auth token
	// -X: http request method
	// -d: request data (json)
	return fmt.Sprintf(
		`curl -s -k -H "authorization: Bearer %s" -X %s "%s" -d "%s"`,
		authtoken,
		endpointInfo.method,
		endpointInfo.url().String(),
		data,
	)
}

func (v *apiSuite) TestDefaultAgentAPIEndpoints() {
	testcases := []struct {
		agentEndpointInfo
		name string
		want string
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
			// TODO: json parse this to a better comparison
			want: `"Major":7,"Minor":5`,
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
			// ec2 instance id's start with i-
			want: `i-`,
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
			// TODO: json parse this to a better comparison
			want: `{"Healthy":[`,
		},
		// TODO: we could possibly do a regexp comparison
		// {
		// 	name: "csrf-token",
		// 	agentEndpointInfo: agentEndpointInfo{
		// 		scheme:   "https",
		// 		port:     agentCmdPort,
		// 		endpoint: "/agent/gui/csrf-token",
		// 		method:   "GET",
		// 		data:     "",
		// 	},
		// 	want: ``,
		// },
		{
			name: "config",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/config",
				method:   "GET",
				data:     "",
			},
			// TODO: find a better setting to compare output to
			want: `api_key: '*******`,
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
			// TODO: find a better setting to compare output to
			want: `dogstatsd_capture_duration`,
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
			// TODO: can we set up a jmx environment
			want: `{"configs":{}`,
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
			// TODO: json parse this to a better comparison
			want: `{"configs":[{"check_name":`,
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
			want: `/tmp/datadog-agent-`,
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
			want: `secrets feature is not enabled`,
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
			// TODO: there isn't a configuration to enable this, it needs a dedicated environment setup
			want: `{"entities":{}}`,
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
			want: `{"entities":{}}`,
		},
		{
			name: "metadata gohai",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/gohai",
				method:   "GET",
				data:     "{}",
			},
			// TODO: json parse this output for a better comparison
			want: `{
  "gohai": {
    "cpu": {`,
		},
		{
			name: "metadata v5",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/v5",
				method:   "GET",
				data:     "{}",
			},
			// TODO: json parse this output for a better comparison
			want: `"agentVersion": "7.5`,
		},
		{
			name: "metadata inventory-check",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-checks",
				method:   "GET",
				data:     "{}",
			},
			// TODO: json parse this output for a better comparison
			want: `"check_metadata": {`,
		},
		{
			name: "metadata inventory-agent",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-agent",
				method:   "GET",
				data:     "{}",
			},
			// TODO: json parse this output for a better comparison
			want: `"agent_metadata": {`,
		},
		{
			name: "metadata inventory-host",
			agentEndpointInfo: agentEndpointInfo{
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/metadata/inventory-host",
				method:   "GET",
				data:     "{}",
			},
			// TODO: json parse this output for a better comparison
			want: `"host_metadata": {`,
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
		// 	want: `dogstatsd_contexts.json.zstd`,
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
			want: `{"SuiteName":"connectivity-datadog-autodiscovery","SuiteDiagnoses":[{`,
		},
	}

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	for _, tc := range testcases {
		cmd := tc.fetchCommand(authtoken)
		host := v.Env().RemoteHost
		v.T().Run(fmt.Sprintf("API - %s test", tc.name), func(t *testing.T) {
			require.EventuallyWithT(t, func(ct *assert.CollectT) {
				resp, err := host.Execute(cmd)
				require.NoError(ct, err)
				assert.Contains(ct, resp, tc.want, "%s %s returned: %s, wanted match: %s", tc.method, tc.endpoint, resp, tc.want)
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

	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("hostname", "e2e.test")

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			secrets.WithUnixSecretSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(config),
		),
	))

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	cmd := e.fetchCommand(authtoken)

	require.EventuallyWithT(v.T(), func(ct *assert.CollectT) {
		resp, err := v.Env().RemoteHost.Execute(cmd)
		require.NoError(ct, err)
		assert.Contains(ct, resp, want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, resp, want)
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
	// TODO: add a better comparison here by json parsing the response
	want := `{"entities":{"process":{"infos":{"sources(merged):`

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

	cmd := e.fetchCommand(authtoken)
	host := v.Env().RemoteHost

	require.EventuallyWithT(v.T(), func(ct *assert.CollectT) {
		resp, err := host.Execute(cmd)
		require.NoError(ct, err)
		assert.Contains(ct, resp, want, "%s %s returned: %s, wanted: %s", e.method, e.endpoint, resp, want)
	}, 2*time.Minute, 10*time.Second)
}
