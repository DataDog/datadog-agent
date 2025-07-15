// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite

type packageTestsWithSkippedFlavors struct {
	t                          packageTests
	skippedFlavors             []e2eos.Descriptor
	skippedInstallationMethods []InstallMethodOption
}

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2404,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		// e2eos.FedoraDefault, // Skipped instead of marked as flaky to avoid useless logs
		e2eos.CentOS7,
		e2eos.Suse15,
	}
	arm64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2404,
		e2eos.AmazonLinux2,
		e2eos.Suse15,
	}
	packagesTestsWithSkippedFlavors = []packageTestsWithSkippedFlavors{
		{t: testAgent},
		{t: testApmInjectAgent, skippedFlavors: []e2eos.Descriptor{e2eos.CentOS7, e2eos.RedHat9, e2eos.FedoraDefault, e2eos.AmazonLinux2}, skippedInstallationMethods: []InstallMethodOption{InstallMethodAnsible}},
		{t: testUpgradeScenario, skippedInstallationMethods: []InstallMethodOption{InstallMethodAnsible}},
	}
)

const latestPython2AnsibleVersion = "5.10.0"

func shouldSkipFlavor(flavors []e2eos.Descriptor, flavor e2eos.Descriptor) bool {
	for _, f := range flavors {
		if f.Flavor == flavor.Flavor && f.Version == flavor.Version {
			return true
		}
	}
	return false
}

func shouldSkipInstallMethod(methods []InstallMethodOption, method InstallMethodOption) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func TestPackages(t *testing.T) {
	// INCIDENT(35594): This will match rate limits. Please remove me once this is fixed
	flake.MarkOnLogRegex(t, "error: read \"\\.pulumi/meta.yaml\":.*429")
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		t.Log("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
		t.FailNow()
	}

	method := GetInstallMethodFromEnv(t)
	var flavors []e2eos.Descriptor
	for _, flavor := range amd64Flavors {
		flavor.Architecture = e2eos.AMD64Arch
		flavors = append(flavors, flavor)
	}
	for _, flavor := range arm64Flavors {
		flavor.Architecture = e2eos.ARM64Arch
		flavors = append(flavors, flavor)
	}
	for _, f := range flavors {
		for _, test := range packagesTestsWithSkippedFlavors {
			flavor := f // capture range variable for parallel tests closure
			if shouldSkipFlavor(test.skippedFlavors, flavor) {
				continue
			}
			if shouldSkipInstallMethod(test.skippedInstallationMethods, method) {
				continue
			}
			// TODO: remove once ansible+suse is fully supported
			if flavor.Flavor == e2eos.Suse && method == InstallMethodAnsible {
				continue
			}

			suite := test.t(flavor, flavor.Architecture, method)
			t.Run(suite.Name(), func(t *testing.T) {
				t.Parallel()
				opts := []awshost.ProvisionerOption{
					awshost.WithEC2InstanceOptions(ec2.WithOSArch(flavor, flavor.Architecture)),
					awshost.WithoutAgent(),
				}
				opts = append(opts, suite.ProvisionerOptions()...)
				e2e.Run(t, suite,
					e2e.WithProvisioner(awshost.Provisioner(opts...)),
					e2e.WithStackName(suite.Name()),
				)
			})
		}
	}
}

type packageSuite interface {
	e2e.Suite[environments.Host]

	Name() string
	ProvisionerOptions() []awshost.ProvisionerOption
}

type packageBaseSuite struct {
	e2e.BaseSuite[environments.Host]
	host *host.Host

	opts          []awshost.ProvisionerOption
	pkg           string
	arch          e2eos.Architecture
	os            e2eos.Descriptor
	installMethod InstallMethodOption
}

func newPackageSuite(pkg string, os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption, opts ...awshost.ProvisionerOption) packageBaseSuite {
	return packageBaseSuite{
		os:            os,
		arch:          arch,
		pkg:           pkg,
		opts:          opts,
		installMethod: method,
	}
}

func (s *packageBaseSuite) Name() string {
	return regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(fmt.Sprintf("%s/%s/%s", s.pkg, s.os, s.installMethod), "_")
}

func (s *packageBaseSuite) ProvisionerOptions() []awshost.ProvisionerOption {
	return s.opts
}

func (s *packageBaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.setupFakeIntake()
	s.host = host.New(s.T, s.Env().RemoteHost, s.os, s.arch)
	s.disableUnattendedUpgrades()
	s.updateCurlOnUbuntu()
	s.updatePythonOnSuse()
}

