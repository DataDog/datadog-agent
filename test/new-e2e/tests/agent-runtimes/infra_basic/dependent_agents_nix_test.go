// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infrabasic

import (
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type dependentAgentsLinuxSuite struct {
	dependentAgentsSuite
}

func TestDependentAgentsLinuxSuite(t *testing.T) {
	t.Parallel()

	suite := &dependentAgentsLinuxSuite{
		dependentAgentsSuite{
			descriptor: e2eos.Ubuntu2204,
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

// TestTraceAgentNotRunning verifies trace-agent does not run in basic mode
func (s *dependentAgentsLinuxSuite) TestTraceAgentNotRunning() {
	s.assertTraceAgentNotRunning()
}

// TestProcessAgentNotRunning verifies process-agent does not run in basic mode
func (s *dependentAgentsLinuxSuite) TestProcessAgentNotRunning() {
	s.assertProcessAgentNotRunning()
}

// TestSystemProbeNotRunning verifies system-probe does not run in basic mode
func (s *dependentAgentsLinuxSuite) TestSystemProbeNotRunning() {
	s.assertSystemProbeNotRunning()
}

// TestSecurityAgentNotRunning verifies security-agent does not run in basic mode
func (s *dependentAgentsLinuxSuite) TestSecurityAgentNotRunning() {
	s.assertSecurityAgentNotRunning()
}

// TestCoreAgentStillRunning verifies the core agent continues to run
func (s *dependentAgentsLinuxSuite) TestCoreAgentStillRunning() {
	s.configureBasicMode()
	s.restartCoreAgent()
	s.assertCoreAgentStillRunning()
}

// ========================================
// Tests with agents explicitly enabled
// These tests verify that basic mode enforcement takes precedence
// over individual agent configuration settings
// ========================================

// TestTraceAgentNotRunningEvenWhenEnabled verifies trace-agent doesn't run even when apm_config.enabled: true
func (s *dependentAgentsLinuxSuite) TestTraceAgentNotRunningEvenWhenEnabled() {
	s.assertTraceAgentNotRunningEvenWhenEnabled()
}

// TestProcessAgentNotRunningEvenWhenEnabled verifies process-agent doesn't run even when process_config.enabled: true
func (s *dependentAgentsLinuxSuite) TestProcessAgentNotRunningEvenWhenEnabled() {
	s.assertProcessAgentNotRunningEvenWhenEnabled()
}

// TestSystemProbeNotRunningEvenWhenEnabled verifies system-probe doesn't run even when system_probe_config.enabled: true
func (s *dependentAgentsLinuxSuite) TestSystemProbeNotRunningEvenWhenEnabled() {
	s.assertSystemProbeNotRunningEvenWhenEnabled()
}

// TestSecurityAgentNotRunningEvenWhenEnabled verifies security-agent doesn't run even when runtime_security_config.enabled: true
func (s *dependentAgentsLinuxSuite) TestSecurityAgentNotRunningEvenWhenEnabled() {
	s.assertSecurityAgentNotRunningEvenWhenEnabled()
}
