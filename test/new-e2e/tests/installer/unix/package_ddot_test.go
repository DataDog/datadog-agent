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
		var scriptURLPrefix string
		if pipelineID, ok := os.LookupEnv("E2E_PIPELINE_ID"); ok {
			scriptURLPrefix = fmt.Sprintf("https://s3.amazonaws.com/installtesting.datad0g.com/pipeline-%s/scripts/", pipelineID)
		} else if commitHash, ok := os.LookupEnv("CI_COMMIT_SHA"); ok {
			scriptURLPrefix = fmt.Sprintf("https://s3.amazonaws.com/installtesting.datad0g.com/%s/scripts/", commitHash)
		} else {
			require.FailNowf(nil, "missing script identifier", "CI_COMMIT_SHA or CI_PIPELINE_ID must be set")
		}
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
	// Install agent and DDOT together via environment variable
	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_OTELCOLLECTOR_ENABLED=true", envForceInstall("datadog-agent"))
	defer s.Purge()

	// Verify agent is installed
	s.host.AssertPackageInstalledByInstaller("datadog-agent")

	// Wait for services to be active
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, ddotUnit)

	state := s.host.State()
	s.assertCoreUnits(state, false)
	s.assertDDOTUnits(state, false)

	// Verify configuration files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")

	// Verify otelcollector configuration is present in datadog.yaml
	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
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
	// Verify running
	s.assertCoreUnits(state, true)
	s.assertDDOTUnits(state, false)

	// Verify files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")

	state.AssertDirExists("/opt/datadog-packages/datadog-agent-ddot/stable", 0755, "dd-agent", "dd-agent")
	state.AssertFileExists("/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", 0755, "dd-agent", "dd-agent")

	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
}

