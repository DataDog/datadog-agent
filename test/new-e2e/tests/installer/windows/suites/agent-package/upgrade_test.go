// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"bufio"
	// "encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	// installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

	// "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	// windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/windows/common/agent"

	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
	"github.com/cenkalti/backoff/v4"
	"testing"
)

type packageName string

const (
	datadogAgent packageName = "datadog-agent"
)

const (
	pipelineOCIRegistry = "installtesting.datad0g.com"
)

var (
	agentWithoutFleetMSIVersion installerwindows.PackageVersion
)

func init() {
	agentWithoutFleetMSIVersion = installerwindows.NewVersionFromPackageVersion("7.63.0-1")
}

type testAgentUpgradeSuite struct {
	installerwindows.BaseSuite
}

// TestAgentUpgrades tests the usage of the Datadog installer to upgrade the Datadog Agent package.
func TestAgentUpgrades(t *testing.T) {
	e2e.Run(t, &testAgentUpgradeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestUpgradeMSI tests manual upgrade using the Datadog Agent MSI package.
//
// The expectation is that the MSI becomes the new stable package
func (s *testAgentUpgradeSuite) TestUpgradeMSI() {
	s.setAgentConfig()

	s.installPreviousAgentVersion()
	s.assertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	s.installCurrentAgentVersion()
	s.assertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
}

// TestUpgradeAgentPackage tests that the daemon can downgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.mustStartExperimentPreviousVersion()
	s.assertSuccessfulAgentStartExperiment(s.StableAgentVersion().Version())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	// Assert
}

// TestStopExperiment tests that the daemon can stop the experiment
// and that it reverts to the stable version.
func (s *testAgentUpgradeSuite) TestStopExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

// TestExperimentForNonExistingPackageFails tests that starting an experiment
// with a non-existing package version fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentForNonExistingPackageFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.Installer().StartExperiment(consts.AgentPackage, "unknown-version")
	s.Require().Error(err, "banana")
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

// TestExperimentCurrentVersionFails tests that starting an experiment
// with the same version as the current one fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentCurrentVersionFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.startExperimentCurrentVersion()
	s.Require().ErrorContains(err, "target package already exists")
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.Installer().StopExperiment(consts.AgentPackage)
	s.Require().NoError(err)

	// Assert
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDies() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.assertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	// wait for service to come back (extended backoff because MSI is running)
	err := s.waitForInstallerServiceWithBackoff("Running",
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
	// backend will send stop experiment now
	_, err = s.Installer().StopExperiment(consts.AgentPackage)
	s.Require().NoError(err)
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

func (s *testAgentUpgradeSuite) installPreviousAgentVersion() {
	// TODO: Update when prod MSI that contains the Installer is available
	agentVersion := s.StableAgentVersion().Version()
	pipelineID := "58521051"
	var urlopt installerwindows.Option
	if packageFile, ok := os.LookupEnv("PREVIOUS_AGENT_MSI_URL"); ok {
		urlopt = installerwindows.WithInstallerURL(packageFile)
	} else if pipeline, ok := os.LookupEnv("PREVIOUS_AGENT_MSI_PIPELINE"); ok {
		urlopt = installerwindows.WithURLFromPipeline(pipeline)
	} else {
		urlopt = installerwindows.WithURLFromPipeline(pipelineID)
	}
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithOption(urlopt),
		installerwindows.WithMSILogFile("install-previous-version.log"),
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) installCurrentAgentVersion() {
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()
	var urlopt installerwindows.Option
	if packageFile, ok := os.LookupEnv("CURRENT_AGENT_MSI_URL"); ok {
		urlopt = installerwindows.WithInstallerURL(packageFile)
	} else if pipeline, ok := os.LookupEnv("CURRENT_AGENT_MSI_PIPELINE"); ok {
		urlopt = installerwindows.WithURLFromPipeline(pipeline)
	}
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithOption(urlopt),
		installerwindows.WithMSILogFile("install-current-version.log"),
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) startExperimentWithCustomPackage(packageConfig TestPackageConfig) (string, error) {
	// Set catalog so daemon can find the package
	_, err := s.Installer().SetCatalog(installerwindows.Catalog{
		Packages: []installerwindows.PackageEntry{
			{
				Package: packageConfig.Name,
				Version: packageConfig.Version,
				URL:     packageConfig.URL(),
			},
		},
	})
	s.Require().NoError(err)

	return s.Installer().StartExperiment(consts.AgentPackage, packageConfig.Version)
}

func (s *testAgentUpgradeSuite) startExperimentPreviousVersion() (string, error) {
	// agentVersion := s.StableAgentVersion().Version()
	// TODO: switch to prod stable entry when available
	pipelineID := "58521051"
	packageConfig := newPackageConfigForPipeline(string(datadogAgent), pipelineID)
	packageConfig, err := applyDevEnvOCIPackageOverrides(s.Env().RemoteHost, "PREVIOUS_AGENT", packageConfig)
	s.Require().NoError(err)

	return s.startExperimentWithCustomPackage(packageConfig)
}

func (s *testAgentUpgradeSuite) mustStartExperimentPreviousVersion() {
	// Arrange
	agentVersion := s.StableAgentVersion().Version()

	// Act
	_, _ = s.startExperimentPreviousVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) startExperimentCurrentVersion() (string, error) {
	// agentVersion := s.CurrentAgentVersion().GetNumberAndPre()
	// Default to using OCI package from current pipeline
	packageConfig := newPackageConfigForPipeline(string(datadogAgent), s.Env().Environment.PipelineID())
	packageConfig, err := applyDevEnvOCIPackageOverrides(s.Env().RemoteHost, "CURRENT_AGENT", packageConfig)
	s.Require().NoError(err)

	return s.startExperimentWithCustomPackage(packageConfig)
}

func (s *testAgentUpgradeSuite) mustStartExperimentCurrentVersion() {
	// Arrange
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()

	// Act
	_, _ = s.startExperimentCurrentVersion()
	// can't check error here because the process will be killed by the MSI "files in use"
	// and experiment started in the background
	// s.Require().NoError(err)

	// Assert
	// have to wait for experiment to finish installing
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) setAgentConfig() {
	s.Env().RemoteHost.MkdirAll("C:\\ProgramData\\Datadog")
	s.Env().RemoteHost.WriteFile(consts.ConfigPath, []byte(`
api_key: aaaaaaaaa
remote_updates: true
`))
}

func (s *testAgentUpgradeSuite) getInstallerStatus() installerStatus {
	// TODO: use JSON status
	out, err := s.Installer().Status()
	s.Require().NoError(err)
	status := parseStatusOutput(out)
	return status
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStartExperiment(version string) {
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	// TODO: use JSON status
	status := s.getInstallerStatus()
	s.Require().Contains(status.Packages, "datadog-agent")
	s.Require().Contains(status.Packages["datadog-agent"].ExperimentVersion, version)
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentPromoteExperiment(version string) {
	err := s.waitForInstallerService("Running")
	s.Require().NoError(err)

	// TODO: use JSON status
	status := s.getInstallerStatus()
	s.Require().Contains(status.Packages, "datadog-agent")
	s.Require().Contains(status.Packages["datadog-agent"].StableVersion, version)
	s.Require().Contains(status.Packages["datadog-agent"].ExperimentVersion, "")
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStopExperiment(version string) {
	// conditions are same as promote, except the stable version should be unchanged
	// since version is an input we can reuse.
	s.assertSuccessfulAgentPromoteExperiment(version)
}

func (s *testAgentUpgradeSuite) waitForInstallerService(state string) error {
	return s.waitForInstallerServiceWithBackoff(state,
		// usually waiting after MSI runs so we have to wait awhile
		// max wait is 30*30 -> 900 seconds (15 minutes)
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
}

func (s *testAgentUpgradeSuite) waitForInstallerServiceWithBackoff(state string, b backoff.BackOff) error {
	return backoff.Retry(func() error {
		out, err := windowscommon.GetServiceStatus(s.Env().RemoteHost, consts.ServiceName)
		if err != nil {
			return err
		}
		if !strings.Contains(out, state) {
			return fmt.Errorf("expected state %s, got %s", state, out)
		}
		return nil
	}, b)
}

// createFileRegistryFromLocalOCI uploads a local OCI package to the remote host and prepares it to
// be used as a `file://` package path for the daemon downloader.
//
// returns the path to the extracted package on the remote host.
//
// Currently, this requires extracting the OCI package to a directory.
func createFileRegistryFromLocalOCI(host *components.RemoteHost, localPackagePath string) (string, error) {
	// Upload OCI package to temporary path
	remotePath, err := windowscommon.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	host.CopyFile(localPackagePath, remotePath)
	// Extract OCI package
	outPath := remotePath + ".extracted"
	// tar is a built-in command on Windows 10+
	cmd := fmt.Sprintf("mkdir %s; tar -xf %s -C %s", outPath, remotePath, outPath)
	_, err = host.Execute(cmd)
	if err != nil {
		return "", err
	}
	// return path to extracted package
	return outPath, nil
}

// applyDevEnvOCIPackageOverrides applies overrides to the package config based on environment variables.
//
// Example: local OCI package file
//
//	export CURRENT_AGENT_OCI_URL="file:///path/to/oci/package.tar"
//
// Example: from a different pipeline
//
//	export CURRENT_AGENT_OCI_PIPELINE="123456"
//
// Example: from a different pipeline
// (assumes that the package being overridden is already from a pipeline)
//
//	export CURRENT_AGENT_OCI_VERSION="pipeline-123456"
//
// Example: custom URL
//
//	export CURRENT_AGENT_OCI_URL="oci://installtesting.datad0g.com/agent-package:pipeline-123456"
func applyDevEnvOCIPackageOverrides(host *components.RemoteHost, prefix string, packageConfig TestPackageConfig) (TestPackageConfig, error) {
	// env vars for convenience
	if url, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_URL", prefix)); ok {
		packageConfig.urloverride = url
	}
	if pipeline, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_PIPELINE", prefix)); ok {
		packageConfig.Registry = "installtesting.datad0g.com"
		packageConfig.Version = fmt.Sprintf("pipeline-%s", pipeline)
	}

	// env vars for specific fields
	if version, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_VERSION", prefix)); ok {
		packageConfig.Version = version
	}
	if registry, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_REGISTRY", prefix)); ok {
		packageConfig.Registry = registry
	}
	if auth, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_AUTH", prefix)); ok {
		packageConfig.Auth = auth
	}

	// If the URL is a file, upload it to the remote host
	if strings.HasPrefix(packageConfig.urloverride, "file://") {
		localPath := strings.TrimPrefix(packageConfig.urloverride, "file://")
		outPath, err := createFileRegistryFromLocalOCI(host, localPath)
		if err != nil {
			return packageConfig, err
		}
		// Must replace slashes so that daemon can parse it correctly
		outPath = strings.Replace(outPath, "\\", "/", -1)
		packageConfig.urloverride = fmt.Sprintf("file://%s", outPath)
	}
	return packageConfig, nil
}

// newPackageConfigForPipeline creates a TestPackageConfig for a package created from a pipeline.
func newPackageConfigForPipeline(packageName string, pipeline string, opts ...PackageOption) TestPackageConfig {
	options := []PackageOption{
		WithName(packageName),
		WithRegistry(pipelineOCIRegistry),
		WithVersion(fmt.Sprintf("pipeline-%s", pipeline)),
	}
	switch packageName {
	case string(datadogAgent):
		options = append(options, WithAlias("agent-package"))
	}
	options = append(options, opts...)
	return newPackageConfig(options...)
}

func newPackageConfig(opts ...PackageOption) TestPackageConfig {
	c := TestPackageConfig{}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// TestPackageConfig is a struct that regroups the fields necessary to install a package from an OCI Registry
type TestPackageConfig struct {
	// Name the name of the package
	Name string
	// Alias Sometimes the package is named differently in some registries
	Alias string
	// Version the version to install
	Version string
	// Registry the URL of the registry
	Registry string
	// Auth the authentication method, "" for no authentication
	Auth string
	// urloverride to use for package
	//
	// The URL is normally constructed from the above parts, this field will take precedence.
	// Useful for development to test local packages.
	urloverride string
}

func (c TestPackageConfig) URL() string {
	if c.urloverride != "" {
		// if the URL had been overridden, use it
		return c.urloverride
	}
	// else construct it from parts
	name := c.Name
	if c.Alias != "" {
		name = c.Alias
	}
	return fmt.Sprintf("oci://%s/%s:%s", c.Registry, name, c.Version)
}

// PackageOption is an optional function parameter type for the Datadog Installer
type PackageOption func(*TestPackageConfig) error

// WithName uses a specific name for the package.
func WithName(name string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Name = name
		return nil
	}
}

// WithAuthentication uses a specific authentication for a Registry to install the package.
func WithAuthentication(auth string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Auth = auth
		return nil
	}
}

