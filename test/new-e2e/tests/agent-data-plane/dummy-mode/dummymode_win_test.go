// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dummymode

import (
	"fmt"
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// TestDummyModeWindowsSuite exercises the dummy mode pre-flight on Windows, where the
// throwaway DogStatsD endpoint is a named pipe rather than a unix socket
// (comp/dataplane/dummymode/impl/listener_windows.go).
func TestDummyModeWindowsSuite(t *testing.T) {
	t.Parallel()

	suite := &dummyModeSuite{
		descriptor: e2eos.WindowsServerDefault,
		goos:       "windows",

		// Windows is the only platform with a second eligibility gate.
		// sanitizeDataPlaneConfig (pkg/config/setup/config.go) pins data_plane.enabled to false
		// at SourceAgentRuntime unless process_manager.enabled is true, and isEligible requires
		// the setting to still be at SourceDefault — so with the process manager off, the
		// pre-flight silently does not run and this suite would fail with no explanation.
		// process_manager.enabled already defaults to true; setting it explicitly keeps the
		// suite meaningful if that default ever changes, and documents the dependency.
		//
		// Setting it is safe for the isEligible check because it is a different setting: only
		// data_plane.enabled's source matters there.
		extraAgentConfig: "process_manager.enabled: true",

		agentLogPath: `C:\ProgramData\Datadog\logs\agent.log`,
		restartAgent: func(host *components.RemoteHost) error {
			// RestartService stops and starts rather than issuing Restart-Service, which fails
			// on datadogagent because other Agent services depend on it.
			return windowsCommon.RestartService(host, "datadogagent")
		},
		grepDummyMode: func(logPath string) string {
			return fmt.Sprintf(
				`Select-String -Path '%s' -Pattern 'dummy mode' | Select-Object -Last 50 | ForEach-Object { $_.Line }`,
				logPath)
		},
	}

	e2e.Run(t, suite, suite.suiteOptions()...)
}
