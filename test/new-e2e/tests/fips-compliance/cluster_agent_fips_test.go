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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/docker"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/docker-compose-cluster-agent.yaml
var clusterAgentDockerCompose string

type fipsServerClusterAgentSuite struct {
	fipsServerSuite[environments.DockerHost]
}

func TestFIPSCiphersClusterAgentSuite(t *testing.T) {
	require.NotEmpty(t, os.Getenv("E2E_COMMIT_SHA"), "E2E_COMMIT_SHA must be set")
	require.NotEmpty(t, os.Getenv("E2E_PIPELINE_ID"), "E2E_PIPELINE_ID must be set")

	e2e.Run(
		t,
		&fipsServerClusterAgentSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithAgentOptions(
					dockeragentparams.WithFIPS(),
					dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(strings.ReplaceAll(clusterAgentDockerCompose, "{APPS_VERSION}", apps.Version))),
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
	// lookup the compose file used by environments.DockerHost (like the nix test does)
	composeFiles := strings.Split(host.MustExecute(`docker inspect --format='{{index (index .Config.Labels "com.docker.compose.project.config_files")}}' cluster-agent`), ",")
	formattedComposeFiles := strings.Join(composeFiles, " -f ")
	formattedComposeFiles = strings.TrimSpace(formattedComposeFiles)
	// supply workers to base fipsServerSuite
	s.fipsServer = newFIPSServer(host, formattedComposeFiles)

	// Configure generateTestTraffic for cluster agent
	s.generateTestTraffic = func() {
		// Use cluster agent diagnose to test connectivity to Datadog core endpoints
		// This triggers TLS connections using the cluster agent's Go-Boring implementation
		// Perfect for testing FIPS cipher compliance against the FIPS server
		_ = host.MustExecute(fmt.Sprintf(
			`docker-compose -f %s exec cluster-agent sh -c "DD_DD_URL=https://dd-fips-server:443 timeout 30 datadog-cluster-agent diagnose --include connectivity-datadog-core-endpoints || true"`,
			formattedComposeFiles,
		))
	}
}
