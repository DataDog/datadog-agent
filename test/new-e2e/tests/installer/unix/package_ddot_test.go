// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/internal/procmgrtest"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageDDOTSuite struct {
	packageBaseSuite
}

func testDDOT(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageDDOTSuite{
		packageBaseSuite: newPackageSuite("ddot", os, arch, method, awshost.WithRunOptions(scenec2.WithoutFakeIntake())),
	}
}

func installScriptParamsUseOTelCollector(params []string) bool {
	for _, param := range params {
		if param == "DD_OTELCOLLECTOR_ENABLED=true" {
			return true
		}
	}
	return false
}

func (s *packageDDOTSuite) RunInstallScriptWithError(params ...string) error {
	hasOTelCollector := installScriptParamsUseOTelCollector(params)
	if hasOTelCollector {
		// This is temporary until the install script is updated to support calling the installer script
		scriptURLPrefix := "https://" + InstallerScriptBaseURL() + "/scripts/"
		_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`%s bash -c "$(curl -L %sinstall.sh)" > /tmp/datadog-installer-stdout.log 2> /tmp/datadog-installer-stderr.log`, strings.Join(params, " "), scriptURLPrefix), client.WithEnvVariables(InstallInstallerScriptEnvWithPackages()))
		return err
	}

	_, err := s.Env().RemoteHost.Execute(strings.Join(params, " ")+" bash -c \"$(curl -L https://dd-agent.s3.amazonaws.com/scripts/install_script_agent7.sh)\"", client.WithEnvVariables(InstallScriptEnv(s.arch)))
	return err
}

func (s *packageDDOTSuite) RunInstallScript(params ...string) {
	switch s.installMethod {
	case InstallMethodInstallScript:
		// bugfix for https://major.io/p/systemd-in-fedora-22-failed-to-restart-service-access-denied/
		if s.os.Flavor == e2eos.CentOS && s.os.Version == e2eos.CentOS7.Version {
			s.Env().RemoteHost.MustExecute("sudo systemctl daemon-reexec")
		}
		err := s.RunInstallScriptWithError(params...)
		require.NoErrorf(s.T(), err, "installer not properly installed. logs: \n%s\n%s", s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log || true"), s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log || true"))
	default:
		s.T().Fatal("unsupported install method")
	}
}

func (s *packageDDOTSuite) TestInstallDDOTInstallScript() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_OTELCOLLECTOR_ENABLED=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)

	state := s.host.State()
	s.assertCoreUnits(state, false)

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")

	s.host.Run(`sudo sh -c 'grep -A30 "otelcollector:" /etc/datadog-agent/datadog.yaml | grep -qE "[[:space:]]*enabled:[[:space:]]*true"'`)

	s.waitForDDOTRunning(procmgrtest.CLIBinFleetStable, procmgrtest.DDOTOtelAgentFleetPackageBinary)
}

func (s *packageDDOTSuite) TestInstallDDOTInstaller() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)

	s.host.Run("sudo datadog-installer install oci://installtesting.datad0g.com.internal.dda-testing.com/ddot-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID"))
	s.host.AssertPackageInstalledByInstaller("datadog-agent-ddot")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)

	state := s.host.State()
	s.assertCoreUnits(state, true)

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")
	s.host.Run(`sudo sh -c 'grep -A30 "otelcollector:" /etc/datadog-agent/datadog.yaml | grep -qE "[[:space:]]*enabled:[[:space:]]*true"'`)

	s.waitForDDOTRunning(procmgrtest.CLIBinFleetStable, procmgrtest.DDOTOtelAgentFleetPackageBinary)
}

