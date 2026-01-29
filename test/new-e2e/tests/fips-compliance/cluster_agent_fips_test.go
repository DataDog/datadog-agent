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

	// supply workers to base fipsServerSuite (same as other tests)
	s.fipsServer = newFIPSServer(host, formattedComposeFiles)

	// Configure generateTestTraffic for cluster agent
	s.generateTestTraffic = func() {
		// Use cluster agent diagnose to test connectivity to Datadog core endpoints
		// This triggers TLS connections using the cluster agent's Go-Boring implementation
		// Note: We bypass the default entrypoint to avoid Kubernetes API dependencies

		// Include CLUSTER_AGENT_IMAGE environment variable to avoid compose parsing errors
		envVars := map[string]string{
			"CLUSTER_AGENT_IMAGE": s.clusterAgentImage,
		}
		cmd := fmt.Sprintf(
			`docker-compose -f %s exec cluster-agent sh -c "DD_DD_URL=https://dd-fips-server:443 timeout 30 /opt/datadog-agent/bin/datadog-cluster-agent diagnose --include connectivity-datadog-core-endpoints || true"`,
			formattedComposeFiles,
		)
		_, _ = host.Execute(cmd, client.WithEnvVariables(envVars))
	}
}

// Override base test methods to add CLUSTER_AGENT_IMAGE environment variable

// startFIPSServerWithClusterAgentImage is a helper that adds CLUSTER_AGENT_IMAGE when starting the server
func (s *fipsServerClusterAgentSuite) startFIPSServerWithClusterAgentImage(tc cipherTestCase) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		// stop currently running server, if any, so we can reset logs+env
		s.stopFIPSServerWithClusterAgentImage()

		// start datadog/apps-fips-server with env vars including CLUSTER_AGENT_IMAGE
		envVars := map[string]string{
			"CERT":                tc.cert,
			"CLUSTER_AGENT_IMAGE": s.clusterAgentImage,
		}
		if tc.cipher != "" {
			envVars["CIPHER"] = "-c " + tc.cipher
		}
		if tc.tlsMax != "" {
			envVars["TLS_MAX"] = "--tls-max " + tc.tlsMax
		}
		if tc.tlsMin != "" {
			envVars["TLS_MIN"] = "--tls-min " + tc.tlsMin
		}

		cmd := fmt.Sprintf("docker-compose -f %s up --detach --wait --timeout 300", strings.TrimSpace(s.fipsServer.composeFiles))
		_, err := s.fipsServer.dockerHost.Execute(cmd, client.WithEnvVariables(envVars))
		if err != nil {
			s.T().Logf("Error starting fips-server: %v", err)
			require.NoError(c, err)
		}
		assert.Nil(c, err)
	}, 120*time.Second, 10*time.Second, "docker-compose timed out starting server")

	// Wait for container to start and ensure it's a fresh instance (reuse base logic)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		serverLogs, _ := s.fipsServer.dockerHost.Execute("docker logs dd-fips-server")
		assert.Contains(c, serverLogs, "Server Starting...", "fips-server timed out waiting for cipher initialization to finish")
		assert.Equal(c, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 60*time.Second, 5*time.Second)
}

// stopFIPSServerWithClusterAgentImage is a helper that adds CLUSTER_AGENT_IMAGE when stopping the server
func (s *fipsServerClusterAgentSuite) stopFIPSServerWithClusterAgentImage() {
	fipsContainer := s.fipsServer.dockerHost.MustExecute("docker container ls -a --filter name=dd-fips-server --format '{{.Names}}'")
	if fipsContainer != "" {
		envVars := map[string]string{"CLUSTER_AGENT_IMAGE": s.clusterAgentImage}
		cmd := fmt.Sprintf("docker-compose -f %s down fips-server", strings.TrimSpace(s.fipsServer.composeFiles))
		_, err := s.fipsServer.dockerHost.Execute(cmd, client.WithEnvVariables(envVars))
		if err != nil {
			// Fallback to direct docker commands (same as base implementation)
			s.fipsServer.dockerHost.MustExecute("docker stop dd-fips-server || true")
			s.fipsServer.dockerHost.MustExecute("docker rm dd-fips-server || true")
		}
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
