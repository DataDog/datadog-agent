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
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type installerScriptTests func(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite

type installerScriptTestsWithSkippedFlavors struct {
	t              installerScriptTests
	skippedFlavors []e2eos.Descriptor
}

var (
	amd64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2404,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		e2eos.CentOS7,
		e2eos.Suse15,
	}
	arm64Flavors = []e2eos.Descriptor{
		e2eos.Ubuntu2404,
		e2eos.AmazonLinux2,
		e2eos.Suse15,
	}
	scriptTestsWithSkippedFlavors = []installerScriptTestsWithSkippedFlavors{
		{t: testDatabricksScript},
		{t: testDefaultScript, skippedFlavors: []e2eos.Descriptor{e2eos.CentOS7}},
		{t: testSSIScript},
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
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		if _, ok := os.LookupEnv("CI_COMMIT_SHA"); !ok {
			t.Log("CI_COMMIT_SHA & E2E_PIPELINE_ID env var are not set, this test requires one of these two variables to be set to work")
			t.FailNow()
		}
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
	var scriptURLPrefix string
	if pipelineID, ok := os.LookupEnv("E2E_PIPELINE_ID"); ok {
		scriptURLPrefix = fmt.Sprintf("https://s3.amazonaws.com/installtesting.datad0g.com/pipeline-%s/scripts/", pipelineID)
	} else if commitHash, ok := os.LookupEnv("CI_COMMIT_SHA"); ok {
		scriptURLPrefix = fmt.Sprintf("https://s3.amazonaws.com/installtesting.datad0g.com/%s/scripts/", commitHash)
	} else {
		require.FailNowf(nil, "missing script identifier", "CI_COMMIT_SHA or CI_PIPELINE_ID must be set")
	}

	return installerScriptBaseSuite{
		scriptURLPrefix: scriptURLPrefix,
		os:              e2eos,
		arch:            arch,
		pkg:             pkg,
		opts:            opts,
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
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.host = host.New(s.T, s.Env().RemoteHost, s.os, s.arch)
}

type installerScriptBaseSuite struct {
	scriptURLPrefix string
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

func (s *installerScriptBaseSuite) getAPIKey() string {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if apiKey == "" || err != nil {
			apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
		}
	}
	return apiKey
}

func (s *installerScriptBaseSuite) RunInstallScriptWithError(url string, params ...string) error {
	// Download scripts -- add retries for network issues
	var err error
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		_, err = s.Env().RemoteHost.Execute(fmt.Sprintf("curl -L %s > install_script", url))
		if err == nil {
			break
		}
		if i == maxRetries-1 {
			return err
		}
		time.Sleep(1 * time.Second)
	}

	scriptParams := append(params, fmt.Sprintf("DD_API_KEY=%s", s.getAPIKey()), "DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com")
	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf("%s bash install_script", strings.Join(scriptParams, " ")))
	return err
}

func (s *installerScriptBaseSuite) Purge() {
	s.Env().RemoteHost.MustExecute("sudo rm -rf install_script")
	s.Env().RemoteHost.Execute("sudo datadog-installer purge")
	s.Env().RemoteHost.Execute("sudo rm -rf /etc/datadog-agent")
}
