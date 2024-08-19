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

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite

type packageTestsWithSkipedFlavors struct {
	t              packageTests
	skippedFlavors []e2eos.Descriptor
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
		{t: testApmInjectAgent, skippedFlavors: []e2eos.Descriptor{e2eos.CentOS7, e2eos.RedHat9, e2eos.Fedora37, e2eos.Suse15}},
		{t: testUpgradeScenario},
	}
)

func shouldSkip(flavors []e2eos.Descriptor, flavor e2eos.Descriptor) bool {
	for _, f := range flavors {
		if f.Flavor == flavor.Flavor && f.Version == flavor.Version {
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
			if shouldSkip(test.skippedFlavors, flavor) {
				continue
			}
			suite := test.t(flavor, flavor.Architecture)
			t.Run(suite.Name(), func(t *testing.T) {
				t.Parallel()
				// FIXME: Fedora currently has DNS issues
				if flavor.Flavor == e2eos.Fedora {
					flake.Mark(t)
				}
				if strings.Contains(suite.Name(), "upgrade") {
					// TODO(baptiste): Remove the flake once 0.4.7 is released
					// it is currently broken as we are changing the socket path
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

	opts []awshost.ProvisionerOption
	pkg  string
	arch e2eos.Architecture
	os   e2eos.Descriptor
}

func newPackageSuite(pkg string, os e2eos.Descriptor, arch e2eos.Architecture, opts ...awshost.ProvisionerOption) packageBaseSuite {
	return packageBaseSuite{
		os:   os,
		arch: arch,
		pkg:  pkg,
		opts: opts,
	}
}

func (s *packageBaseSuite) Name() string {
	return regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(fmt.Sprintf("%s/%s", s.pkg, s.os), "_")
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
	// bugfix for https://major.io/p/systemd-in-fedora-22-failed-to-restart-service-access-denied/
	if s.os.Flavor == e2eos.CentOS && s.os.Version == e2eos.CentOS7.Version {
		s.Env().RemoteHost.MustExecute("sudo systemctl daemon-reexec")
	}
	err := s.RunInstallScriptWithError(params...)
	require.NoErrorf(s.T(), err, "installer not properly installed. logs: \n%s\n%s", s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log"), s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log"))
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
