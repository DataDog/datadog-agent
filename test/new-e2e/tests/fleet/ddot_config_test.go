// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/ddot"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
)

// ddotOtelConfigBenignPatch is a benign, always-valid merge patch for otel-config.yaml.
// It only sets service.telemetry.logs.level, so it cannot break the collector — the point
// is to exercise the config-experiment lifecycle, not the config content (the reported bug
// is content-independent).
const ddotOtelConfigBenignPatch = `{"service":{"telemetry":{"logs":{"level":"debug"}}}}`

// TestDDOTConfigUpdateSurvivesPromote reproduces and guards against the reported bug:
// a remote otel-config.yaml config experiment applies fine (the stable file is updated) but
// DDOT never restarts and stops reporting after promote.
//
// DDOT here runs as the auto-installed extension supervised by dd-procmgr (the Linux default).
// The config-experiment update relies on the service manager (systemd BindsTo/Conflicts/Wants)
// to stop the stable stack and bring it back on promote; this test asserts DDOT is actually
// running and reporting throughout start -> promote, which the existing config suite never
// checks (its restart detection only looks at the installer daemon PID).
func (s *extensionsSuite) TestDDOTConfigUpdateSurvivesPromote() {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		s.T().Skip("DDOT procmgr management is Linux-only")
	}

	s.Agent.MustInstall(agent.WithOTelCollectorEnabled())
	defer s.Agent.MustUninstall()

	// Baseline: DDOT auto-installed under dd-procmgr and reporting.
	ddot.AssertDDOTAutoInstallUnderProcmgr(s.T(), s.Env().RemoteHost)
	s.verifyDDOTRunning()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "ddot-config-promote",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/otel-config.yaml",
				Patch:             []byte(ddotOtelConfigBenignPatch),
			},
		},
	}, nil)
	s.Require().NoError(err)

	// During the experiment DDOT must still be running (under the experiment stack).
	ddot.AssertDDOTManagedByProcmgr(s.T(), s.Env().RemoteHost)
	s.verifyDDOTRunning()

	err = s.Backend.PromoteConfigExperiment()
	s.Require().NoError(err)

	// After promote DDOT must be restarted on the new stable config and keep reporting.
	// This is the crux of the bug: DDOT is left stopped and never restarts.
	ddot.AssertDDOTSystemdUnitsNotActive(s.T(), s.Env().RemoteHost)
	ddot.AssertDDOTManagedByProcmgr(s.T(), s.Env().RemoteHost)
	ddot.AssertProcmgrDDOTTelemetry(s.T(), s.Env().RemoteHost)
	s.verifyDDOTRunning()

	// The promoted otel-config.yaml carries the benign change.
	otelCfg, err := s.Env().RemoteHost.Execute("sudo cat /etc/datadog-agent/otel-config.yaml")
	s.Require().NoError(err)
	s.Require().Contains(otelCfg, "level: debug", "promoted otel-config.yaml should carry the experiment change")
}

// TestDDOTConfigUpdateRollback verifies that stopping (rolling back) an otel-config.yaml
// config experiment restores DDOT to a running state on the stable config.
func (s *extensionsSuite) TestDDOTConfigUpdateRollback() {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		s.T().Skip("DDOT procmgr management is Linux-only")
	}

	s.Agent.MustInstall(agent.WithOTelCollectorEnabled())
	defer s.Agent.MustUninstall()

	ddot.AssertDDOTAutoInstallUnderProcmgr(s.T(), s.Env().RemoteHost)
	s.verifyDDOTRunning()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "ddot-config-rollback",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/otel-config.yaml",
				Patch:             []byte(ddotOtelConfigBenignPatch),
			},
		},
	}, nil)
	s.Require().NoError(err)
	s.verifyDDOTRunning()

	err = s.Backend.StopConfigExperiment()
	s.Require().NoError(err)

	// After rollback DDOT must be restored and running on the stable config.
	ddot.AssertDDOTManagedByProcmgr(s.T(), s.Env().RemoteHost)
	ddot.AssertProcmgrDDOTTelemetry(s.T(), s.Env().RemoteHost)
	s.verifyDDOTRunning()
}
