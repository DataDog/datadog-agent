// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// parMonolithProcess is the always-installed monolithic runner definition. It must
// stand down while split mode is active.
const parMonolithProcess = "datadog-agent-action"

type windowsPARSplitLifecycleSuite struct {
	e2e.BaseSuite[environments.Host]

	installRoot string
}

func TestWindowsPARSplitLifecycleSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	config := fmt.Sprintf(`private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: false
  urn: %s
  private_key: %s
`, urn, privateKey)

	e2e.Run(t, &windowsPARSplitLifecycleSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(scenec2.WithOS(e2eos.WindowsServerDefault)),
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *windowsPARSplitLifecycleSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// Everything below can fail, so the framework needs the failure hook.
	defer s.CleanupOnSetupFailure()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	s.installRoot = installRoot

	exists, err := s.Env().RemoteHost.FileExists(s.parControlBinary())
	s.Require().NoError(err)
	s.Require().True(exists, "par-control.exe should be installed at %s", s.parControlBinary())
}

// TestControlStartsExecutor proves the MSI ships both process definitions and that
// dd-procmgrd starts par-control, which connects to dd-procmgrd over the named
// pipe and starts the on-demand executor.
func (s *windowsPARSplitLifecycleSuite) TestControlStartsExecutor() {
	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
}

// TestMonolithStandsDown proves the monolithic runner exits cleanly instead of
// polling alongside the control plane. Its definition is auto_start, so a clean
// exit (not a crash) is the only correct outcome.
func (s *windowsPARSplitLifecycleSuite) TestMonolithStandsDown() {
	s.waitForProcessState(parMonolithProcess, "Exited", 2*time.Minute)
}

// TestControlStopsWithoutReenteringSupervisor proves par-control handles the
// CTRL_BREAK that dd-procmgrd sends for a graceful stop, instead of ignoring it
// and being force-killed once the 30s stop_timeout expires. The supervisor keeps
// owning the executor.
func (s *windowsPARSplitLifecycleSuite) TestControlStopsWithoutReenteringSupervisor() {
	defer func() {
		s.Require().NoError(s.runProcmgr("start", parControlProcess))
		s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	}()

	started := time.Now()
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	s.Require().Less(time.Since(started), 15*time.Second, "par-control stop should not hit its 30s timeout")

	s.waitForProcessState(parControlProcess, "Stopped", 10*time.Second)
	s.waitForProcessState(parExecutorProcess, "Running", 10*time.Second)
}

func (s *windowsPARSplitLifecycleSuite) parControlBinary() string {
	return filepath.Join(s.installRoot, "bin", "agent", "par-control.exe")
}

func (s *windowsPARSplitLifecycleSuite) procmgrCLI() string {
	return filepath.Join(s.installRoot, "bin", "agent", "dd-procmgr.exe")
}

// runProcmgr shells out to the CLI, which reaches dd-procmgrd over the default
// named pipe.
func (s *windowsPARSplitLifecycleSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`& "%s" %s %s`, s.procmgrCLI(), command, name))
	return err
}

func (s *windowsPARSplitLifecycleSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`& "%s" describe %s`, s.procmgrCLI(), name))
		assert.NoError(c, err)
		assert.Contains(c, strings.ReplaceAll(output, " ", ""), "State:"+state)
	}, timeout, 2*time.Second, "%s should become %s", name, state)
}
