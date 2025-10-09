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

type dependentAgentsWindowsSuite struct {
	dependentAgentsSuite
}

func TestDependentAgentsWindowsSuite(t *testing.T) {
	t.Parallel()

	suite := &dependentAgentsWindowsSuite{
		dependentAgentsSuite{
			descriptor: e2eos.WindowsServerDefault,
		},
	}

	e2e.Run(t, suite, suite.getSuiteOptions()...)
}

// TestTraceAgentNotRunning verifies trace-agent does not run in basic mode on Windows
func (s *dependentAgentsWindowsSuite) TestTraceAgentNotRunning() {
	s.assertTraceAgentNotRunning()
}

// TestProcessAgentNotRunning verifies process-agent does not run in basic mode on Windows
func (s *dependentAgentsWindowsSuite) TestProcessAgentNotRunning() {
	s.assertProcessAgentNotRunning()
}

// TestSystemProbeNotRunning verifies system-probe does not run in basic mode on Windows
func (s *dependentAgentsWindowsSuite) TestSystemProbeNotRunning() {
	s.assertSystemProbeNotRunning()
}

// TestSecurityAgentNotRunning verifies security-agent does not run in basic mode on Windows
func (s *dependentAgentsWindowsSuite) TestSecurityAgentNotRunning() {
	s.assertSecurityAgentNotRunning()
}

// TestCoreAgentStillRunning verifies the core agent continues to run on Windows
func (s *dependentAgentsWindowsSuite) TestCoreAgentStillRunning() {
	s.configureBasicMode()
	s.restartCoreAgent()
	s.assertCoreAgentStillRunning()
}

// ========================================
// Tests with agents explicitly enabled
// These tests verify that basic mode enforcement takes precedence
// over individual agent configuration settings
// ========================================

// TestTraceAgentNotRunningEvenWhenEnabled verifies trace-agent doesn't run even when apm_config.enabled: true on Windows
func (s *dependentAgentsWindowsSuite) TestTraceAgentNotRunningEvenWhenEnabled() {
	s.assertTraceAgentNotRunningEvenWhenEnabled()
}

// TestProcessAgentNotRunningEvenWhenEnabled verifies process-agent doesn't run even when process_config.enabled: true on Windows
func (s *dependentAgentsWindowsSuite) TestProcessAgentNotRunningEvenWhenEnabled() {
	s.assertProcessAgentNotRunningEvenWhenEnabled()
}

// TestSystemProbeNotRunningEvenWhenEnabled verifies system-probe doesn't run even when system_probe_config.enabled: true on Windows
func (s *dependentAgentsWindowsSuite) TestSystemProbeNotRunningEvenWhenEnabled() {
	s.assertSystemProbeNotRunningEvenWhenEnabled()
}

// TestSecurityAgentNotRunningEvenWhenEnabled verifies security-agent doesn't run even when runtime_security_config.enabled: true on Windows
func (s *dependentAgentsWindowsSuite) TestSecurityAgentNotRunningEvenWhenEnabled() {
	s.assertSecurityAgentNotRunningEvenWhenEnabled()
}
