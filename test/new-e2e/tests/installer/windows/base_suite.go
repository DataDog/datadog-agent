// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"os"

	"github.com/stretchr/testify/suite"
)

// isWERDumpCollectionEnabled returns true by default; it is disabled when DD_E2E_SKIP_WER_DUMPS is truthy
func isWERDumpCollectionEnabled() bool {
	val := os.Getenv("DD_E2E_SKIP_WER_DUMPS")
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return false
	default:
		return true
	}
}

// BaseSuite the base suite for all installer tests on Windows (install script, MSI, exe etc...).
// To run the test suites locally, pick a pipeline and define the following environment variables:
// E2E_PIPELINE_ID: the ID of the pipeline
// CURRENT_AGENT_VERSION: pull it from one of the jobs that builds the Agent
// STABLE_AGENT_VERSION_PACKAGE: use `crane ls public.ecr.aws/datadog/agent-package | sort | tail -n 2 | head -n 1`
// or pick any other version from that registry.
//
// For example:
//
//	CI_COMMIT_SHA=ac2acaffab7b039f8c2524df8ae82f9f5fd04d5d;
//	E2E_PIPELINE_ID=40537701;
//	CURRENT_AGENT_VERSION=7.57.0-devel+git.370.d429ae3;
//	STABLE_AGENT_VERSION_PACKAGE=7.55.2-1
type BaseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer          DatadogInstallerRunner
	installScriptImpl  InstallScriptRunner
	currentAgent       *AgentVersionManager
	stableAgent        *AgentVersionManager
	CreateCurrentAgent func() (*AgentVersionManager, error)
	CreateStableAgent  func() (*AgentVersionManager, error)
	dumpFolder         string
}

// Installer The Datadog Installer for testing.
func (s *BaseSuite) Installer() DatadogInstallerRunner {
	return s.installer
}

// InstallScript returns the installer implementation.
// Override this method in your test suite to use a different implementation.
func (s *BaseSuite) InstallScript() InstallScriptRunner {
	return s.installScriptImpl
}

// SetInstallScriptImpl sets a custom installer implementation.
// Use this in your test suite's SetupSuite to override the default implementation.
func (s *BaseSuite) SetInstallScriptImpl(impl InstallScriptRunner) {
	s.installScriptImpl = impl
}

// SetInstaller sets a custom installer implementation.
// Use this in your test suite's SetupSuite to override the default implementation.
func (s *BaseSuite) SetInstaller(impl DatadogInstallerRunner) {
	s.installer = impl
}

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog installer `BaseSuite` object.
func (s *BaseSuite) Require() *suiteasserts.SuiteAssertions {
	return suiteasserts.New(s, s.BaseSuite.Require())
}

// CurrentAgentVersion the version of the Agent in the current pipeline
func (s *BaseSuite) CurrentAgentVersion() *AgentVersionManager {
	return s.currentAgent
}

// StableAgentVersion the version of the last published stable agent
func (s *BaseSuite) StableAgentVersion() *AgentVersionManager {
	return s.stableAgent
}

// SetupSuite checks that the environment variables are correctly setup for the test
func (s *BaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// The below current and stable artifacts can be configured with environment variables.
	// See doc.go for more information.
	// TODO: not every test needs every artifact, it might be nice to have a way to opt-in to specific artifacts
	//       which would let us create better "required but not set" messages.
	s.createCurrentAgent()
	s.T().Logf("current agent version: %s", s.CurrentAgentVersion())
	s.createStableAgent()
	s.T().Logf("stable agent version: %s", s.StableAgentVersion())

	// Enable crash dumps
	if isWERDumpCollectionEnabled() {
		host := s.Env().RemoteHost
		s.dumpFolder = `C:\dumps`
		err := windowscommon.EnableWERGlobalDumps(host, s.dumpFolder)
		s.Require().NoError(err, "should enable WER dumps")
		// Set the environment variable at the machine level.
		// The tests will be re-installing services so the per-service environment
		// won't be persisted.
		_, err = host.Execute(`[Environment]::SetEnvironmentVariable("GOTRACEBACK", "wer", "Machine")`)
		s.Require().NoError(err, "should set GOTRACEBACK environment variable")
	} else {
		s.T().Log("WER dump collection disabled via DD_E2E_SKIP_WER_DUMPS")
	}
}

