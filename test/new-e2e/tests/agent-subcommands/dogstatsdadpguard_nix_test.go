// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	common "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-pipelines/common"
)

// dogstatsdADPGuardSuite verifies that DogStatsD diagnostic CLI commands print
// a clear error and exit non-zero when the Agent Data Plane owns the
// DogStatsD socket.
type dogstatsdADPGuardSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (s *dogstatsdADPGuardSuite) SetupTest() {
	// Confirm ADP is actually running and owns UDP/8125 before testing the
	// guard. Without this, a misconfigured agent could silently fall back to
	// the Core Agent pipeline and the guard would never fire.
	common.AssertADPRunning(s.T(), s.Env().RemoteHost)
}

func (s *dogstatsdADPGuardSuite) TestDogstatsdCaptureBlockedByADP() {
	s.assertCommandBlocked("dogstatsd-capture -d 5s")
}

// assertCommandBlocked runs a dogstatsd CLI command via the agent binary and
// asserts that it fails with the ADP guard error message.
func (s *dogstatsdADPGuardSuite) assertCommandBlocked(subcommand string) {
	output, err := s.Env().RemoteHost.Execute("sudo datadog-agent " + subcommand)
	require.Error(s.T(), err, "command should fail when ADP owns DogStatsD")
	combined := output + err.Error()
	assert.Contains(s.T(), combined, "DogStatsD traffic is being served by the Agent Data Plane")
	assert.Contains(s.T(), combined, "agent-data-plane")
}

// TestADPGuardSuite runs the ADP guard e2e tests on Linux.
func TestADPGuardSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogstatsdADPGuardSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
log_level: INFO
use_dogstatsd: true
data_plane.enabled: true
data_plane.dogstatsd.enabled: true
`),
				),
			),
		)))
}
