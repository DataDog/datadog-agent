// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sysprobefunctional

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

const (
	vdiAgentConfig = `
vdi:
  enabled: true
`
	vdiIntegrationConfig = `
init_config:

instances:
  - provider: aws_workspaces
    aws_workspaces:
      product: personal
    inventory_stale_ttl: 300
`
)

type vdiSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestVDISuite(t *testing.T) {
	e2e.Run(t, &vdiSuite{}, e2e.WithProvisioner(awsHostWindows.Provisioner(
		awsHostWindows.WithRunOptions(
			windows.WithAgentOptions(
				agentparams.WithAgentConfig(vdiAgentConfig),
				agentparams.WithIntegration("vdi.d", vdiIntegrationConfig),
			),
		),
	)))
}

func (s *vdiSuite) TestVDIStartsSystemProbeAndFailsOpenWithoutDCV() {
	status, err := windowsCommon.GetServiceStatus(s.Env().RemoteHost, "datadog-system-probe")
	s.Require().NoError(err)
	s.Require().Equal("running", strings.ToLower(strings.TrimSpace(status)))

	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(collect *assert.CollectT) {
		dcvHealth, err := fakeintake.FilterCheckRuns("vdi.dcv.health")
		require.NoError(collect, err)
		require.NotEmpty(collect, dcvHealth)
		require.Equal(collect, 2, dcvHealth[len(dcvHealth)-1].Status)

		enrichmentHealth, err := fakeintake.FilterCheckRuns("vdi.session_enrichment.health")
		require.NoError(collect, err)
		require.NotEmpty(collect, enrichmentHealth)
		require.Equal(collect, 1, enrichmentHealth[len(enrichmentHealth)-1].Status)
	}, 2*time.Minute, 10*time.Second)
}