// createCurrentAgent sets the current agent version for the test suite.
//
// By default, the current agent is the current pipeline, but tests can
// override this by setting the CreateCurrentAgent function.
//
// For testing, the version and artifacts can be overridden via environment variables, see
// doc.go for more information.
func (s *BaseSuite) createCurrentAgent() {
	if s.CreateCurrentAgent != nil {
		agent, err := s.CreateCurrentAgent()
		s.Require().NoError(err, "failed to create current agent")
		s.currentAgent = agent
		return
	}
	// else, use the defaults (current pipeline)

	// Get current version OCI package
	currentOCI, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithPipeline(s.Env().Environment.PipelineID()),
		WithDevEnvOverrides("CURRENT_AGENT"),
	)
	s.Require().NoError(err, "failed to lookup OCI package for current agent version")

	// Get current version MSI package
	currentMSI, err := windowsagent.NewPackage(
		windowsagent.WithURLFromPipeline(s.Env().Environment.PipelineID()),
		windowsagent.WithDevEnvOverrides("CURRENT_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for current agent version")
	s.Require().NotEmpty(currentMSI.URL, "Agent MSI URL is required but not set, set E2E_PIPELINE_ID or CURRENT_AGENT devenv overrides")

	// Setup current Agent artifacts
	currentVersion, currentPackageVersion := s.getAgentVersionVars("CURRENT_AGENT")
	s.currentAgent, err = NewAgentVersionManager(
		currentVersion,
		currentPackageVersion,
		currentOCI,
		currentMSI,
	)
	s.Require().NoError(err, "Current agent version was in an incorrect format")
}

// createStableAgent sets the stable agent version for the test suite.
//
// By default, the stable agent is the last stable release, but tests can
// override this by setting the CreateStableAgent function.
//
// For testing, the version and artifacts can be overridden via environment variables, see
// doc.go for more information.
func (s *BaseSuite) createStableAgent() {
	if s.CreateStableAgent != nil {
		agent, err := s.CreateStableAgent()
		s.Require().NoError(err, "failed to create stable agent")
		s.stableAgent = agent
		return
	}
	// else, use the defaults (last stable release)

	agentVersion := "7.75.0"
	agentVersionPackage := "7.75.0-1"
	agentRegistry := consts.StableS3OCIRegistry
	agentMSIURL := "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-7.75.0.msi"
	// Allow override of version and version package via environment variables
	if val := os.Getenv("STABLE_AGENT_VERSION"); val != "" {
		agentVersion = val
	}
	if val := os.Getenv("STABLE_AGENT_VERSION_PACKAGE"); val != "" {
		agentVersionPackage = val
	}

	// Get previous version OCI package
	previousOCI, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion(agentVersionPackage),
		WithRegistry(agentRegistry),
		WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup OCI package for previous agent version")

	// Get previous version MSI package
	previousMSI, err := windowsagent.NewPackage(
		windowsagent.WithVersion(agentVersionPackage),
		windowsagent.WithURL(agentMSIURL),
		windowsagent.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	// Setup previous Agent artifacts
	s.stableAgent, err = NewAgentVersionManager(
		agentVersion,
		agentVersionPackage,
		previousOCI,
		previousMSI,
	)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")
}

// getAgentVersionVars retrieves the agent version and package version from environment variables
//
// example: CURRENT_AGENT_VERSION and CURRENT_AGENT_VERSION_PACKAGE
//
// see doc.go for more information
func (s *BaseSuite) getAgentVersionVars(prefix string) (string, string) {
	versionVar := prefix + "_VERSION"
	versionPackageVar := prefix + "_VERSION_PACKAGE"

	// Agent version
	version := os.Getenv(versionVar)
	s.Require().NotEmpty(versionVar, "%s is required but not set", versionVar)

	// Package version
	versionPackage := os.Getenv(versionPackageVar)
	if versionPackage == "" && os.Getenv("CI") == "" {
		// locally, the version package can be the same as the version
		versionPackage = version
	} else {
		// The CI is expected to configure this
		s.Require().NotEmpty(versionPackage, "%s is required but not set", versionPackageVar)
	}

	return version, versionPackage
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// Create a new subdir per test since these suites often have multiple tests
	testPart := common.SanitizeDirectoryName(testName)
	outputDir := filepath.Join(s.SessionOutputDir(), testPart)
	s.Require().NoError(os.MkdirAll(outputDir, 0755))

	s.installer = NewDatadogInstaller(s.Env(), s.CurrentAgentVersion().MSIPackage().URL, outputDir)
	s.installScriptImpl = NewDatadogInstallScript(s.Env().RemoteHost)

	// clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		s.T().Logf("Clearing %s event log", logName)
		err := windowscommon.ClearEventLog(s.Env().RemoteHost, logName)
		s.Require().NoError(err, "should clear %s event log", logName)
	}

}

