// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"bytes"
	"testing"
	"text/template"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var (
	testcases = map[string]bool{
		"ecc -c TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       true,
		"rsa -c TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         true,
		"ecc -c TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       true,
		"rsa -c TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         true,
		"rsa -c TLS_AES_128_GCM_SHA256 --tls-max=1.3":          true,
		"rsa -c TLS_AES_256_GCM_SHA384 --tls-max=1.3":          true,
		"ecc -c TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": false,
		"rsa -c TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   false,
		"ecc -c TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":          false,
		"rsa -c TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":            false,
		"ecc -c TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":          false,
		"rsa -c TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":            false,
		"rsa -c TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":           false,
	}
)

//go:embed fixtures/docker-compose.yaml.tmpl
var dockerComposeTemplate string

type fipsServerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestFIPSCiphersSuite(t *testing.T) {
	e2e.Run(t, &fipsServerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner()))
}

func (v *fipsServerSuite) TestFIPSCiphersFIPSEnabled() {
	templateVars := map[string]interface{}{
		"FIPSEnabled": "1",
	}
	dockerCompose := fillTmplConfig(v.T(), dockerComposeTemplate, templateVars)

	v.UpdateEnv(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(dockerCompose)),
			),
		),
	)

	for command, shouldConnect := range testcases {
		v.T().Run(fmt.Sprintf("FIPS enabled testing '%v' (should connect %v)", command, shouldConnect), func(t *testing.T) {

			// Start the fips-server and waits for it to be ready
			runFipsServer(v, command)

			// Run diagnose to send requests and verify the server logs
			runAgentDiagnose(v)

			serverLogs := v.Env().RemoteHost.MustExecute("docker logs fips-server")
			if shouldConnect {
				assert.NotContains(t, serverLogs, "no cipher suite supported by both client and server")
			} else {
				assert.Contains(t, serverLogs, "no cipher suite supported by both client and server")
			}

			stopFipsServer(v)
		})
	}
}

func (v *fipsServerSuite) TestFIPSCiphersFIPSDisabled() {
	templateVars := map[string]interface{}{
		"FIPSEnabled": "0",
	}
	dockerCompose := fillTmplConfig(v.T(), dockerComposeTemplate, templateVars)

	v.UpdateEnv(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(dockerCompose)),
			),
		),
	)

	for command := range testcases {
		v.T().Run(fmt.Sprintf("FIPS disabled testing '%v'", command), func(t *testing.T) {

			// Start the fips-server and waits for it to be ready
			runFipsServer(v, command)
			// Run diagnose to send requests and verify the server logs
			runAgentDiagnose(v)

			serverLogs := v.Env().RemoteHost.MustExecute("docker logs fips-server")
			assert.NotContains(t, serverLogs, "no cipher suite supported by both client and server")

			stopFipsServer(v)
		})
	}
}

func (v *fipsServerSuite) TestFIPSCiphersTLSVersion() {
	templateVars := map[string]interface{}{
		"FIPSEnabled": "1",
	}
	dockerCompose := fillTmplConfig(v.T(), dockerComposeTemplate, templateVars)

	v.UpdateEnv(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(dockerCompose)),
			),
		),
	)

	runFipsServer(v, "rsa --tls-max=1.1")
	runAgentDiagnose(v)

	serverLogs := v.Env().RemoteHost.MustExecute("docker logs fips-server")
	assert.Contains(v.T(), serverLogs, "tls: client offered only unsupported version")

	stopFipsServer(v)
}

func runFipsServer(v *fipsServerSuite, command string) {
	_, err := v.Env().RemoteHost.Execute("docker run --rm -d --network fips-network --name fips-server ghcr.io/datadog/apps-fips-server:main " + command)
	require.NoError(v.T(), err)

	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		serverLogs, _ := v.Env().RemoteHost.Execute("docker logs fips-server")
		assert.Contains(t, serverLogs, "Server Starting...", "Server should start")
		assert.Equal(t, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 10*time.Second, 2*time.Second)
}

func runAgentDiagnose(v *fipsServerSuite) {
	_ = v.Env().Docker.Client.ExecuteCommand("datadog-agent", `sh`, `-c`, `DD_DD_URL=https://fips-server:443 agent diagnose --include connectivity-datadog-core-endpoints --local`)
}

func stopFipsServer(v *fipsServerSuite) {
	_, err := v.Env().RemoteHost.Execute("docker stop fips-server")
	require.NoError(v.T(), err)
}

func fillTmplConfig(t *testing.T, tmplContent string, templateVars any) string {
	t.Helper()

	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	require.NoError(t, err)

	err = tmpl.Execute(&buffer, templateVars)
	require.NoError(t, err)

	return buffer.String()
}
