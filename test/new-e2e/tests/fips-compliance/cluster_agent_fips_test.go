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
	"testing"
	"time"

	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/docker-compose-cluster-agent.yaml
var clusterAgentDockerCompose string

// buildClusterAgentImagePath constructs the cluster agent FIPS image path
// following the same logic as dockerClusterAgentFullImagePath()
func buildClusterAgentImagePath() string {
	pipelineID := os.Getenv("E2E_PIPELINE_ID")
	commitSHA := os.Getenv("E2E_COMMIT_SHA")

	if pipelineID == "" || commitSHA == "" {
		panic("E2E_PIPELINE_ID and E2E_COMMIT_SHA must be set for FIPS cluster agent tests")
	}
	tag := fmt.Sprintf("%s-%s-fips", pipelineID, commitSHA)
	registry := "669783387624.dkr.ecr.us-east-1.amazonaws.com"

	return fmt.Sprintf("%s/cluster-agent-qa:%s", registry, tag)
}

type fipsServerClusterAgentSuite struct {
	fipsServerSuite[environments.DockerHost]
	clusterAgentImage string
	// appsRegistry is the registry provisioning used for apps-fips-server.
	// Per-subtest compose calls must pass it as DD_APPS_REGISTRY or the fixture
	// falls back to ghcr.io/datadog.
	appsRegistry string
}

func TestFIPSCiphersClusterAgentSuite(t *testing.T) {
	require.NotEmpty(t, os.Getenv("E2E_COMMIT_SHA"), "E2E_COMMIT_SHA must be set")
	require.NotEmpty(t, os.Getenv("E2E_PIPELINE_ID"), "E2E_PIPELINE_ID must be set")

	// Build cluster agent image path for provisioning (needed by docker-compose)
	clusterAgentImage := buildClusterAgentImagePath()

	e2e.Run(
		t,
		&fipsServerClusterAgentSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					scendocker.WithAgentOptions(
						dockeragentparams.WithFIPS(),
						dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(strings.ReplaceAll(clusterAgentDockerCompose, "{APPS_VERSION}", apps.Version))),
						dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{
							"CLUSTER_AGENT_IMAGE": pulumi.String(clusterAgentImage),
						}),
					),
				),
			),
		),
		e2e.WithSkipDeleteOnFailure(),
	)
}

func (s *fipsServerClusterAgentSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	// lookup the compose file used by environments.DockerHost
	composeFiles := strings.Split(host.MustExecute(`docker inspect --format='{{index (index .Config.Labels "com.docker.compose.project.config_files")}}' cluster-agent`), ",")
	formattedComposeFiles := strings.Join(composeFiles, " -f ")
	formattedComposeFiles = strings.TrimSpace(formattedComposeFiles)

	// Get the cluster agent image from the running container to use during docker-compose operations
	s.clusterAgentImage = strings.TrimSpace(host.MustExecute(`docker inspect --format='{{.Config.Image}}' cluster-agent`))
	fipsImage := strings.TrimSpace(host.MustExecute(`docker inspect --format='{{.Config.Image}}' dd-fips-server`))
	registry, ok := appsRegistryFromImage(fipsImage)
	require.Truef(s.T(), ok, "dd-fips-server image %q has no /apps-fips-server segment to derive DD_APPS_REGISTRY", fipsImage)
	s.appsRegistry = registry

	// supply workers to base fipsServerSuite (same as other tests)
	s.fipsServer = newFIPSServer(host, formattedComposeFiles)

	// Configure generateTestTraffic for cluster agent
	s.generateTestTraffic = func() {
		// Use cluster agent diagnose to test connectivity to Datadog core endpoints
		// This triggers TLS connections using the cluster agent's Go-Boring implementation
		// Note: We bypass the default entrypoint to avoid Kubernetes API dependencies

		// CLUSTER_AGENT_IMAGE has no default in the fixture. DD_APPS_REGISTRY keeps
		// the fips-server image on the registry provisioning already pulled.
		envVars := s.composeEnv(nil)
		cmd := fmt.Sprintf(
			`docker-compose -f %s exec cluster-agent sh -c "DD_DD_URL=https://dd-fips-server:443 timeout 30 /opt/datadog-agent/bin/datadog-cluster-agent diagnose --include connectivity-datadog-core-endpoints || true"`,
			formattedComposeFiles,
		)
		_, _ = host.Execute(cmd, client.WithEnvVariables(envVars))
	}
}

