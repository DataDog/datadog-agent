// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
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
	e2e.Run(t, &apiSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

type agentEndpointInfo struct {
	name     string
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

func (v *apiSuite) TestInternalAgentAPIEndpoints() {
	testcases := []struct {
		agentEndpointInfo
		// additional_setup func(*apiSuite)
		// filter string
		want string
	}{
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "version",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/version",
				method:   "GET",
				data:     "",
			},
			// filter: `jq -r '. | [.Major, .Minor, .Patch] | join(".")'`,
			// want: `7.54.0`,
			want: `"Major":7,"Minor":5`,
		},
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "flare",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/flare",
				method:   "POST",
				data:     "{}",
			},
			want: `/tmp/datadog-agent-`,
		},
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "secrets",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/secrets",
				method:   "GET",
				data:     "",
			},
			// additional_setup:
			// TODO: this requires the secrets_backend to be enabled
			want: `secrets feature is not enabled`,
		},
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "secrets",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/secrets",
				method:   "GET",
				data:     "",
			},
			// TODO: this requires the secrets_backend to be enabled
			want: `secrets feature is not enabled`,
		},
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "tagger",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/tagger-list",
				method:   "GET",
				data:     "",
			},
			// TODO: extend this with better tagger settings
			want: `{"entities":{}}`,
		},
		{
			agentEndpointInfo: agentEndpointInfo{
				name:     "workloadmeta",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/workload-list",
				method:   "GET",
				data:     "",
			},
			// TODO: extend this with better workloadmeta settings
			want: `{"entities":{}}`,
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
				assert.Contains(ct, resp, tc.want, "%s %s returned: %s, wanted: %s", tc.method, tc.endpoint, resp, tc.want)
			}, 2*time.Minute, 10*time.Second)
		})
	}
}
