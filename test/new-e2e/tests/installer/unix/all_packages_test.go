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

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite

type packageTestsWithSkipedFlavors struct {
	t                          packageTests
	skippedFlavors             []e2eos.Descriptor
	skippedInstallationMethods []InstallMethodOption
}

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		e2eos.Fedora37,
		e2eos.CentOS7,
		e2eos.Suse15,
	}
	arm64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		e2eos.Suse15,
	}
	packagesTestsWithSkippedFlavors = []packageTestsWithSkipedFlavors{
		{t: testInstaller},
		{t: testAgent},
		{t: testApmInjectAgent, skippedFlavors: []e2eos.Descriptor{e2eos.CentOS7, e2eos.RedHat9, e2eos.Fedora37, e2eos.Suse15}, skippedInstallationMethods: []InstallMethodOption{InstallMethodAnsible}},
		{t: testUpgradeScenario},
	}
)

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
				// FIXME: Fedora currently has DNS issues
				if flavor.Flavor == e2eos.Fedora {
					flake.Mark(t)
				}

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
	s.setupFakeIntake()
	s.host = host.New(s.T(), s.Env().RemoteHost, s.os, s.arch)
	s.disableUnattendedUpgrades()
}

func (s *packageBaseSuite) disableUnattendedUpgrades() {
	if _, err := s.Env().RemoteHost.Execute("which apt"); err == nil {
		s.Env().RemoteHost.MustExecute("sudo apt remove -y unattended-upgrades")
	}
}

func (s *packageBaseSuite) RunInstallScriptProdOci(params ...string) error {
	env := map[string]string{}
	installScriptPackageManagerEnv(env, s.arch)
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`%s bash -c "$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)"`, strings.Join(params, " ")), client.WithEnvVariables(env))
	return err
}

func (s *packageBaseSuite) RunInstallScriptWithError(params ...string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`%s bash -c "$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)"`, strings.Join(params, " ")), client.WithEnvVariables(InstallScriptEnv(s.arch)))
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
		ansiblePrefix := s.installAnsible(s.os)

		s.Env().RemoteHost.MustExecute(fmt.Sprintf("%sansible-galaxy collection install -vvv datadog.dd", ansiblePrefix))

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

	s.Env().RemoteHost.MustExecute("sudo apt-get remove -y --purge datadog-installer || sudo yum remove -y datadog-installer || sudo zypper remove -y datadog-installer")
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