// appsRegistryFromImage returns the registry prefix of a pulled apps-fips-server image.
// The fixture interpolates ${DD_APPS_REGISTRY}/apps-fips-server, so the prefix is everything
// before that path segment.
func appsRegistryFromImage(image string) (string, bool) {
	const marker = "/apps-fips-server"
	i := strings.LastIndex(image, marker)
	if i <= 0 {
		return "", false
	}
	return image[:i], true
}

func (s *fipsServerClusterAgentSuite) composeEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		"CLUSTER_AGENT_IMAGE": s.clusterAgentImage,
		"DD_APPS_REGISTRY":    s.appsRegistry,
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// startFIPSServerWithClusterAgentImage restarts the fips server and the cluster-agent container.
// The compose command runs once under a host timeout so a hung pull or health wait returns
// an error instead of outliving the test timer.
func (s *fipsServerClusterAgentSuite) startFIPSServerWithClusterAgentImage(tc cipherTestCase) {
	s.stopFIPSServerWithClusterAgentImage()

	extra := map[string]string{"CERT": tc.cert}
	if tc.cipher != "" {
		extra["CIPHER"] = "-c " + tc.cipher
	}
	if tc.tlsMax != "" {
		extra["TLS_MAX"] = "--tls-max " + tc.tlsMax
	}
	if tc.tlsMin != "" {
		extra["TLS_MIN"] = "--tls-min " + tc.tlsMin
	}

	cmd := fmt.Sprintf(
		"timeout 150 docker-compose -f %s up --detach --wait --wait-timeout 90 dd-fips-server cluster-agent",
		strings.TrimSpace(s.fipsServer.composeFiles),
	)
	if _, err := s.fipsServer.dockerHost.Execute(cmd, client.WithEnvVariables(s.composeEnv(extra))); err != nil {
		s.T().Logf("Error starting fips-server: %v", err)
		require.NoError(s.T(), err)
	}

	// Wait for container to start and ensure it's a fresh instance (reuse base logic)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		serverLogs, _ := s.fipsServer.dockerHost.Execute("docker logs dd-fips-server")
		assert.Contains(c, serverLogs, "Server Starting...", "fips-server timed out waiting for cipher initialization to finish")
		assert.Equal(c, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 60*time.Second, 5*time.Second)
}

// stopFIPSServerWithClusterAgentImage removes only the fips-server container.
// The fixture service is dd-fips-server, so compose down of "fips-server" never matches it.
// docker rm -f leaves the shared compose network that agent and cluster-agent use.
func (s *fipsServerClusterAgentSuite) stopFIPSServerWithClusterAgentImage() {
	names, err := s.fipsServer.dockerHost.Execute(`docker container ls -a --filter name=dd-fips-server --format '{{.Names}}'`)
	if err != nil {
		s.T().Logf("listing dd-fips-server: %v", err)
		return
	}
	if strings.TrimSpace(names) == "" {
		return
	}
	if _, err := s.fipsServer.dockerHost.Execute("docker rm -f dd-fips-server"); err != nil {
		s.T().Logf("removing dd-fips-server: %v", err)
	}
}

// TestFIPSCiphers overrides the base test to use our helper methods with CLUSTER_AGENT_IMAGE
func (s *fipsServerClusterAgentSuite) TestFIPSCiphers() {
	for _, tc := range testcases {
		s.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {
			s.startFIPSServerWithClusterAgentImage(tc)
			s.T().Cleanup(func() {
				s.stopFIPSServerWithClusterAgentImage()
			})

			s.generateTestTraffic()

			serverLogs := s.fipsServer.Logs()
			if tc.want {
				assert.Contains(s.T(), serverLogs, "Negotiated cipher suite: "+tc.cipher)
			} else {
				assert.Contains(s.T(), serverLogs, "no cipher suite supported by both client and server")
			}
		})
	}
}

// TestFIPSCiphersTLSVersion overrides the base test to use our helper methods with CLUSTER_AGENT_IMAGE
func (s *fipsServerClusterAgentSuite) TestFIPSCiphersTLSVersion() {
	tc := cipherTestCase{cert: "rsa", tlsMax: "1.1"}
	s.startFIPSServerWithClusterAgentImage(tc)
	s.T().Cleanup(func() {
		s.stopFIPSServerWithClusterAgentImage()
	})

	s.generateTestTraffic()

	serverLogs := s.fipsServer.Logs()
	assert.Contains(s.T(), serverLogs, "tls: client offered only unsupported version")
}