func (s *packageDDOTSuite) TestInstallDDOTWithoutDatadogYAML() {
	testAPIKey := GetAPIKey()
	testSite := "datadoghq.com"
	defer s.Purge()

	// Step 1: install the agent via the standard install script.
	// This creates /etc/datadog-agent/datadog.yaml.
	s.RunInstallScript()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")

	// Step 2: remove the agent package while keeping config files.
	// apt-get remove (not purge) preserves /etc/datadog-agent/.
	switch s.host.GetPkgManager() {
	case "apt":
		s.Env().RemoteHost.MustExecute("sudo apt-get remove -y datadog-agent")
	case "yum":
		s.Env().RemoteHost.MustExecute("sudo yum remove -y datadog-agent")
	case "zypper":
		s.Env().RemoteHost.MustExecute("sudo zypper remove -y datadog-agent")
	default:
		s.T().Fatalf("unsupported package manager: %s", s.host.GetPkgManager())
	}

	// Step 3: move datadog.yaml out of the way so the reinstall has no yaml.
	s.Env().RemoteHost.MustExecute("sudo mv /etc/datadog-agent/datadog.yaml /etc/datadog-agent/datadog.yaml.bak")

	// Step 4: reinstall via the package manager with DD_OTELCOLLECTOR_ENABLED=true.
	// The repos are already configured by the install script in step 1.
	// The full env from InstallScriptEnvWithPackages is required because the postinst
	// hook downloads the DDOT extension via OCI and needs the registry/version overrides.
	env := InstallScriptEnvWithPackages(s.arch, PackagesConfig)
	env["DD_OTELCOLLECTOR_ENABLED"] = "true"
	env["DD_API_KEY"] = testAPIKey
	env["DD_SITE"] = testSite
	switch s.host.GetPkgManager() {
	case "apt":
		s.Env().RemoteHost.MustExecute("sudo -E apt-get install -y datadog-agent", client.WithEnvVariables(env))
	case "yum":
		s.Env().RemoteHost.MustExecute("sudo -E yum install -y datadog-agent", client.WithEnvVariables(env))
	case "zypper":
		s.Env().RemoteHost.MustExecute("sudo -E zypper --non-interactive install datadog-agent", client.WithEnvVariables(env))
	default:
		s.T().Fatalf("unsupported package manager: %s", s.host.GetPkgManager())
	}

	// Step 5: datadog-agent and procmgr stay inactive when datadog.yaml is missing, so ddot must also remain stopped.
	state := s.host.State()
	state.AssertUnitsDead(agentUnit, procmgrUnit, ddotUnit)

	// Step 6: otel-config.yaml must exist and contain the api_key and site from env vars.
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")
	s.host.Run(fmt.Sprintf("sudo grep -q '%s' /etc/datadog-agent/otel-config.yaml", testAPIKey))
	s.host.Run(fmt.Sprintf("sudo grep -q '%s' /etc/datadog-agent/otel-config.yaml", testSite))
	state.AssertPathDoesNotExist("/etc/datadog-agent/datadog.yaml")

	// Step 7: restore datadog.yaml and append the otelcollector activation stanza.
	s.Env().RemoteHost.MustExecute("sudo mv /etc/datadog-agent/datadog.yaml.bak /etc/datadog-agent/datadog.yaml")
	s.Env().RemoteHost.MustExecute(`sudo sh -c "printf 'otelcollector:\n  enabled: true\n  agent_ipc:\n    port: 5009\n    config_refresh_interval: 60\n' >> /etc/datadog-agent/datadog.yaml"`)

	// Step 8: restart the agent so it picks up the updated configuration.
	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent.service")

	// Step 9: verify the agent and ddot are both running.
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)
	state = s.host.State()
	s.assertCoreUnits(state, true)
	s.waitForDDOTRunning(procmgrtest.CLIBinFleetStable, procmgrtest.DDOTOtelAgentExtensionBinary)
}

func (s *packageDDOTSuite) TestInstallDDOTSubcommand() {
	// Install the base agent without DDOT.
	s.RunInstallScript()
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

	// Install the ddot extension via the new datadog-agent otel subcommand.
	agentPackageURL := "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID")
	s.host.Run("sudo datadog-agent otel install --url " + agentPackageURL)

	// DDOT is process-manager-managed; wait for core services including procmgr.
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)

	state := s.host.State()
	s.assertCoreUnits(state, true)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")
	s.host.Run(`sudo sh -c 'grep -A30 "otelcollector:" /etc/datadog-agent/datadog.yaml | grep -qE "[[:space:]]*enabled:[[:space:]]*true"'`)
	state.AssertFileExists(procmgrtest.DDOTOtelAgentExtensionBinary, 0755, "dd-agent", "dd-agent")
	s.waitForDDOTRunning(procmgrtest.CLIBinDefault, procmgrtest.DDOTOtelAgentExtensionBinary)

	// Remove the ddot extension and verify the service stops.
	s.host.Run("sudo datadog-agent otel remove")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)
	state = s.host.State()
	s.assertCoreUnits(state, true)
}

func (s *packageDDOTSuite) assertCoreUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit, procmgrUnit)
	state.AssertUnitsEnabled(agentUnit)
	// Cannot assert process-agent and system-probe state: they may be running or dead based on timing.
	state.AssertUnitsRunning(agentUnit, traceUnit, procmgrUnit)
	state.AssertUnitsDead(securityUnit, ddotUnit)
	s.assertUnitFragmentPaths(oldUnits, agentUnit, traceUnit, processUnit, probeUnit, securityUnit, procmgrUnit)
}

func (s *packageDDOTSuite) assertUnitFragmentPaths(oldUnits bool, units ...string) {
	systemdPath := s.systemdUnitDir(oldUnits)
	for _, unit := range units {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

func (s *packageDDOTSuite) systemdUnitDir(oldUnits bool) string {
	if !oldUnits {
		return "/etc/systemd/system"
	}
	pkgManager := s.host.GetPkgManager()
	switch pkgManager {
	case "apt":
		if s.os.Flavor == e2eos.Ubuntu {
			// Ubuntu 24.04 moved to a new systemd path
			return "/usr/lib/systemd/system"
		}
		return "/lib/systemd/system"
	case "yum", "zypper":
		return "/usr/lib/systemd/system"
	default:
		s.T().Fatalf("unsupported package manager: %s", pkgManager)
		return ""
	}
}

// waitForDDOTRunning waits until dd-procmgr reports DDOT as Running with the given CLI path and expected binary.
func (s *packageDDOTSuite) waitForDDOTRunning(cliBin, expectedBinary string) procmgrtest.WaitForProcessResult {
	s.T().Helper()
	return procmgrtest.WaitForProcess(s.T(), s, procmgrtest.WaitForProcessArgs{
		ProcmgrCLIBin:  cliBin,
		ProcessName:    procmgrtest.DDOTProcessName,
		ExpectedBinary: expectedBinary,
		DesiredState:   procmgrtest.ProcessStateRunning,
	})
}

func (s *packageDDOTSuite) ExecuteCommand(command string) (string, error) {
	return s.Env().RemoteHost.Execute(command)
}
