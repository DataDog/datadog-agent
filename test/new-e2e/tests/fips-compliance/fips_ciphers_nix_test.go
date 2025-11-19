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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/docker-compose.yaml
var dockerCompose string

type fipsServerLinuxSuite struct {
	fipsServerSuite[environments.DockerHost]
}

func TestFIPSCiphersLinuxSuite(t *testing.T) {
	require.NotEmpty(t, os.Getenv("E2E_COMMIT_SHA"), "E2E_COMMIT_SHA must be set")
	require.NotEmpty(t, os.Getenv("E2E_PIPELINE_ID"), "E2E_PIPELINE_ID must be set")

	e2e.Run(
		t,
		&fipsServerLinuxSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					scendocker.WithAgentOptions(
						dockeragentparams.WithFIPS(),
						dockeragentparams.WithExtraComposeManifest("fips-server", pulumi.String(strings.ReplaceAll(dockerCompose, "{APPS_VERSION}", apps.Version))),
					),
				),
			),
		),
		e2e.WithSkipDeleteOnFailure(),
	)
}

func (s *fipsServerLinuxSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	// lookup the compose file used by environments.DockerHost
	composeFiles := strings.Split(host.MustExecute(`docker inspect --format='{{index (index .Config.Labels "com.docker.compose.project.config_files")}}' datadog-agent`), ",")
	formattedComposeFiles := strings.Join(composeFiles, " -f ")
	formattedComposeFiles = strings.TrimSpace(formattedComposeFiles)
	// supply workers to base fipsServerSuite
	s.fipsServer = newFIPSServer(host, formattedComposeFiles)
	s.generateTestTraffic = func() {
		_ = host.MustExecute(fmt.Sprintf(`docker-compose -f %s exec agent sh -c "DD_DD_URL=https://dd-fips-server:443 agent diagnose --include connectivity-datadog-core-endpoints --local"`, formattedComposeFiles))
	}
}
