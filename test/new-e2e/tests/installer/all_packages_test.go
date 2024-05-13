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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type packageTests func(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		// e2eos.AmazonLinux2,
		// e2eos.Debian12,
		// e2eos.RedHat9,
		// e2eos.Suse15,
		// e2eos.Fedora37,
		// e2eos.CentOS7,
	}
	arm64Flavors = []e2eos.Descriptor{
		// e2eos.Ubuntu2204,
		// e2eos.AmazonLinux2,
		// e2eos.Suse15,
	}
	packagesTests = []packageTests{
		testInstaller,
		testAgent,
		testApmInjectAgent,
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
	for _, flavor := range flavors {
		for _, test := range packagesTests {
			suite := test(flavor, flavor.Architecture)
			t.Run(suite.Name(), func(t *testing.T) {
				t.Parallel()
				e2e.Run(t, suite,
					e2e.WithProvisioner(
						awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(flavor, flavor.Architecture)), awshost.WithoutAgent()),
					),
					e2e.WithStackName(suite.Name()),
				)
			})
		}
	}
}

type packageSuite interface {
	e2e.Suite[environments.Host]

	Name() string
}

type packageBaseSuite struct {
	e2e.BaseSuite[environments.Host]
	host *host.Host

	opts []host.Option
	pkg  string
	arch e2eos.Architecture
	os   e2eos.Descriptor
}

func newPackageSuite(pkg string, os e2eos.Descriptor, arch e2eos.Architecture, opts ...host.Option) packageBaseSuite {
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

func (s *packageBaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.host = host.New(s.T(), s.Env().RemoteHost, s.os, s.arch, s.opts...)
}

func apiKey() string {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		return "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	return apiKey
}

func (s *packageBaseSuite) RunInstallScript() {
	env := map[string]string{
		"DD_API_KEY":                     apiKey(),
		"DD_SITE":                        "datadoghq.com",
		"DD_INSTALLER":                   "true",
		"DD_NO_AGENT_INSTALL":            "true",
		"TESTING_KEYS_URL":               "keys.datadoghq.com",
		"TESTING_APT_URL":                "apttesting.datad0g.com",
		"TESTING_APT_REPO_VERSION":       fmt.Sprintf("pipeline-%s-i7-%s 7", os.Getenv("CI_PIPELINE_ID"), s.os.Architecture),
		"TESTING_YUM_URL":                "yumtesting.datad0g.com",
		"TESTING_YUM_VERSION_PATH":       fmt.Sprintf("testing/pipeline-%s-i7/7", os.Getenv("CI_PIPELINE_ID")),
		"DD_INSTALLER_REGISTRY":          "669783387624.dkr.ecr.us-east-1.amazonaws.com",
		"DD_INSTALLER_REGISTRY_AUTH":     "ecr",
		"DD_INSTALLER_BOOTSTRAP_VERSION": fmt.Sprintf("pipeline-%v", os.Getenv("CI_PIPELINE_ID")),
	}
	// fixme: use the official install & remove manual creation of /etc/datadog-agent/datadog.yaml
	s.Env().RemoteHost.MustExecute(`bash -c "$(curl -L https://storage.googleapis.com/updater-dev/install_script_agent7.sh)"`, components.WithEnvVariables(env))
	datadogConfig := map[string]string{
		"api_key": apiKey(),
		"site":    "datadoghq.com",
	}
	rawDatadogConfig, err := yaml.Marshal(datadogConfig)
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute("sudo mkdir -p /etc/datadog-agent")
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("echo '%s' | sudo tee /etc/datadog-agent/datadog.yaml", string(rawDatadogConfig)))
	s.Env().RemoteHost.MustExecute("sudo chown -R dd-agent:dd-agent /etc/datadog-agent")
}

func (s *packageBaseSuite) InstallAgentPackage() {
	env := map[string]string{
		"DD_API_KEY":                 apiKey(),
		"DD_SITE":                    "datadoghq.com",
		"DD_INSTALLER_REGISTRY":      "669783387624.dkr.ecr.us-east-1.amazonaws.com",
		"DD_INSTALLER_REGISTRY_AUTH": "ecr",
	}
	s.Env().RemoteHost.MustExecute(`sudo -E datadog-installer install oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-package:`+fmt.Sprintf("pipeline-%v", os.Getenv("CI_PIPELINE_ID")), components.WithEnvVariables(env))
}

func (s *packageBaseSuite) InstallPackageLatest(pkg string) {
	env := map[string]string{
		"DD_API_KEY": apiKey(),
		"DD_SITE":    "datadoghq.com",
	}
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo -E datadog-installer install oci://gcr.io/datadoghq/%s-package:latest`, strings.TrimPrefix(pkg, "datadog-")), components.WithEnvVariables(env))
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
