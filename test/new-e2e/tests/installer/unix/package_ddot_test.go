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

func (s *packageDDOTSuite) ExecuteCommand(command string) (string, error) {
	return s.Env().RemoteHost.Execute(command)
}

func testDDOT(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageDDOTSuite{
		packageBaseSuite: newPackageSuite("ddot", os, arch, method, awshost.WithRunOptions(scenec2.WithoutFakeIntake())),
	}
}

func (s *packageDDOTSuite) RunInstallScriptWithError(params ...string) error {
	hasOTelCollector := false
	for _, param := range params {
		if param == "DD_OTELCOLLECTOR_ENABLED=true" {
			hasOTelCollector = true
			break
		}
	}
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
	procmgrtest.WaitForDDOTRunning(s.T(), s, procmgrtest.DDOTOtelAgentFleetStableExtensionBinary)

	state := s.host.State()
	s.assertCoreUnits(state, false)
	s.assertDDOTUnitsProcmgr(state, false, false)

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")

	s.host.Run("sudo grep -A 30 '^otelcollector:' /etc/datadog-agent/datadog.yaml | grep -qE '^[[:space:]]*enabled:[[:space:]]*true[[:space:]]*$'")
}

func (s *packageDDOTSuite) TestInstallDDOTInstaller() {
	// Install datadog-agent (base infrastructure)
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

	// Install ddot
	s.host.Run("sudo datadog-installer install oci://installtesting.datad0g.com.internal.dda-testing.com/ddot-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID"))
	s.host.AssertPackageInstalledByInstaller("datadog-agent-ddot")

	// Check if datadog.yaml exists, if not return an error
	s.host.Run("sudo test -f /etc/datadog-agent/datadog.yaml || { echo 'Error: datadog.yaml does not exist'; exit 1; }")

	s.host.WaitForUnitActive(s.T(), ddotUnit)

	state := s.host.State()
	s.assertCoreUnits(state, true)
	s.assertDDOTUnits(state, false)

	// Verify files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")

	state.AssertDirExists("/opt/datadog-packages/datadog-agent-ddot/stable", 0755, "dd-agent", "dd-agent")
	state.AssertFileExists("/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", 0755, "dd-agent", "dd-agent")

	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
}

func (s *packageDDOTSuite) TestInstallDDOTWithoutDatadogYAML() {
	testAPIKey := GetAPIKey()
	testSite := "datadoghq.com"
	defer s.Purge()

	// Step 1: install the agent via the standard install script
	// and creates /etc/datadog-agent/datadog.yaml.
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

	// Step 5: ddot must NOT have started — there is no datadog.yaml to enable it.
	state := s.host.State()
	state.AssertUnitsDead(ddotUnit)

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

	// Step 9: with datadog.yaml restored, DDOT runs under dd-procmgr (ddot systemd unit stays inactive).
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)
	// Install-script host: fleet stable procmgr + extension paths (step 1), same as TestInstallDDOTInstallScript.
	procmgrtest.WaitForDDOTRunning(s.T(), s, procmgrtest.DDOTOtelAgentFleetStableExtensionBinary)
	state = s.host.State()
	s.assertCoreUnits(state, true)
	s.assertDDOTUnitsProcmgr(state, true, true)
}

func (s *packageDDOTSuite) TestInstallDDOTSubcommand() {
	s.RunInstallScript()
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)

	agentPackageURL := "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID")
	s.host.Run("sudo datadog-agent otel install --url " + agentPackageURL)

	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)
	// Install-script host: extension runs from fleet stable after otel install (same as other ddot install_script tests).
	procmgrtest.WaitForDDOTRunning(s.T(), s, procmgrtest.DDOTOtelAgentFleetStableExtensionBinary)

	state := s.host.State()
	s.assertCoreUnits(state, true)
	s.assertDDOTUnitsProcmgr(state, true, true)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")
	s.host.Run("sudo grep -A 30 '^otelcollector:' /etc/datadog-agent/datadog.yaml | grep -qE '^[[:space:]]*enabled:[[:space:]]*true[[:space:]]*$'")

	s.host.Run("sudo datadog-agent otel remove")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, procmgrUnit)
	state = s.host.State()
	state.AssertUnitsDead(ddotUnit)
	s.assertCoreUnits(state, true)
}

// systemdUnitFragmentDir maps oldUnits to the directory used for FragmentPath (deb/rpm agent layout).
func (s *packageDDOTSuite) systemdUnitFragmentDir(oldUnits bool) string {
	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			if s.os.Flavor == e2eos.Ubuntu {
				systemdPath = "/usr/lib/systemd/system"
			} else {
				systemdPath = "/lib/systemd/system"
			}
		case "yum", "zypper":
			systemdPath = "/usr/lib/systemd/system"
		default:
			s.T().Fatalf("unsupported package manager: %s", pkgManager)
		}
	}
	return systemdPath
}

func (s *packageDDOTSuite) assertCoreUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit)
	state.AssertUnitsEnabled(agentUnit)
	state.AssertUnitsRunning(agentUnit, traceUnit) // process/sysprobe timing-dependent
	state.AssertUnitsDead(securityUnit)

	systemdPath := s.systemdUnitFragmentDir(oldUnits)

	for _, unit := range []string{agentUnit, traceUnit, processUnit, probeUnit, securityUnit} {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

// assertDDOTUnits verifies the legacy datadog-agent-ddot systemd unit (standalone package install).
func (s *packageDDOTSuite) assertDDOTUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(ddotUnit)
	state.AssertUnitsRunning(ddotUnit)

	ddotPath := s.systemdUnitFragmentDir(oldUnits)
	s.host.AssertUnitProperty(ddotUnit, "FragmentPath", filepath.Join(ddotPath, ddotUnit))
}

// assertDDOTUnitsProcmgr: procmgr running, ddot systemd inactive (collector under dd-procmgr), FragmentPath per oldUnits flags (OCI ddot unit often needs ddotOldUnits=false).
func (s *packageDDOTSuite) assertDDOTUnitsProcmgr(state host.State, procmgrOldUnits, ddotOldUnits bool) {
	state.AssertUnitsLoaded(procmgrUnit)
	state.AssertUnitsRunning(procmgrUnit)

	state.AssertUnitsLoaded(ddotUnit)
	state.AssertUnitsDead(ddotUnit)

	procmgrPath := s.systemdUnitFragmentDir(procmgrOldUnits)
	ddotPath := s.systemdUnitFragmentDir(ddotOldUnits)
	s.host.AssertUnitProperty(procmgrUnit, "FragmentPath", filepath.Join(procmgrPath, procmgrUnit))
	s.host.AssertUnitProperty(ddotUnit, "FragmentPath", filepath.Join(ddotPath, ddotUnit))
}