func (s *packageBaseSuite) updatePythonOnSuse() {
	// Suse15 comes with Python3.6 by default which is too old for injection
	if s.os.Flavor != e2eos.Suse {
		return
	}
	s.host.Run("sudo zypper --non-interactive ar http://download.opensuse.org/distribution/leap/15.5/repo/oss/ oss || true")
	s.host.Run("sudo zypper --non-interactive --gpg-auto-import-keys in python311")
	s.host.Run("sudo ln -sf /usr/bin/python3.11 /usr/bin/python3")
}

func (s *packageBaseSuite) disableUnattendedUpgrades() {
	if _, err := s.Env().RemoteHost.Execute("which apt"); err == nil {
		// Try to disable unattended-upgrades to avoid interfering with the tests, it can fail if it is not installed, we ignore errors
		s.Env().RemoteHost.Execute("sudo apt remove -y unattended-upgrades") //nolint:errcheck
	}
}

func (s *packageBaseSuite) updateCurlOnUbuntu() {
	// There is an issue with the default cURL version on Ubuntu that causes sporadic
	// SSL failures, and the fix is to update it.
	// See https://stackoverflow.com/questions/72627218/openssl-error-messages-error0a000126ssl-routinesunexpected-eof-while-readin
	if s.os.Flavor == e2eos.Ubuntu {
		s.Env().RemoteHost.MustExecute("sudo apt update && sudo apt upgrade -y curl")
	}
}

func (s *packageBaseSuite) RunInstallScriptProdOci(params ...string) error {
	env := map[string]string{}
	installScriptPackageManagerEnv(env, s.arch)
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`%s bash -c "$(curl -L https://dd-agent.s3.amazonaws.com/scripts/install_script_agent7.sh)"`, strings.Join(params, " ")), client.WithEnvVariables(env))
	return err
}

func (s *packageBaseSuite) RunInstallScriptWithError(params ...string) error {
	hasRemoteUpdates := false
	for _, param := range params {
		if param == "DD_REMOTE_UPDATES=true" {
			hasRemoteUpdates = true
			break
		}
	}
	if hasRemoteUpdates {
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

	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`%s bash -c "$(curl -L https://dd-agent.s3.amazonaws.com/scripts/install_script_agent7.sh)"`, strings.Join(params, " ")), client.WithEnvVariables(InstallScriptEnv(s.arch)))
	return err
}

func (s *packageBaseSuite) RunInstallScript(params ...string) {
	switch s.installMethod {
	case InstallMethodInstallScript:
		// bugfix for https://major.io/p/systemd-in-fedora-22-failed-to-restart-service-access-denied/
		if s.os.Flavor == e2eos.CentOS && s.os.Version == e2eos.CentOS7.Version {
			s.Env().RemoteHost.MustExecute("sudo systemctl daemon-reexec")
		}
		err := s.RunInstallScriptWithError(params...)
		require.NoErrorf(s.T(), err, "installer not properly installed. logs: \n%s\n%s", s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log"), s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log"))
	case InstallMethodAnsible:
		// Install ansible then install the agent
		var ansiblePrefix string
		for i := 0; i < 3; i++ {
			var err error
			ansiblePrefix = s.installAnsible(s.os)
			if (s.os.Flavor == e2eos.AmazonLinux && s.os.Version == e2eos.AmazonLinux2.Version) ||
				(s.os.Flavor == e2eos.CentOS && s.os.Version == e2eos.CentOS7.Version) {
				_, err = s.Env().RemoteHost.Execute(fmt.Sprintf("%sansible-galaxy collection install -vvv datadog.dd:==%s", ansiblePrefix, latestPython2AnsibleVersion))
			} else {
				_, err = s.Env().RemoteHost.Execute(fmt.Sprintf("%sansible-galaxy collection install -vvv datadog.dd", ansiblePrefix))
			}
			if err == nil {
				break
			}
			if i == 2 {
				s.T().Fatal("failed to install ansible-galaxy collection after 3 attempts")
			}
			time.Sleep(time.Second)
		}

		// Write the playbook
		env := InstallScriptEnv(s.arch)
		playbookPath := s.writeAnsiblePlaybook(env, params...)

		// Run the playbook
		s.Env().RemoteHost.MustExecute(fmt.Sprintf("%sansible-playbook -vvv %s", ansiblePrefix, playbookPath))

		// touch install files for compatibility
		s.Env().RemoteHost.MustExecute("touch /tmp/datadog-installer-stdout.log")
		s.Env().RemoteHost.MustExecute("touch /tmp/datadog-installer-stderr.log")
	default:
		s.T().Fatal("unsupported install method")
	}
}

func envForceInstall(pkg string) string {
	return "DD_INSTALLER_DEFAULT_PKG_INSTALL_" + strings.ToUpper(strings.ReplaceAll(pkg, "-", "_")) + "=true"
}