// AfterTest collects the event logs and agent logs after each test
// NOTE: AfterTest is not called after subtests
func (s *BaseSuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&s.BaseSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	// look for and download crashdumps
	if isWERDumpCollectionEnabled() {
		// Poll for up to ~30s (1s interval) to allow WER to finish writing full dumps (DumpType=2)
		h := s.Env().RemoteHost
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			entries, _ := h.ReadDir(s.dumpFolder)
			hasDump := false
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".dmp") {
					hasDump = true
					break
				}
			}
			if hasDump {
				break
			}
			time.Sleep(1 * time.Second)
		}
		dumps, err := windowscommon.DownloadAllWERDumps(s.Env().RemoteHost, s.dumpFolder, s.SessionOutputDir())
		s.Assert().NoError(err, "should download crash dumps")
		if !s.Assert().Empty(dumps, "should not have crash dumps") {
			s.T().Logf("Found crash dumps:")
			for _, dump := range dumps {
				s.T().Logf("  %s", dump)
			}
		}
	}
	// Collect WER ReportArchive entries for powershell.exe if present.
	// Synchronously scan the WER ReportArchive for PowerShell crash
	// reports and copy any found files into the test’s output directory.
	func() {
		host := s.Env().RemoteHost
		base := `C:\ProgramData\Microsoft\Windows\WER\ReportArchive`
		entries, derr := host.ReadDir(base)
		if derr != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(strings.ToLower(name), strings.ToLower("AppCrash_powershell.exe")) {
				continue
			}
			dir := filepath.Join(base, name)
			files, derr2 := host.ReadDir(dir)
			if derr2 != nil {
				continue
			}
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				src := filepath.Join(dir, f.Name())
				dst := filepath.Join(s.SessionOutputDir(), fmt.Sprintf("WER_%s_%s", name, f.Name()))
				_ = host.GetFile(src, dst)
			}
		}
	}()

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		vm := s.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			s.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(s.SessionOutputDir(), logName+".evtx")
			err := windowscommon.ExportEventLog(vm, logName, outputPath)
			s.Assert().NoError(err, "should export %s event log", logName)
			// Log errors and warnings to the screen for easy access
			out, err := windowscommon.GetEventLogErrorsAndWarnings(vm, logName)
			if s.Assert().NoError(err, "should get errors and warnings from %s event log", logName) && out != "" {
				s.T().Logf("Errors and warnings from %s event log:\n%s", logName, out)
			}
		}
		// collect agent logs
		s.collectAgentLogs()
		s.collectInstallerLogs()
	}
}

func (s *BaseSuite) collectAgentLogs() {
	host := s.Env().RemoteHost

	s.T().Logf("Collecting agent logs")
	logsFolder, err := host.GetLogsFolder()
	if !s.Assert().NoError(err, "should get logs folder") {
		return
	}
	entries, err := host.ReadDir(logsFolder)
	if !s.Assert().NoError(err, "should read log folder") {
		return
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(logsFolder, entry.Name())
		destPath := filepath.Join(s.SessionOutputDir(), entry.Name())

		if entry.IsDir() {
			s.T().Logf("Found log directory: %s", entry.Name())
			err = host.GetFolder(sourcePath, destPath)
		} else {
			s.T().Logf("Found log file: %s", entry.Name())
			err = host.GetFile(sourcePath, destPath)
		}
		s.Assert().NoError(err, "should download %s", entry.Name())
	}
}

