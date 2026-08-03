// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

const (
	legacyProcmgrUnit = "datadog-agent-procmgrd.service"
)

type legacyProcmgrUpgradeSuite struct {
	packageBaseSuite
}

// TestLegacyProcmgrUpgradeContinuesOnRetirementFailure exercises the RPM upgrade path when
// SELinux blocks systemd from stopping legacy datadog-agent-procmgrd units during postinst.
// postinst must log a warning and continue installing current Agent units.
func TestLegacyProcmgrUpgradeContinuesOnRetirementFailure(t *testing.T) {
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		t.Skip("E2E_PIPELINE_ID env var is not set, this test requires a pipeline build")
	}

	flavor := e2eos.RedHat9
	flavor.Architecture = e2eos.AMD64Arch
	suite := &legacyProcmgrUpgradeSuite{
		packageBaseSuite: newPackageSuite(
			"legacy_procmgr_selinux",
			flavor,
			e2eos.AMD64Arch,
			InstallMethodInstallScript,
			awshost.WithRunOptions(scenec2.WithoutFakeIntake()),
		),
	}

	e2e.Run(t, suite,
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(scenec2.WithOSArch(flavor, flavor.Architecture)),
				scenec2.WithoutAgent(),
			),
		)),
		e2e.WithStackName(suite.Name()),
	)
}

func (s *legacyProcmgrUpgradeSuite) TestLegacyProcmgrUpgradeContinuesOnRetirementFailure() {
	defer s.Purge()

	s.requireSELinuxEnforcing()
	s.installProductionAgent(legacyProcmgrStableAgentVersion())

	if _, err := s.Env().RemoteHost.Execute("systemctl cat " + legacyProcmgrUnit); err != nil {
		s.T().Skipf("stable agent %s does not ship %s", legacyProcmgrStableAgentVersion(), legacyProcmgrUnit)
	}

	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, legacyProcmgrUnit)
	s.simulateSELinuxRunDirMislabel()

	timestamp := s.host.LastJournaldTimestamp()
	s.RunInstallScript("DD_REMOTE_UPDATES=true")

	s.host.AssertJournalContainsSubstring(timestamp, "failed to retire legacy procmgr units")
	s.host.AssertJournalContainsSubstring(timestamp, "Access denied")

	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)
	state := s.host.State()
	state.AssertUnitsNotLoaded(legacyProcmgrUnit)
	state.AssertUnitsLoaded(procmgrUnit)
	s.host.Run("sudo datadog-agent status")
}

func legacyProcmgrStableAgentVersion() string {
	if version := os.Getenv("STABLE_AGENT_ASSERT_VERSION"); version != "" {
		return version
	}
	return "7.79.2"
}

func (s *legacyProcmgrUpgradeSuite) requireSELinuxEnforcing() {
	enforce := strings.TrimSpace(s.host.Run("getenforce"))
	if enforce != "Enforcing" {
		s.T().Skipf("SELinux is %q, need Enforcing for this scenario", enforce)
	}
}

func (s *legacyProcmgrUpgradeSuite) installProductionAgent(version string) {
	env := map[string]string{
		"DD_API_KEY": GetAPIKey(),
		"DD_SITE":    "datadoghq.com",
	}
	cmd := fmt.Sprintf(
		"agent_version=%s bash -c \"$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)\"",
		version,
	)
	_, err := s.Env().RemoteHost.Execute(cmd, client.WithEnvVariables(env))
	require.NoErrorf(
		s.T(),
		err,
		"failed to install datadog-agent %s from production; stdout=%s stderr=%s",
		version,
		s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log || true"),
		s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log || true"),
	)
}

func (s *legacyProcmgrUpgradeSuite) simulateSELinuxRunDirMislabel() {
	s.host.Run("sudo mkdir -p /opt/datadog-agent/run")
	s.host.Run("sudo touch /opt/datadog-agent/run/process-agent.pid /opt/datadog-agent/run/system-probe.pid")
	_, err := s.Env().RemoteHost.Execute("sudo chcon -R -t usr_t /opt/datadog-agent/run")
	require.NoError(s.T(), err, "chcon usr_t on agent run dir (needs SELinux)")
}