// WithRegistry uses a specific Registry from where to install the package.
func WithRegistry(registryURL string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Registry = registryURL
		return nil
	}
}

// WithVersion uses a specific version of the package.
func WithVersion(version string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Version = version
		return nil
	}
}

// WithAlias specifies the package's alias.
func WithAlias(alias string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Alias = alias
		return nil
	}
}

// WithURLOverride specifies the package's URL.
func WithURLOverride(url string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.urloverride = url
		return nil
	}
}

type packageStatus struct {
	Name              string
	StableVersion     string
	ExperimentVersion string
}

type installerStatus struct {
	Version  string
	Packages map[string]packageStatus
}

// TODO:
// Linux tests use curl to hit the unix socket and get JSON output but we can't do the same
// for the named pipe on Windows. We should consider adding a JSON output option to the status command.
func parseStatusOutput(output string) installerStatus {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentPackage packageStatus

	var status installerStatus
	status.Packages = make(map[string]packageStatus)

	// Regular expressions for extracting relevant lines
	versionRegex := regexp.MustCompile(`Datadog Installer v(\S+)`)
	packageNameRegex := regexp.MustCompile(`^\s*([a-zA-Z0-9\-_]+)$`)
	stableVersionRegex := regexp.MustCompile(`\s*. stable:\s*(\S+)`)
	experimentVersionRegex := regexp.MustCompile(`\s*. experiment:\s*(\S+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if match := versionRegex.FindStringSubmatch(line); match != nil {
			status.Version = match[1]
			continue
		}

		// Check for package name
		if match := packageNameRegex.FindStringSubmatch(line); match != nil {
			// If we already have a package, store it before starting a new one
			if currentPackage.Name != "" {
				status.Packages[currentPackage.Name] = currentPackage
			}
			currentPackage = packageStatus{Name: match[1]}
			continue
		}

		// Check for stable version
		if match := stableVersionRegex.FindStringSubmatch(line); match != nil {
			currentPackage.StableVersion = match[1]
			continue
		}

		// Check for experiment version
		if match := experimentVersionRegex.FindStringSubmatch(line); match != nil {
			if match[1] == "none" {
				// handle this case here instead of in tests.
				// the JSON seems to use an empty string, so it'll save us some work later.
				continue
			}
			currentPackage.ExperimentVersion = match[1]
			continue
		}
	}

	// Append the last parsed package
	if currentPackage.Name != "" {
		status.Packages[currentPackage.Name] = currentPackage
	}

	return status
}

// end of file
