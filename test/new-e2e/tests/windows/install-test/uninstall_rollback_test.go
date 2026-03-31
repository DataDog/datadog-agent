// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"testing"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testUninstallRollbackSuite struct {
	baseAgentMSISuite
}

func TestUninstallRollback(t *testing.T) {
	s := &testUninstallRollbackSuite{}
	Run(t, s)
}

// TestUninstallRollback verifies that when an uninstall fails and rolls back,
// the agent service can still start. This tests that the LSA password and SCM
// service credentials are properly restored during rollback.
func (s *testUninstallRollbackSuite) TestUninstallRollback() {
	vm := s.Env().RemoteHost

	// Install the agent normally
	s.installAgentPackage(vm, s.AgentPackage)

	// Trigger an uninstall that will fail during the deferred phase and roll back
	s.Run("uninstall rollback", func() {
		product, err := windowsAgent.GetDatadogAgentProductCode(vm)
		s.Require().NoError(err, "should get product code")

		err = windowsCommon.UninstallMSI(vm, product,
			"WIXFAILWHENDEFERRED=1",
			filepath.Join(s.SessionOutputDir(), "uninstall-rollback.log"))
		s.Require().Error(err, "uninstall should fail and roll back")
	})

	// Start the agent service — MSI rollback does not restart services that
	// were stopped during uninstall. This is the key assertion: if the password
	// was not restored, the service will fail to start because it runs as
	// ddagentuser and needs the password to log on.
	s.Run("agent service starts after rollback", func() {
		err := windowsCommon.StartService(vm, "datadogagent")
		s.Require().NoError(err, "should be able to start agent service after uninstall rollback")
		err = s.waitForServiceRunning(vm, "datadogagent", 300)
		s.Require().NoError(err, "agent service should be running after uninstall rollback")
	})

	// TODO(WINA-2538): Enable TestInstallExpectations once CleanupFiles no longer
	// deletes embedded3 before RemoveFiles, which prevents rollback restoration of
	// the Python distribution.
	// s.Run("agent is still installed", func() {
	// 	t := s.newTester(vm)
	// 	t.TestInstallExpectations(s.T())
	// })
}
