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
	"slices"
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
	"gopkg.in/yaml.v2"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite

type packageTestsWithSkipedFlavors struct {
	t              packageTests
	skippedFlavors []e2eos.Descriptor
}

type testPackageConfig struct {
	name           string
	defaultVersion string
	registry       string
	auth           string
}

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		e2eos.Fedora37,
		e2eos.CentOS7,
		// e2eos.Suse15,
	}
	arm64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		// e2eos.Suse15,
	}
	packagesTestsWithSkipedFlavors = []packageTestsWithSkipedFlavors{
		{t: testInstaller},
		{t: testAgent},
		{t: testApmInjectAgent, skippedFlavors: []e2eos.Descriptor{e2eos.CentOS7, e2eos.RedHat9, e2eos.Fedora37}},
	}
)

var (
	packagesConfig = []testPackageConfig{
		{name: "datadog-installer", defaultVersion: fmt.Sprintf("pipeline-%v", os.Getenv("CI_PIPELINE_ID")), registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", auth: "ecr"},
		{name: "datadog-agent", defaultVersion: fmt.Sprintf("pipeline-%v", os.Getenv("CI_PIPELINE_ID")), registry: "669783387624.dkr.ecr.us-east-1.amazonaws.com", auth: "ecr"},
		{name: "apm-inject", defaultVersion: "latest", registry: "gcr.io/datadoghq", auth: "docker"},
	}
)

func TestPackages(t *testing.T) {
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
		for _, test := range packagesTestsWithSkipedFlavors {
			flavor := f // capture range variable for parallel tests closure
			if slices.Contains(test.skippedFlavors, flavor) {
				continue
			}
			suite := test.t(flavor, flavor.Architecture)
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

	env  map[string]string
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
		env:  env(arch),
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
	s.host = host.New(s.T(), s.Env().RemoteHost, s.os, s.arch)
}

func (s *packageBaseSuite) RunInstallScript() {
	// fixme: use the official install
	s.Env().RemoteHost.MustExecute(`bash -c "$(curl -L https://storage.googleapis.com/updater-dev/install_script_agent7.sh)"`, client.WithEnvVariables(s.env))
	datadogConfig := map[string]interface{}{
		"api_key": "deadbeefdeadbeefdeadbeefdeadbeef",
	}
	if s.Env().FakeIntake != nil {
		datadogConfig["dd_url"] = s.Env().FakeIntake.URL
		datadogConfig["skip_ssl_validation"] = true
		datadogConfig["apm_config"] = map[string]interface{}{
			"apm_dd_url": s.Env().FakeIntake.URL,
		}
	}
	rawDatadogConfig, err := yaml.Marshal(datadogConfig)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("echo '%s' | sudo tee /etc/datadog-agent/datadog.yaml", string(rawDatadogConfig)))
	_, err = s.Env().RemoteHost.Execute("sudo datadog-installer version")
	// Right now the install script can fail installing the installer silently, so we need to do this check or it will fail later in a way that is hard to debug
	require.NoErrorf(s.T(), err, "installer not properly installed. logs: \n%s\n%s", s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log"), s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log"))
}

func (s *packageBaseSuite) InstallAgentPackage() {
	s.Env().RemoteHost.MustExecute(`sudo -E datadog-installer install oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-package:`+fmt.Sprintf("pipeline-%v", os.Getenv("CI_PIPELINE_ID")), client.WithEnvVariables(s.env))
	s.Env().RemoteHost.MustExecute(`timeout=30; unit=datadog-agent.service; while ! systemctl is-active --quiet $unit && [ $timeout -gt 0 ]; do sleep 1; ((timeout--)); done; [ $timeout -ne 0 ]`)
}

func (s *packageBaseSuite) InstallPackageLatest(pkg string) {
	require.NoError(s.T(), s.InstallPackageLatestWithError(pkg))
}

func (s *packageBaseSuite) InstallPackageLatestWithError(pkg string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`sudo -E datadog-installer install oci://gcr.io/datadoghq/%s-package:latest`, strings.TrimPrefix(pkg, "datadog-")), client.WithEnvVariables(s.env))
	return err
}

func (s *packageBaseSuite) Purge() {
	s.Env().RemoteHost.MustExecute("sudo apt-get remove -y --purge datadog-installer || sudo yum remove -y datadog-installer")
	s.Env().RemoteHost.MustExecute("sudo rm -rf /etc/datadog-agent")
}

func (s *packageBaseSuite) BootstraperVersion() string {
	return strings.TrimSpace(s.Env().RemoteHost.MustExecute("sudo datadog-bootstrap version"))
}

func (s *packageBaseSuite) InstallerVersion() string {
	return strings.TrimSpace(s.Env().RemoteHost.MustExecute("sudo datadog-installer version"))
}

func env(arch e2eos.Architecture) map[string]string {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	env := map[string]string{
		"DD_API_KEY": apiKey,
		"DD_SITE":    "datadoghq.com",
		// Install Script env variables
		"DD_INSTALLER":             "true",
		"DD_NO_AGENT_INSTALL":      "true",
		"TESTING_KEYS_URL":         "keys.datadoghq.com",
		"TESTING_APT_URL":          "apttesting.datad0g.com",
		"TESTING_APT_REPO_VERSION": fmt.Sprintf("pipeline-%s-i7-%s 7", os.Getenv("CI_PIPELINE_ID"), arch),
		"TESTING_YUM_URL":          "yumtesting.datad0g.com",
		"TESTING_YUM_VERSION_PATH": fmt.Sprintf("testing/pipeline-%s-i7/7", os.Getenv("CI_PIPELINE_ID")),
	}
	for _, pkg := range packagesConfig {
		name := strings.ToUpper(strings.ReplaceAll(pkg.name, "-", "_"))
		image := strings.TrimPrefix(name, "DATADOG_") + "_PACKAGE"
		env[fmt.Sprintf("DD_INSTALLER_REGISTRY_%s", image)] = pkg.registry
		env[fmt.Sprintf("DD_INSTALLER_REGISTRY_AUTH_%s", image)] = pkg.auth
		env[fmt.Sprintf("DD_INSTALLER_DEFAULT_VERSION_%s", name)] = pkg.defaultVersion
	}
	return env
}
