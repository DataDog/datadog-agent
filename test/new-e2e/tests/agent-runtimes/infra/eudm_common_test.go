// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"testing"
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

type eudmSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

// ============================================================================
// Configuration
// ============================================================================

func (s *eudmSuite) getSuiteOptions() []e2e.SuiteOption {
	// Build agent options with EUDM mode configuration
	// The wlan check is automatically loaded via StaticConfigListener
	// (checks with ad_identifiers: [_end_user_device] are scheduled)
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(`infrastructure_mode: "end_user_device"`),
	}

	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor)),
				ec2.WithAgentOptions(agentOptions...),
			),
		),
	))

	return suiteOptions
}

// ============================================================================
// Test Functions
// ============================================================================

// TestEUDMChecks verifies that EUDM checks are scheduled and run
// in EUDM (end_user_device) infrastructure mode.
// Note: On EC2 instances without WiFi hardware, the wlan check runs but emits no metrics
// since there's no WLAN interface to monitor. This is expected behavior.
// Note: The battery check is also configured for EUDM mode but cannot be tested on EC2
// instances since they don't have battery hardware - the check skips itself during Configure().
func (s *eudmSuite) TestEUDMChecks() {
	checks := []string{"wlan"}

	for _, checkName := range checks {
		s.T().Run(checkName+"_scheduled", func(t *testing.T) {
			t.Logf("Verifying %s check is scheduled in EUDM mode...", checkName)

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				verifyCheckSchedulingViaStatusAPI(t, c, s.Env(), []string{checkName}, true)
			}, 4*time.Minute, 10*time.Second, "%s check should be scheduled in EUDM mode", checkName)
		})

		s.T().Run(checkName+"_runs", func(t *testing.T) {
			t.Logf("Verifying %s check runs successfully...", checkName)

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				ran := verifyCheckRuns(t, s.Env(), checkName)
				assert.True(c, ran, "%s check must run in EUDM mode", checkName)
			}, 4*time.Minute, 10*time.Second, "%s check should run in EUDM mode", checkName)
		})
	}
}
