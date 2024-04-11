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
	var data string
	if len(endpointInfo.data) > 0 {
		data = endpointInfo.data
	} else {
		data = "{}"
	}

	// -L: follow redirects
	// -s: silent
	// -k: allow insecure server connections
	// -H: add a header
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
		endpoint agentEndpointInfo
		want     string
	}{
		{
			endpoint: agentEndpointInfo{
				name:     "version",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/version",
				method:   "GET",
				data:     "",
			},
			want: `"Major":7,"Minor":5`,
		},
		{
			endpoint: agentEndpointInfo{
				name:     "flare",
				scheme:   "https",
				port:     agentCmdPort,
				endpoint: "/agent/flare",
				method:   "POST",
				data:     "{}",
			},
			want: `Z-info.zip`,
		},
	}

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	for _, tc := range testcases {
		cmd := tc.endpoint.fetchCommand(authtoken)
		host := v.Env().RemoteHost
		require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
			resp, err := host.Execute(cmd)
			require.NoError(t, err)
			assert.Contains(t, resp, tc.want, "%s %s returned: %s, wanted: %s", tc.endpoint.method, tc.endpoint.endpoint, resp, tc.want)
		}, 2*time.Minute, 10*time.Second)
	}
}