func (s *BaseSuite) collectInstallerLogs() {
	host := s.Env().RemoteHost

	s.T().Logf("Collecting installer logs")
	tmpFolder := filepath.Join(consts.BaseConfigPath, "tmp")
	tmpEntries, err := host.ReadDir(tmpFolder)
	if !s.Assert().NoError(err, "should read tmp folder") {
		return
	}
	for _, entry := range tmpEntries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "datadog-agent") {
			logsFolder := filepath.Join(tmpFolder, entry.Name())
			logEntries, err := host.ReadDir(logsFolder)
			if !s.Assert().NoError(err, "should read logs folder") {
				continue
			}
			for _, log := range logEntries {
				s.T().Logf("Found log file: %s", log.Name())
				err = host.GetFile(
					filepath.Join(logsFolder, log.Name()),
					filepath.Join(s.SessionOutputDir(), log.Name()),
				)
				s.Assert().NoError(err, "should download %s", log.Name())
			}
		}
	}
}

// SetCatalogWithCustomPackage sets the catalog with a custom package
// and returns the package config created from the opts.
func (s *BaseSuite) SetCatalogWithCustomPackage(opts ...PackageOption) (TestPackageConfig, error) {
	packageConfig, err := NewPackageConfig(opts...)
	s.Require().NoError(err)
	packageConfig, err = CreatePackageSourceIfLocal(s.Env().RemoteHost, packageConfig)
	s.Require().NoError(err)

	// Set catalog so daemon can find the package
	_, err = s.Installer().SetCatalog(Catalog{
		Packages: []PackageEntry{
			{
				Package: packageConfig.Name,
				Version: packageConfig.Version,
				URL:     packageConfig.URL(),
			},
		},
	})
	return packageConfig, err
}

func (s *BaseSuite) startExperimentWithCustomPackage(opts ...PackageOption) (string, error) {
	packageConfig, err := s.SetCatalogWithCustomPackage(opts...)
	s.Require().NoError(err)
	return s.Installer().StartExperiment(consts.AgentPackage, packageConfig.Version)
}

func (s *BaseSuite) startExperimentPreviousVersion() (string, error) {
	return s.startExperimentWithCustomPackage(WithName(consts.AgentPackage),
		WithPackage(s.StableAgentVersion().OCIPackage()),
	)
}