func envForceNoInstall(pkg string) string {
	return "DD_INSTALLER_DEFAULT_PKG_INSTALL_" + strings.ToUpper(strings.ReplaceAll(pkg, "-", "_")) + "=false"
}

func envForceVersion(pkg, version string) string {
	return "DD_INSTALLER_DEFAULT_PKG_VERSION_" + strings.ToUpper(strings.ReplaceAll(pkg, "-", "_")) + "=" + version
}

func (s *packageBaseSuite) Purge() {
	// Reset the systemctl failed counter, best effort as they may not be loaded
	for _, service := range []string{agentUnit, agentUnitXP, traceUnit, traceUnitXP, processUnit, processUnitXP, probeUnit, probeUnitXP, securityUnit, securityUnitXP} {
		s.Env().RemoteHost.Execute(fmt.Sprintf("sudo systemctl reset-failed %s", service))
	}

	// Unfortunately no guarantee that the datadog-installer symlink exists
	s.Env().RemoteHost.Execute("sudo datadog-installer purge")
	s.Env().RemoteHost.Execute("sudo /opt/datadog-packages/datadog-installer/stable/bin/installer/installer purge")
	s.Env().RemoteHost.Execute("sudo /opt/datadog-packages/datadog-agent/stable/embedded/bin/installer purge")
	s.Env().RemoteHost.Execute("sudo apt-get remove -y --purge datadog-installer datadog-agent|| sudo yum remove -y datadog-installer datadog-agent || sudo zypper remove -y datadog-installer datadog-agent")
	s.Env().RemoteHost.Execute("sudo rm -rf /etc/datadog-agent")
}

// setupFakeIntake sets up the fake intake for the agent and trace agent.
// This is done with SystemD environment files overrides to avoid having to touch the agent configuration files
// and potentially interfere with the tests.
func (s *packageBaseSuite) setupFakeIntake() {
	var env []string
	if s.Env().FakeIntake != nil {
		env = append(env, []string{
			"DD_SKIP_SSL_VALIDATION=true",
			"DD_URL=" + s.Env().FakeIntake.URL,
			"DD_APM_DD_URL=" + s.Env().FakeIntake.URL,
		}...)
	}
	for _, e := range env {
		s.Env().RemoteHost.MustExecute(fmt.Sprintf(`echo "%s" | sudo tee -a /etc/environment`, e))
	}

	if _, err := s.Env().RemoteHost.Execute("which systemctl"); err != nil {
		// If systemctl isn't on the system we rely on /etc/environment being read
		return
	}

	s.Env().RemoteHost.MustExecute("sudo mkdir -p /etc/systemd/system/datadog-agent.service.d")
	s.Env().RemoteHost.MustExecute("sudo mkdir -p /etc/systemd/system/datadog-agent-trace.service.d")
	s.Env().RemoteHost.MustExecute(`printf "[Service]\nEnvironmentFile=-/etc/environment\n" | sudo tee /etc/systemd/system/datadog-agent-trace.service.d/fake-intake.conf`)
	s.Env().RemoteHost.MustExecute(`printf "[Service]\nEnvironmentFile=-/etc/environment\n" | sudo tee /etc/systemd/system/datadog-agent-trace.service.d/fake-intake.conf`)
	s.Env().RemoteHost.MustExecute("sudo systemctl daemon-reload")
}

func (s *packageBaseSuite) installAnsible(flavor e2eos.Descriptor) string {
	pathPrefix := ""
	switch flavor.Flavor {
	case e2eos.Ubuntu, e2eos.Debian:
		s.Env().RemoteHost.MustExecute("sudo apt update && sudo apt install -y ansible")
	case e2eos.Fedora:
		s.Env().RemoteHost.MustExecute("sudo dnf install -y ansible")
	case e2eos.CentOS:
		// Can't install ansible with yum install because the available package on centos is max ansible 2.9, EOL since May 2022
		s.Env().RemoteHost.MustExecute("sudo yum install -y python3 curl")
		s.Env().RemoteHost.MustExecute("curl https://bootstrap.pypa.io/pip/3.6/get-pip.py -o get-pip.py && python3 get-pip.py && rm get-pip.py")
		s.Env().RemoteHost.MustExecute("python3 -m pip install ansible")
		pathPrefix = "/home/centos/.local/bin/"
	case e2eos.AmazonLinux, e2eos.RedHat:
		s.Env().RemoteHost.MustExecute("sudo yum install -y python3 python3-pip && yes | pip3 install ansible")
		pathPrefix = "/home/ec2-user/.local/bin/"
	case e2eos.Suse:
		s.Env().RemoteHost.MustExecute("sudo zypper install -y python3 python3-pip && sudo pip3 install ansible")
	default:
		s.Env().RemoteHost.MustExecute("python3 -m ensurepip --upgrade && python3 -m pip install pipx && python3 -m pipx ensurepath")
		pathPrefix = "/usr/bin/"
	}

	return pathPrefix
}

