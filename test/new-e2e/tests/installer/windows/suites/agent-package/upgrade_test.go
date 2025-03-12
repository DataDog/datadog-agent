// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	// "encoding/json"
	"fmt"
	"os"
	// "time"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

	// "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	// windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/windows/common/agent"

	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
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

func (s *testAgentUpgradeSuite) installPreviousAgentVersion() {
	// TODO: Update when prod MSI that contains the Installer is available
	agentVersion := s.StableAgentVersion().Version()
	pipelineID := "58495742"
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
		WithVersionEqual(agentVersion)
}

func (s *testAgentUpgradeSuite) installCurrentAgentVersion() {
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

func (s *testAgentUpgradeSuite) startExperimentPreviousVersion() {
	agentVersion := s.StableAgentVersion().Version()
	// TODO: switch to prod stable entry when available
	pipelineID := "58495742"
	packageConfig := newPackageConfigForPipeline(string(datadogAgent), pipelineID)
	packageConfig, err := applyDevEnvOCIPackageOverrides(s.Env().RemoteHost, "PREVIOUS_AGENT", packageConfig)
	s.Require().NoError(err)

	_, _ = s.startExperimentWithCustomPackage(packageConfig)
	// TODO: currently returns error (MSI kills it?)
	//       {"error":"Post \"http://daemon/datadog-agent/experiment/start\": EOF","code":0}
	//       : Process exited with status 4294967295
	// s.Require().NoError(err)

	// sanity check: make sure we did indeed install the previous version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) startExperimentCurrentVersion() {
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()
	// Default to using OCI package from current pipeline
	packageConfig := newPackageConfigForPipeline(string(datadogAgent), s.Env().Environment.PipelineID())
	packageConfig, err := applyDevEnvOCIPackageOverrides(s.Env().RemoteHost, "CURRENT_AGENT", packageConfig)
	s.Require().NoError(err)

	_, _ = s.startExperimentWithCustomPackage(packageConfig)
	// TODO: currently returns error (MSI kills it?)
	//       {"error":"Post \"http://daemon/datadog-agent/experiment/start\": EOF","code":0}
	//       : Process exited with status 4294967295
	// s.Require().NoError(err)

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

func (s *testAgentUpgradeSuite) TestUpgradeMSI() {
	s.setAgentConfig()

	s.installPreviousAgentVersion()
	out, err := s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)

	s.installCurrentAgentVersion()
	out, err = s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)
}

func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	s.setAgentConfig()

	s.installPreviousAgentVersion()
	out, err := s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)

	s.startExperimentCurrentVersion()
	out, err = s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)
}

func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	s.setAgentConfig()

	s.installCurrentAgentVersion()
	out, err := s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)

	s.startExperimentPreviousVersion()
	out, err = s.Installer().Status()
	s.Require().NoError(err)
	s.T().Log(out)
}

// // TestUpgradeFromNonFleetAgent tests that
// func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
// 	s.T().Cleanup(func() {
// 		s.Installer().Purge()
// 	})
// 	// Arrange
// 	s.Require().NoError(s.Installer().Install(
// 		installerwindows.WithOption(installerwindows.WithURLFromInstallersJSON(
// 			pipeline.StableURL, agentWithoutFleetMSIVersion.PackageVersion(),
// 		))),
// 		installerwindows.WithMSILogFile("install-non-fleet.log"),
// 	)
// 	// sanity check: make sure we did indeed install the stable version
// 	s.Require().Host(s.Env().RemoteHost).
// 		HasBinary(`C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`).
// 		// Don't check the binary signature because it could have been updated since the last stable was built
// 		WithVersionMatchPredicate(func(version string) {
// 			s.Require().Contains(version, agentWithoutFleetMSIVersion.Version())
// 		})

// 	// Act
// 	s.Require().NoError(s.Installer().Install(
// 		installerwindows.WithOption(installerwindows.WithURLFromInstallersJSON(
// 			pipeline.StableURL, agentWithoutFleetMSIVersion.PackageVersion(),
// 		))),
// 		installerwindows.WithMSILogFile("install-non-fleet.log"),
// 	)
// 	_, err := s.Installer().SetCatalog(installerwindows.Catalog{
// 		Packages: []installerwindows.PackageEntry{
// 			{
// 				Package: string(datadogAgent),
// 				Version: "pipeline-58348454",
// 				URL:     "oci://installtesting.datad0g.com/agent-package:pipeline-58348454",
// 			},
// 		},
// 	})
// 	s.Require().NoError(err)
// 	_, err = s.Installer().StartExperiment(consts.AgentPackage, "pipeline-58348454")

// 	// Assert
// 	s.Require().NoErrorf(err, "failed to upgrade to the latest Datadog Agent package")
// 	s.Require().Host(s.Env().RemoteHost).
// 		HasARunningDatadogAgentService().
// 		WithVersionMatchPredicate(func(version string) {
// 			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
// 		}).
// 		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))