// MustStartExperimentPreviousVersion starts an experiment with the previous version of the Agent
func (s *BaseSuite) MustStartExperimentPreviousVersion() {
	s.T().Helper()

	// Arrange
	agentVersion := s.StableAgentVersion().Version()

	// Act
	s.WaitForDaemonToStop(func() {
		_, err := s.startExperimentPreviousVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	// Assert
	// have to wait for experiment to finish installing
	s.Require().NoError(s.WaitForInstallerService("Running"))

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

// StartExperimentCurrentVersion starts an experiment of current agent version
func (s *BaseSuite) StartExperimentCurrentVersion() (string, error) {
	return s.startExperimentWithCustomPackage(WithName(consts.AgentPackage),
		WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
}

func (s *BaseSuite) startxperf() {
	host := s.Env().RemoteHost

	err := host.HostArtifactClient.Get("windows-products/xperf-5.0.8169.zip", "C:/xperf.zip")
	s.Require().NoError(err)

	// extract if C:/xperf dir does not exist
	_, err = host.Execute("if (-Not (Test-Path -Path C:/xperf)) { Expand-Archive -Path C:/xperf.zip -DestinationPath C:/xperf }")
	s.Require().NoError(err)

	outputPath := "C:/kernel.etl"
	xperfPath := "C:/xperf/xperf.exe"
	_, err = host.Execute(fmt.Sprintf(`& "%s" -On Base+Latency+CSwitch+PROC_THREAD+LOADER+Profile+DISPATCHER -stackWalk CSwitch+Profile+ReadyThread+ThreadCreate -f %s -MaxBuffers 1024 -BufferSize 1024 -MaxFile 1024 -FileMode Circular`, xperfPath, outputPath))
	s.Require().NoError(err)
}

func (s *BaseSuite) collectxperf() {
	host := s.Env().RemoteHost

	xperfPath := "C:/xperf/xperf.exe"
	outputPath := "C:/full_host_profiles.etl"

	_, err := host.Execute(fmt.Sprintf(`& "%s" -stop -d %s`, xperfPath, outputPath))
	s.Require().NoError(err)

	// collect xperf if the test failed
	if s.T().Failed() {
		outDir := s.SessionOutputDir()
		err = host.GetFile(outputPath, filepath.Join(outDir, "full_host_profiles.etl"))
		s.Require().NoError(err)
	}
}

// startProcdump sets up procdump and starts it in the background.
func (s *BaseSuite) startProcdump() *windowscommon.ProcdumpSession {
	host := s.Env().RemoteHost

	// Setup procdump on remote host
	s.T().Log("Setting up procdump on remote host")
	err := windowscommon.SetupProcdump(host)
	s.Require().NoError(err, "should setup procdump")

	// Start procdump
	ps, err := windowscommon.StartProcdump(host, "agent.exe")
	s.Require().NoError(err, "should start procdump")

	return ps
}

// collectProcdumps stops procdump and downloads any captured dumps if the test failed.
func (s *BaseSuite) collectProcdumps(ps *windowscommon.ProcdumpSession) {
	// Only collect dumps if the test failed
	if !s.T().Failed() {
		ps.Close()
		return
	}

	host := s.Env().RemoteHost

	// Wait for procdump to finish writing dump files BEFORE closing the session.
	// Procdump is configured to capture 5 dumps, so wait until all 5 are created.
	expectedDumpCount := 5
	s.T().Logf("Waiting for procdump to create %d dump files...", expectedDumpCount)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		output, err := host.Execute(fmt.Sprintf(`(Get-ChildItem -Path '%s' -Filter '*.dmp' -ErrorAction SilentlyContinue | Measure-Object).Count`, windowscommon.ProcdumpsPath))
		if err == nil {
			countStr := strings.TrimSpace(output)
			count, parseErr := strconv.Atoi(countStr)
			if parseErr == nil && count >= expectedDumpCount {
				s.T().Logf("All %d dump files ready", count)
				break
			}
			s.T().Logf("Found %s dump files, waiting for %d...", countStr, expectedDumpCount)
		}
		time.Sleep(5 * time.Second)
	}

	ps.Close()

	// Download all dump files
	outDir := s.SessionOutputDir()
	if err := host.GetFolder(windowscommon.ProcdumpsPath, outDir); err != nil {
		s.T().Logf("Warning: failed to download dump %s: %v", windowscommon.ProcdumpsPath, err)
	} else {
		s.T().Logf("Downloaded procdumps to: %s", outDir)
	}
}

// InstallWithXperf installs the MSI with xperf tracing to diagnose service startup issues.
// This wraps the MSI installation with performance tracing and service status checking.
//
// Usage:
//
//	s.InstallWithXperf(
//	    installerwindows.WithMSILogFile("install.log"),
//	)
//
// The xperf trace will be collected automatically if the test fails.
func (s *BaseSuite) InstallWithXperf(opts ...MsiOption) {
	s.T().Helper()

	s.T().Log("Starting xperf tracing")
	s.startxperf()
	defer s.collectxperf()

	s.T().Log("Installing MSI")
	err := s.Installer().Install(opts...)
	s.Require().NoError(err, "MSI installation failed")

	s.T().Log("Checking agent service status after MSI installation")
	err = s.WaitForAgentService("Running")
	s.Require().NoError(err, "Agent service status check failed")

	s.T().Log("MSI installation and service startup completed successfully")
}

// InstallWithDiagnostics installs the MSI with comprehensive diagnostics collection:
// - xperf tracing for system-wide performance analysis
// - procdump collection to capture agent memory dump if it crashes during startup
func (s *BaseSuite) InstallWithDiagnostics(opts ...MsiOption) {
	s.T().Helper()

	// Start xperf tracing
	s.T().Log("Starting xperf tracing")
	s.startxperf()
	defer s.collectxperf()

	// Start procdump in background to capture crash dumps
	s.T().Log("Starting procdump")
	ps := s.startProcdump()
	defer s.collectProcdumps(ps)

	// Proceed with installation
	s.T().Log("Installing MSI")
	err := s.Installer().Install(opts...)
	s.Require().NoError(err, "MSI installation failed")

	// Wait for service to be running
	s.T().Log("Checking agent service status after MSI installation")
	err = s.WaitForAgentService("Running")
	s.Require().NoError(err, "Agent service status check failed")
	s.T().Log("MSI installation and service startup completed successfully")
}

// MustStartExperimentCurrentVersion start an experiment with current version of the Agent
func (s *BaseSuite) MustStartExperimentCurrentVersion() {
	s.T().Helper()

	// Arrange
	agentVersion := s.CurrentAgentVersion().Version()

	// Act
	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	// Assert
	// have to wait for experiment to finish installing
	s.Require().NoError(s.WaitForInstallerService("Running"))

	// sanity check: make sure we did indeed install the current version
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

// AssertSuccessfulAgentStartExperiment that experiment started successfully
func (s *BaseSuite) AssertSuccessfulAgentStartExperiment(version string) {
	s.T().Helper()

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithExperimentVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		HasARunningDatadogAgentService()
}

// AssertSuccessfulAgentPromoteExperiment that experiment was promoted successfully
func (s *BaseSuite) AssertSuccessfulAgentPromoteExperiment(version string) {
	s.T().Helper()

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-agent").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("").
		HasARunningDatadogAgentService()
}

// WaitForInstallerService waits for installer service to be expected state
func (s *BaseSuite) WaitForInstallerService(state string) error {
	// usually waiting after MSI runs so we have to wait awhile
	// max wait is 30*30 -> 900 seconds (15 minutes)
	return s.WaitForServicesWithBackoff(state, []string{consts.ServiceName}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(30))
}

// WaitForAgentService waits for the Datadog Agent service to be in the expected state
func (s *BaseSuite) WaitForAgentService(state string) error {
	// usually waiting after MSI runs so we have to wait awhile
	// max wait is 30*30 -> 900 seconds (15 minutes)
	return s.WaitForServicesWithBackoff(state, []string{"datadogagent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(30))
}

// WaitForServicesWithBackoff waits for the specified services to be in the desired state using backoff retry.
func (s *BaseSuite) WaitForServicesWithBackoff(state string, services []string, opts ...backoff.RetryOption) error {
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		for _, service := range services {
			status, err := windowscommon.GetServiceStatus(s.Env().RemoteHost, service)
			if err != nil {
				return nil, err
			}
			if !strings.Contains(status, state) {
				return nil, fmt.Errorf("service %s is not in state %s, status: %s", service, state, status)
			}
		}
		return nil, nil
	}, opts...)
	return err
}

// AssertSuccessfulConfigStartExperiment that config experiment started successfully
func (s *BaseSuite) AssertSuccessfulConfigStartExperiment(configID string) {
	s.T().Helper()

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasConfigState(consts.AgentPackage).
		WithExperimentConfigEqual(configID).
		HasARunningDatadogAgentService()
}

// AssertSuccessfulConfigPromoteExperiment that config experiment was promoted successfully
func (s *BaseSuite) AssertSuccessfulConfigPromoteExperiment(configID string) {
	s.T().Helper()

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasConfigState(consts.AgentPackage).
		WithStableConfigEqual(configID).
		WithExperimentConfigEqual("").
		HasARunningDatadogAgentService()
}

// AssertSuccessfulConfigStopExperiment that config experiment was stopped successfully
func (s *BaseSuite) AssertSuccessfulConfigStopExperiment() {
	s.T().Helper()

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasConfigState(consts.AgentPackage).
		WithExperimentConfigEqual("").
		HasARunningDatadogAgentService()
}

// WaitForDaemonToStop waits for the daemon service PID or start time to change after the function is called.
func (s *BaseSuite) WaitForDaemonToStop(f func(), opts ...backoff.RetryOption) {
	s.T().Helper()

	// service must be running before we can get the PID
	// might be redundant in some cases but we keep forgetting to ensure it
	// in others and it keeps causing flakes.
	s.Require().NoError(s.WaitForInstallerService("Running"))

	originalPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
	s.Require().NoError(err)
	s.Require().Greater(originalPID, 0)

	originalStartTime, err := windowscommon.GetProcessStartTimeAsFileTimeUtc(s.Env().RemoteHost, originalPID)
	s.Require().NoError(err)

	s.startxperf()
	defer s.collectxperf()

	f()

	_, err = backoff.Retry(context.Background(), func() (any, error) {
		newPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
		if err != nil {
			return nil, err
		}
		if newPID != originalPID {
			// PID changed, the daemon has restarted
			return nil, nil
		}
		// PID is the same, check if start time changed (in case of PID reuse)
		newStartTime, err := windowscommon.GetProcessStartTimeAsFileTimeUtc(s.Env().RemoteHost, newPID)
		if err != nil {
			return nil, err
		}
		if newStartTime != originalStartTime {
			// Start time changed, the daemon has restarted with the same PID
			return nil, nil
		}
		return nil, fmt.Errorf("daemon PID %d with start time %d is still running", newPID, newStartTime)
	}, opts...)
	s.Require().NoError(err)
}
