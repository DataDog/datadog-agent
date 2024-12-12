// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

type installerScriptTests func(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite

type installerScriptTestsWithSkipedFlavors struct {
	t              installerScriptTests
	skippedFlavors []e2eos.Descriptor
}

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		e2eos.FedoraDefault,
		e2eos.CentOS7,
		e2eos.Suse15,
	}
	arm64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2204,
		e2eos.AmazonLinux2,
		e2eos.Suse15,
	}
	scriptTestsWithSkippedFlavors = []installerScriptTestsWithSkipedFlavors{
		{t: testDatabricksScript},
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

func TestScripts(t *testing.T) {
	if _, ok := os.LookupEnv("CI_COMMIT_SHA"); !ok {
		t.Log("CI_COMMIT_SHA env var is not set, this test requires this variable to be set to work")
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
		for _, test := range scriptTestsWithSkippedFlavors {
			flavor := f // capture range variable for parallel tests closure
			if shouldSkipFlavor(test.skippedFlavors, flavor) {
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

type installerScriptSuite interface {
	e2e.Suite[environments.Host]

	Name() string
	ProvisionerOptions() []awshost.ProvisionerOption
}

func newInstallerScriptSuite(pkg string, e2eos e2eos.Descriptor, arch e2eos.Architecture, opts ...awshost.ProvisionerOption) installerScriptBaseSuite {
	return installerScriptBaseSuite{
		commitHash: os.Getenv("CI_COMMIT_SHA"),
		os:         e2eos,
		arch:       arch,
		pkg:        pkg,
		opts:       opts,
	}
}

func (s *installerScriptBaseSuite) Name() string {
	return regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(fmt.Sprintf("%s/%s", s.pkg, s.os), "_")
}

func (s *installerScriptBaseSuite) ProvisionerOptions() []awshost.ProvisionerOption {
	return s.opts
}

func (s *installerScriptBaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.host = host.New(s.T(), s.Env().RemoteHost, s.os, s.arch)
}

type installerScriptBaseSuite struct {
	commitHash string
	e2e.BaseSuite[environments.Host]

	host *host.Host
	opts []awshost.ProvisionerOption
	pkg  string
	arch e2eos.Architecture
	os   e2eos.Descriptor
}

func (s *installerScriptBaseSuite) RunInstallScript(url string, params ...string) {
	err := s.RunInstallScriptWithError(url, params...)
	require.NoErrorf(s.T(), err, "install script failed")
}

func (s *installerScriptBaseSuite) RunInstallScriptWithError(url string, params ...string) error {
	scriptParams := append(params, "DD_API_KEY=test", "DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE=installtesting.datad0g.com")
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("curl -L %s > install_script; %s bash install_script", url, strings.Join(scriptParams, " ")))
	return err
}

func (s *installerScriptBaseSuite) Purge() {
	s.Env().RemoteHost.MustExecute("sudo rm -rf install_script")
	s.Env().RemoteHost.Execute("sudo datadog-installer purge")
}