func (s *packageBaseSuite) writeAnsiblePlaybook(env map[string]string, params ...string) string {
	playbookPath := "/tmp/datadog-agent-playbook.yml"
	playbookStringPrefix := `
- hosts: localhost
  tasks:
    - name: Import the Datadog Agent role from the Datadog collection
      become: true
      retries: 3
      import_role:
        name: datadog.dd.agent
`
	playbookStringSuffix := `
  vars:
    datadog_api_key: "abcdef"
    datadog_site: "datadoghq.com"
`

	defaultRepoEnv := map[string]string{
		// APT
		"TESTING_APT_KEY":          "/usr/share/keyrings/datadog-archive-keyring.gpg",
		"TESTING_APT_URL":          "apt.datadoghq.com",
		"TESTING_APT_REPO_VERSION": "",
		// YUM
		"TESTING_YUM_URL":          "yum.datadoghq.com",
		"TESTING_YUM_VERSION_PATH": "",
	}
	mergedParams := make([]string, len(params))
	copy(mergedParams, params)
	for k, v := range env {
		mergedParams = append(mergedParams, fmt.Sprintf("%s=%s", k, v))
	}

	environments := []string{}
	for _, param := range mergedParams {
		key, value := strings.Split(param, "=")[0], strings.Split(param, "=")[1]
		switch key {
		case "DD_REMOTE_UPDATES":
			playbookStringSuffix += fmt.Sprintf("    datadog_remote_updates: %s\n", value)
		case "DD_APM_INSTRUMENTATION_ENABLED":
			playbookStringSuffix += fmt.Sprintf("    datadog_apm_instrumentation_enabled: \"%s\"\n", value)
		case "DD_APM_INSTRUMENTATION_LIBRARIES":
			playbookStringSuffix += fmt.Sprintf("    datadog_apm_instrumentation_libraries: [%s]\n", value)
		case "DD_INSTALLER":
			playbookStringSuffix += fmt.Sprintf("    datadog_installer_enabled: %s\n", value)
		case "DD_INSTALLER_REGISTRY_AUTH_INSTALLER_PACKAGE":
			playbookStringSuffix += fmt.Sprintf("    datadog_installer_auth: %s\n", value)
			environments = append(environments, fmt.Sprintf("%s: %s", key, value))
		case "DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE":
			playbookStringSuffix += fmt.Sprintf("    datadog_installer_registry: %s\n", value)
			environments = append(environments, fmt.Sprintf("%s: %s", key, value))
		case "TESTING_APT_REPO_VERSION", "TESTING_APT_URL", "TESTING_APT_KEY", "TESTING_YUM_URL", "TESTING_YUM_VERSION_PATH":
			defaultRepoEnv[key] = value
			environments = append(environments, fmt.Sprintf("%s: %s", key, value))
		case "DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER":
			playbookStringSuffix += fmt.Sprintf("    datadog_installer_version: %s\n", value)
			environments = append(environments, fmt.Sprintf("%s: \"%s\"", key, value))
		case "DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT":
			playbookStringSuffix += fmt.Sprintf("    datadog_apm_inject_version: %s\n", value)
			environments = append(environments, fmt.Sprintf("%s: \"%s\"", key, value))
		default:
			environments = append(environments, fmt.Sprintf("%s: \"%s\"", key, value))
		}
	}
	if defaultRepoEnv["TESTING_APT_REPO_VERSION"] != "" {
		playbookStringSuffix += fmt.Sprintf("    datadog_apt_repo: \"deb [signed-by=%s] https://%s/ %s\"\n", defaultRepoEnv["TESTING_APT_KEY"], defaultRepoEnv["TESTING_APT_URL"], defaultRepoEnv["TESTING_APT_REPO_VERSION"])
	}
	if defaultRepoEnv["TESTING_YUM_VERSION_PATH"] != "" {
		archi := "x86_64"
		if s.arch == e2eos.ARM64Arch {
			archi = "aarch64"
		}
		playbookStringSuffix += fmt.Sprintf("    datadog_yum_repo: \"https://%s/%s/%s/\"\n", defaultRepoEnv["TESTING_YUM_URL"], defaultRepoEnv["TESTING_YUM_VERSION_PATH"], archi)
	}
	if len(environments) > 0 {
		playbookStringPrefix += "      environment:\n"
		for _, env := range environments {
			playbookStringPrefix += fmt.Sprintf("        %s\n", env)
		}
	}

	playbookString := playbookStringPrefix + playbookStringSuffix

	// Write the playbook to a file
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("echo '%s' | sudo tee %s", playbookString, playbookPath))

	return playbookPath
}
