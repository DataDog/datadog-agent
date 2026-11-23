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

	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	s.fipsServer = newFIPSServer(s.T(), host, formattedComposeFiles)
	// The cluster agent fixture names the compose service dd-fips-server and interpolates CLUSTER_AGENT_IMAGE
	// with no default, so every compose call made by the shared fipsServer needs both.
	s.fipsServer.service = "dd-fips-server"
	s.fipsServer.extraEnv = map[string]string{"CLUSTER_AGENT_IMAGE": s.clusterAgentImage}

	// Configure generateTestTraffic for cluster agent
	s.generateTestTraffic = func() {
		// Use cluster agent diagnose to test connectivity to Datadog core endpoints
		// This triggers TLS connections using the cluster agent's Go-Boring implementation
		// Note: We bypass the default entrypoint to avoid Kubernetes API dependencies

		// Include CLUSTER_AGENT_IMAGE environment variable to avoid compose parsing errors
		envVars := map[string]string{
			"CLUSTER_AGENT_IMAGE": s.clusterAgentImage,
		}
		// The cluster-agent container runs sleep 3600 with no restart policy, and starting the fips server no
		// longer starts it. Bring it back if it has exited, without recreating a container that is still running.
		up := fmt.Sprintf("timeout -k 15 60 docker-compose -f %s up --detach --no-recreate cluster-agent", formattedComposeFiles)
		if _, err := host.Execute(up, client.WithEnvVariables(envVars)); err != nil {
			s.T().Logf("cluster-agent was not running and could not be started: %v", err)
		}
		cmd := fmt.Sprintf(
			`docker-compose -f %s exec cluster-agent sh -c "DD_DD_URL=https://dd-fips-server:443 timeout 30 /opt/datadog-agent/bin/datadog-cluster-agent diagnose --include connectivity-datadog-core-endpoints || true"`,
			formattedComposeFiles,
		)
		_, _ = host.Execute(cmd, client.WithEnvVariables(envVars))
	}
}
