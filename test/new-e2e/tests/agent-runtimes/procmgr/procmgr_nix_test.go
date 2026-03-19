// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procmgr provides e2e tests for the Datadog Process Manager (dd-procmgrd)
package procmgr

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	procmgrdBin   = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	procmgrCLI    = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	procmgrSocket = "/var/run/datadog-procmgrd/dd-procmgrd.sock"

	testProcessConfig = `command: /bin/sleep
args:
  - "3600"
auto_start: true
restart: always
description: E2E test process
`
)

type procmgrLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
	hasCLI bool
}

func TestProcmgrSmokeLinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &procmgrLinuxSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithFile("/etc/datadog-agent/processes.d/test-sleep.yaml", testProcessConfig, true),
				),
			),
		),
	))
}

func (s *procmgrLinuxSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	_, err := s.Env().RemoteHost.Execute("test -f " + procmgrdBin)
	if err != nil {
		s.T().Skip("dd-procmgrd not included in this agent package; skipping process manager tests")
	}

	_, err = s.Env().RemoteHost.Execute("test -f " + procmgrCLI)
	s.hasCLI = err == nil

	if s.hasCLI {
		// Make the socket accessible to the SSH user so the CLI can connect without sudo.
		// The socket is owned by dd-agent; sudo with full paths outside secure_path
		// requires a TTY that SSH non-interactive sessions don't provide.
		require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
			_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("sudo chmod 0777 %s", procmgrSocket))
			assert.NoError(t, err, "socket not yet available")
		}, 30*time.Second, 2*time.Second)
	}
}

func (s *procmgrLinuxSuite) TestBinariesExist() {
	s.Env().RemoteHost.MustExecute("test -f " + procmgrdBin)

	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
	s.Env().RemoteHost.MustExecute("test -f " + procmgrCLI)
}

func (s *procmgrLinuxSuite) TestServiceRunning() {
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-procmgrd")
		assert.Equal(t, "active", strings.TrimSpace(out))
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIStatus() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " status")
		assert.Contains(t, out, "Version")
		assert.Contains(t, out, "Uptime")
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIListShowsConfiguredProcess() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " list")
		assert.Contains(t, out, "test-sleep")
		assert.Contains(t, out, "Running")
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIDescribe() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " describe test-sleep")
		assert.Contains(t, out, "test-sleep")
		assert.Contains(t, out, "/bin/sleep")
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) requireCLI() {
	s.T().Helper()
	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
}
