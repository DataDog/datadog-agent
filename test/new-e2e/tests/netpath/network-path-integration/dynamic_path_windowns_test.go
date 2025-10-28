// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type windowsDynamicPathTestSuite struct {
	baseNetworkPathIntegrationTestSuite
}

// TestDynamicPathSuiteLinux runs the Network Path Integration e2e suite for windows
func TestWindowsDynamicPathSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsNetworkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(string(dynamicPathDatadogYaml)),
			agentparams.WithSystemProbeConfig(string(dynamicPathSystemProbeYaml)),
		),
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
	)))
}

func (s *windowsDynamicPathTestSuite) TestWindowsDynamicPathMetrics() {
	hostname := s.Env().Agent.Client.Hostname()
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.checkDynamicPath(c, hostname)
	}, 5*time.Minute, 3*time.Second)
}
