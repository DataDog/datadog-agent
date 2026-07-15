// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const dockerHostSecurityAgentConfig = `compliance_config:
  enabled: true
`

type dockerHostCSPMSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestDockerHostCSPM is the symmetric counter-test of
// TestDockerRulesFilteredOnContainerdCRI: on a non-K8s Docker host the
// benchmark must still fire. Catches an over-greedy filter.
func TestDockerHostCSPM(t *testing.T) {
	e2e.Run(t, &dockerHostCSPMSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithDocker(),
				scenec2.WithAgentOptions(
					agentparams.WithSecurityAgentConfig(dockerHostSecurityAgentConfig),
				),
			),
		)),
	)
}

func (s *dockerHostCSPMSuite) TestDockerRulesFireOnDockerHost() {
	host := s.Env().RemoteHost

	host.MustExecute("sudo /opt/datadog-agent/embedded/bin/security-agent compliance check --dump-reports /tmp/reports")
	dump := host.MustExecute("sudo cat /tmp/reports")

	findings, err := parseFindingOutput(dump)
	require.NoError(s.T(), err)

	seenDockerRule := false
	for rule, results := range findings {
		if !strings.HasPrefix(rule, "cis-docker-1.2.0-") {
			continue
		}
		for _, r := range results {
			if r["result"] == "passed" || r["result"] == "failed" {
				seenDockerRule = true
				break
			}
		}
		if seenDockerRule {
			break
		}
	}
	assert.True(s.T(), seenDockerRule,
		"expected at least one cis-docker-1.2.0-* finding with result passed or failed on a non-K8s Docker host; the filter must not suppress rules outside Kubernetes")
}
