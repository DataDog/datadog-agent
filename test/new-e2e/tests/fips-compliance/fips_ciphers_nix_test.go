// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/docker"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type cipherTestCase struct {
	cert   string
	cipher string
	tlsMax string
	want   bool
}

var (
	testcases = []cipherTestCase{
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", want: true},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", want: true},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", want: true},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", want: true},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256", want: false},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA", want: false},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA", want: false},
		// TODO: the below are approved for TLS 1.3 but not supported by our fips-server yet
		//   see https://github.com/DataDog/test-infra-definitions/blob/221bbc806266eb15b90cb875deb79180e7591fbc/components/datadog/apps/fips/images/fips-server/src/tls.go#L48-L58
		// {cert: "rsa", cipher: "TLS_AES_128_GCM_SHA256", tlsMax: "1.3", want: true},
		// {cert: "rsa", cipher: "TLS_AES_256_GCM_SHA384", tlsMax: "1.3", want: true},
	}
)

//go:embed fixtures/docker-compose.yaml
var dockerCompose string

type fipsServerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestFIPSCiphersSuite(t *testing.T) {
	e2e.Run(t, &fipsServerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner()))
}

func (v *fipsServerSuite) TestFIPSCiphersFIPSEnabled() {
	imageTag := fmt.Sprintf("%s-%s-fips", os.Getenv("E2E_PIPELINE_ID"), os.Getenv("CI_COMMIT_SHA"))

	v.UpdateEnv(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithImageTag(imageTag),
				//dockeragentparams.WithFullImagePath("669783387624.dkr.ecr.us-east-1.amazonaws.com/agent:52591054-61b96d15-fips"),
				dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(dockerCompose)),
			),
		),
	)

	composeFiles := strings.Split(v.Env().RemoteHost.MustExecute(`docker inspect --format='{{index (index .Config.Labels "com.docker.compose.project.config_files")}}' dd-fips-server`), ",")
	formattedComposeFiles := strings.Join(composeFiles, " -f ")

	for _, tc := range testcases {
		v.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {

			// Start the fips-server and waits for it to be ready
			runFipsServer(v, tc, formattedComposeFiles)
			defer stopFipsServer(v, formattedComposeFiles)

			// Run diagnose to send requests and verify the server logs
			runAgentDiagnose(v, formattedComposeFiles)

			serverLogs := v.Env().RemoteHost.MustExecute("docker logs dd-fips-server")
			if tc.want {
				assert.Contains(v.T(), serverLogs, fmt.Sprintf("Negotiated cipher suite: %s", tc.cipher))
			} else {
				assert.Contains(v.T(), serverLogs, "no cipher suite supported by both client and server")
			}
		})
	}
}

func (v *fipsServerSuite) TestFIPSCiphersTLSVersion() {
	imageTag := fmt.Sprintf("%s-%s-fips", os.Getenv("E2E_PIPELINE_ID"), os.Getenv("CI_COMMIT_SHA"))

	v.UpdateEnv(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithImageTag(imageTag),
				//dockeragentparams.WithFullImagePath("669783387624.dkr.ecr.us-east-1.amazonaws.com/agent:52591054-61b96d15-fips"),
				dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(dockerCompose)),
			),
		),
	)

	composeFiles := strings.Split(v.Env().RemoteHost.MustExecute(`docker inspect --format='{{index (index .Config.Labels "com.docker.compose.project.config_files")}}' dd-fips-server`), ",")
	formattedComposeFiles := strings.Join(composeFiles, " -f ")

	runFipsServer(v, cipherTestCase{cert: "rsa", tlsMax: "1.1"}, formattedComposeFiles)
	defer stopFipsServer(v, formattedComposeFiles)

	runAgentDiagnose(v, formattedComposeFiles)

	serverLogs := v.Env().RemoteHost.MustExecute("docker logs dd-fips-server")
	assert.Contains(v.T(), serverLogs, "tls: client offered only unsupported version")
}

func runFipsServer(v *fipsServerSuite, tc cipherTestCase, composeFiles string) {
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		stopFipsServer(v, composeFiles)
		envvar := fmt.Sprintf("CERT=%s", tc.cert)
		if tc.cipher != "" {
			envvar = fmt.Sprintf(`%s CIPHER="-c %s"`, envvar, tc.cipher)
		}
		if tc.tlsMax != "" {
			envvar = fmt.Sprintf(`%s TLS_MAX="--tls-max %s"`, envvar, tc.tlsMax)
		}

		_, err := v.Env().RemoteHost.Execute(fmt.Sprintf("%s docker-compose -f %s up --detach  --wait --timeout 300", envvar, strings.TrimSpace(composeFiles)))
		if err != nil {
			v.T().Logf("Error starting fips-server: %v", err)
			require.NoError(t, err)
		}
		assert.Nil(t, err)
	}, 60*time.Second, 20*time.Second)

	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		serverLogs, _ := v.Env().RemoteHost.Execute("docker logs dd-fips-server")
		assert.Contains(t, serverLogs, "Server Starting...", "Server should start")
		assert.Equal(t, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 10*time.Second, 2*time.Second)
}

func runAgentDiagnose(v *fipsServerSuite, composeFiles string) {
	_ = v.Env().RemoteHost.MustExecute(fmt.Sprintf(`docker-compose -f %s exec agent sh -c "GOFIPS=1 DD_DD_URL=https://dd-fips-server:443 agent diagnose --include connectivity-datadog-core-endpoints --local"`, strings.TrimSpace(composeFiles)))
}

func stopFipsServer(v *fipsServerSuite, composeFiles string) {
	fipsContainer := v.Env().RemoteHost.MustExecute("docker container ls -a --filter name=dd-fips-server --format '{{.Names}}'")
	if fipsContainer != "" {
		v.Env().RemoteHost.MustExecute(fmt.Sprintf("docker-compose -f %s down fips-server", strings.TrimSpace(composeFiles)))
	}
}