// 	// s.Run("Install stable", func() {
// 	// 	s.installStableAgent()
// 	// 	s.Run("Upgrade to latest using an experiment", func() {
// 	// 		s.startLatestExperiment()
// 	// 		s.Run("Stop experiment", s.stopExperiment)
// 	// 	})
// 	// })
// }

// TestUpgradeAgentPackage tests that it's possible to upgrade the Datadog Agent using the Datadog installer.
// func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
// 	s.Run("Install stable", func() {
// 		s.installStableAgent()
// 		s.Run("Upgrade to latest using an experiment", func() {
// 			s.startLatestExperiment()
// 			s.Run("Stop experiment", s.stopExperiment)
// 		})
// 	})
// }

// // TestDowngradeAgentPackage tests that it's possible to downgrade the Datadog Agent using the Datadog installer.
// func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
// 	// Arrange
// 	_, err := s.Installer().InstallPackage(consts.AgentPackage)
// 	s.Require().NoErrorf(err, "failed to install the stable Datadog Agent package")

// 	// Act
// 	_, err = s.Installer().StartExperiment(consts.AgentPackage)

// 	// Assert
// 	s.Require().NoErrorf(err, "failed to downgrade to stable Datadog Agent package")
// 	s.Require().Host(s.Env().RemoteHost).
// 		HasARunningDatadogAgentService().
// 		WithVersionMatchPredicate(func(version string) {
// 			s.Require().Contains(version, s.StableAgentVersion().Version())
// 		}).
// 		DirExists(consts.GetStableDirFor(consts.AgentPackage))
// }

func (s *testAgentUpgradeSuite) TestExperimentFailure() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act
	_, err := s.Installer().InstallExperiment(consts.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion("unknown-version"),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().Error(err, "expected an error when trying to start an experiment with an unknown version")
	s.stopExperiment()
	// TODO: is this the same test as TestStopWithoutExperiment?
}

func (s *testAgentUpgradeSuite) TestExperimentCurrentVersion() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act
	_, err := s.Installer().InstallExperiment(consts.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion(s.StableAgentVersion().PackageVersion()),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().Error(err, "expected an error when trying to start an experiment with the same version as the current one")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(consts.GetStableDirFor(consts.AgentPackage))
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.Run("Install stable", func() {
		s.installStableAgent()
	})

	// Act

	// Assert
	s.stopExperiment()
	// TODO: Currently uninstalls stable then reinstalls stable. functional but a waste.
}

func (s *testAgentUpgradeSuite) installStableAgent() {
	// Arrange

	// Act
	output, err := s.Installer().InstallPackage(consts.AgentPackage,
		installer.WithRegistry("install.datadoghq.com"),
		installer.WithVersion(s.StableAgentVersion().PackageVersion()),
		installer.WithAuthentication(""),
	)

	// Assert
	s.Require().NoErrorf(err, "failed to install the stable Datadog Agent package: %s", output)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(consts.GetStableDirFor(consts.AgentPackage))
}

// func (s *testAgentUpgradeSuite) startLatestExperiment() {
// 	// Arrange

// 	// Act
// 	output, err := s.Installer().InstallExperiment(consts.AgentPackage)

// 	// Assert
// 	s.Require().NoErrorf(err, "failed to upgrade to the latest Datadog Agent package: %s", output)
// 	s.Require().Host(s.Env().RemoteHost).
// 		HasARunningDatadogAgentService().
// 		WithVersionMatchPredicate(func(version string) {
// 			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
// 		}).
// 		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
// }

func (s *testAgentUpgradeSuite) stopExperiment() {
	// Arrange

	// Act
	output, err := s.Installer().RemoveExperiment(consts.AgentPackage)

	// Assert
	s.Require().NoErrorf(err, "failed to remove the experiment for the Datadog Agent package: %s", output)

	// Remove experiment uninstalls the experimental version but also re-installs the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		}).
		DirExists(consts.GetStableDirFor(consts.AgentPackage))
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
	if url, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_URL", prefix)); ok {
		packageConfig.URLOverride = url
	}
	if pipeline, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_PIPELINE", prefix)); ok {
		packageConfig.Registry = "installtesting.datad0g.com"
		packageConfig.Version = fmt.Sprintf("pipeline-%s", pipeline)
	}

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
	if strings.HasPrefix(packageConfig.URLOverride, "file://") {
		localPath := strings.TrimPrefix(packageConfig.URLOverride, "file://")
		outPath, err := createFileRegistryFromLocalOCI(host, localPath)
		if err != nil {
			return packageConfig, err
		}
		// Must replace slashes so that daemon can parse it correctly
		outPath = strings.Replace(outPath, "\\", "/", -1)
		packageConfig.URLOverride = fmt.Sprintf("file://%s", outPath)
	}
	return packageConfig, nil
}

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
	// URLOverride to use for package
	//
	// The URL is normally constructed from the above parts, this field will take precedence.
	// Useful for development to test local packages.
	URLOverride string
}

func (c TestPackageConfig) URL() string {
	if c.URLOverride != "" {
		// if the URL had been overridden, use it
		return c.URLOverride
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
		params.URLOverride = url
		return nil
	}
}

// end of file
