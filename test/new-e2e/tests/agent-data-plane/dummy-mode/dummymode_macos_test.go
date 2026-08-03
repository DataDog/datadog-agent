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

// TestDummyModeMacOSSuite exercises the dummy mode pre-flight on macOS. The transport is the
// same unix datagram socket as on Linux, but the packaging and service model are not: the DMG
// bootstraps a com.datadoghq.data-plane LaunchDaemon unconditionally
// (omnibus/package-scripts/agent-dmg/postinst), which exits immediately because
// data_plane.enabled is false. The dummy process is a separate short-lived child of the Core
// Agent.
//
// This suite provisions a mac1.metal dedicated host, which AWS bills with a 24-hour minimum
// allocation. Its CI job is therefore deploy-pipeline and manual only — see
// .gitlab/test/e2e/e2e.yml.
func TestDummyModeMacOSSuite(t *testing.T) {
	t.Parallel()

	suite := &dummyModeSuite{
		descriptor:   e2eos.MacOSDefault,
		goos:         "darwin",
		agentLogPath: "/opt/datadog-agent/logs/agent.log",
		restartAgent: func(host *components.RemoteHost) error {
			// kickstart -k kills the job if it is running and starts it again, which is the
			// launchd equivalent of a service restart.
			_, err := host.Execute("sudo launchctl kickstart -k system/com.datadoghq.agent")
			return err
		},
		grepDummyMode: nixGrepDummyMode,
	}

	e2e.Run(t, suite, suite.suiteOptions()...)
}
