// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dummymode

import (
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

// TestDummyModeLinuxSuite exercises the dummy mode pre-flight on Linux, where the throwaway
// DogStatsD endpoint is a unix datagram socket under run_path
// (comp/dataplane/dummymode/impl/listener_nix.go).
//
// Note that the packaging also installs a real datadog-agent-data-plane.service, which is
// started alongside the Agent and exits immediately because data_plane.enabled is false. The
// dummy process is a separate short-lived child of the Core Agent, which is why this suite
// asserts on the probe metric rather than on an agent-data-plane process being alive.
func TestDummyModeLinuxSuite(t *testing.T) {
	t.Parallel()

	suite := &dummyModeSuite{
		descriptor:   e2eos.Ubuntu2204,
		goos:         "linux",
		agentLogPath: "/var/log/datadog/agent.log",
		restartAgent: func(host *components.RemoteHost) error {
			_, err := host.Execute("sudo systemctl restart datadog-agent.service")
			return err
		},
		grepDummyMode: nixGrepDummyMode,
	}

	e2e.Run(t, suite, suite.suiteOptions()...)
}