// TestInstallDDOTExtensionWithoutDatadogYAML verifies that when the datadog-agent package is
// installed via the package manager with DD_OTELCOLLECTOR_ENABLED=true, the postinst hook installs
// the DDOT extension and postInstallDDOTExtension falls back to DD_API_KEY / DD_SITE env vars to write
// otel-config.yaml since datadog.yaml is never created.
//
// The repo setup mirrors what install_script_agent7.sh does when TESTING_APT_URL,
// TESTING_KEYS_URL, TESTING_YUM_URL, and TESTING_YUM_VERSION_PATH are set.
func (s *packageDDOTSuite) TestInstallDDOTExtensionWithoutDatadogYAML() {
	testAPIKey := GetAPIKey()
	testSite := "datadoghq.com"
	defer s.Purge()

	env := InstallScriptEnvWithPackages(s.arch, PackagesConfig)
	env["DD_OTELCOLLECTOR_ENABLED"] = "true"
	env["DD_API_KEY"] = testAPIKey
	env["DD_SITE"] = testSite

	keysURL := env["TESTING_KEYS_URL"]

	switch s.host.GetPkgManager() {
	case "apt":
		aptURL := env["TESTING_APT_URL"]
		aptRepoVersion := env["TESTING_APT_REPO_VERSION"]
		keyring := "/usr/share/keyrings/datadog-archive-keyring.gpg"

		s.Env().RemoteHost.MustExecute(fmt.Sprintf(
			`sudo touch %s && sudo chmod a+r %s`, keyring, keyring))
		for _, key := range []string{
			"DATADOG_APT_KEY_CURRENT.public",
			"DATADOG_APT_KEY_F14F620E.public",
			"DATADOG_APT_KEY_382E94DE.public",
		} {
			s.Env().RemoteHost.MustExecute(fmt.Sprintf(
				`sudo curl --retry 5 -o /tmp/%s "https://%s/%s" && `+
					`cat /tmp/%s | sudo gpg --import --batch --no-default-keyring --keyring %s`,
				key, keysURL, key, key, keyring))
		}
		s.Env().RemoteHost.MustExecute(fmt.Sprintf(
			`echo "deb [signed-by=%s] https://%s/ %s" | sudo tee /etc/apt/sources.list.d/datadog.list`,
			keyring, aptURL, aptRepoVersion))
		s.Env().RemoteHost.MustExecute(
			`sudo apt-get update -o Dir::Etc::sourcelist="sources.list.d/datadog.list" ` +
				`-o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"`)
		s.Env().RemoteHost.MustExecute(`sudo -E apt-get install -y datadog-agent`, client.WithEnvVariables(env))

	case "yum":
		yumURL := env["TESTING_YUM_URL"]
		yumVersionPath := env["TESTING_YUM_VERSION_PATH"]
		archi := "x86_64"
		if s.arch == e2eos.ARM64Arch {
			archi = "aarch64"
		}
		for _, key := range []string{
			"DATADOG_RPM_KEY_CURRENT.public",
			"DATADOG_RPM_KEY_E09422B3.public",
			"DATADOG_RPM_KEY_FD4BF915.public",
		} {
			s.Env().RemoteHost.MustExecute(fmt.Sprintf(
				`sudo rpm --import "https://%s/%s"`, keysURL, key))
		}
		s.Env().RemoteHost.MustExecute(fmt.Sprintf(
			`sudo sh -c 'printf "[datadog]\nname = Datadog, Inc.\nbaseurl = https://%s/%s/%s/\nenabled=1\ngpgcheck=1\nrepo_gpgcheck=1\npriority=1\ngpgkey=https://%s/DATADOG_RPM_KEY_CURRENT.public\n       https://%s/DATADOG_RPM_KEY_E09422B3.public\n       https://%s/DATADOG_RPM_KEY_FD4BF915.public\n" > /etc/yum.repos.d/datadog.repo'`,
			yumURL, yumVersionPath, archi, keysURL, keysURL, keysURL))
		s.Env().RemoteHost.MustExecute(`sudo yum -y clean metadata`)
		s.Env().RemoteHost.MustExecute(`sudo -E yum install -y datadog-agent`, client.WithEnvVariables(env))

	case "zypper":
		yumURL := env["TESTING_YUM_URL"]
		yumVersionPath := env["TESTING_YUM_VERSION_PATH"]
		archi := "x86_64"
		if s.arch == e2eos.ARM64Arch {
			archi = "aarch64"
		}
		for _, key := range []string{
			"DATADOG_RPM_KEY_CURRENT.public",
			"DATADOG_RPM_KEY_E09422B3.public",
			"DATADOG_RPM_KEY_FD4BF915.public",
		} {
			s.Env().RemoteHost.MustExecute(fmt.Sprintf(
				`sudo rpm --import "https://%s/%s"`, keysURL, key))
		}
		s.Env().RemoteHost.MustExecute(fmt.Sprintf(
			`sudo sh -c 'printf "[datadog]\nname=datadog\nenabled=1\nbaseurl=https://%s/suse/%s/%s\ntype=rpm-md\ngpgcheck=1\nrepo_gpgcheck=1\ngpgkey=https://%s/DATADOG_RPM_KEY_CURRENT.public\n       https://%s/DATADOG_RPM_KEY_E09422B3.public\n       https://%s/DATADOG_RPM_KEY_FD4BF915.public\n" > /etc/zypp/repos.d/datadog.repo'`,
			yumURL, yumVersionPath, archi, keysURL, keysURL, keysURL))
		s.Env().RemoteHost.MustExecute(`sudo zypper --non-interactive --no-gpg-checks refresh datadog`)
		s.Env().RemoteHost.MustExecute(`sudo -E zypper --non-interactive install datadog-agent`, client.WithEnvVariables(env))

	default:
		s.T().Fatalf("unsupported package manager: %s", s.host.GetPkgManager())
	}

	state := s.host.State()

	// otel-config.yaml must exist with correct permissions and contain
	// the api_key and site sourced from env vars (datadog.yaml was never written).
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0640, "dd-agent", "dd-agent")
	s.host.Run(fmt.Sprintf("sudo grep -q '%s' /etc/datadog-agent/otel-config.yaml", testAPIKey))
	s.host.Run(fmt.Sprintf("sudo grep -q '%s' /etc/datadog-agent/otel-config.yaml", testSite))
	state.AssertPathDoesNotExist("/etc/datadog-agent/datadog.yaml")
}

func (s *packageDDOTSuite) assertCoreUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit)
	state.AssertUnitsEnabled(agentUnit)
	state.AssertUnitsRunning(agentUnit, traceUnit) //cannot assert process-agent and system-probe because they may be running or dead based on timing
	state.AssertUnitsDead(securityUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			if s.os.Flavor == e2eos.Ubuntu {
				// Ubuntu 24.04 moved to a new systemd path
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

	for _, unit := range []string{agentUnit, traceUnit, processUnit, probeUnit, securityUnit} {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

// Verify ddot service running
func (s *packageDDOTSuite) assertDDOTUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(ddotUnit)
	state.AssertUnitsRunning(ddotUnit)

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

	s.host.AssertUnitProperty(ddotUnit, "FragmentPath", filepath.Join(systemdPath, ddotUnit))
}
