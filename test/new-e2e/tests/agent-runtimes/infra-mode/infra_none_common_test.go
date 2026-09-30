// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inframode

import (
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// ============================================================================
// Type Definitions
// ============================================================================

type noneSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

// ============================================================================
// Utility Functions
// ============================================================================

func (s *noneSuite) getSuiteOptions() []e2e.SuiteOption {
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(`infrastructure_mode: "none"`),
	}

	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInternetAccess()),
				ec2.WithAgentOptions(agentOptions...),
			),
		),
	))

	return suiteOptions
}

// ============================================================================
// Test Functions
// ============================================================================

// TestHostTags verifies that the infra_mode:none marker tag is attached
// to the agent's host-tags payload when running in none infrastructure mode.
func (s *noneSuite) TestHostTags() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		hosts, err := fakeintake.GetHosts()
		if !assert.NoError(c, err, "failed to fetch hosts from fakeintake") {
			return
		}
		if !assert.NotEmpty(c, hosts, "no hosts have sent host-tags payloads yet") {
			return
		}

		for _, host := range hosts {
			payloads, err := fakeintake.GetHostTags(host)
			if !assert.NoError(c, err, "failed to fetch host-tags for host %s", host) {
				continue
			}
			if !assert.NotEmpty(c, payloads, "no host-tags payloads for host %s", host) {
				continue
			}

			// Latest payload — host_tags are eventually consistent.
			tags := payloads[len(payloads)-1].HostTags

			assert.Contains(c, tags, "infra_mode:none",
				"expected infra_mode marker on host %s; got %v", host, tags)
		}
	}, 5*time.Minute, 15*time.Second, "none mode host tags did not appear in fakeintake host-tags payload")
}
