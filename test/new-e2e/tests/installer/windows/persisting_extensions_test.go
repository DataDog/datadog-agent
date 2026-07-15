// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v6"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/ddot"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	// stableExtensionsVersion is the OCI tag for the staging build used as the "stable" base for
	// extension persistence tests. It must point to a build that already includes extension support.
	stableExtensionsVersion = "7.78.0-beta-fleet-ext-1"
	// stableExtensionsAgentVersion is the version as reported by the agent binary (OCI tag without the -1 package suffix).
	stableExtensionsAgentVersion = "7.78.0-beta-fleet-ext"
)

type testExtensionsSuite struct {
	testAgentUpgradeSuite
}

// TestExtensionPersistence tests Agent extension persistence behaviour on Windows.
func TestExtensionPersistence(t *testing.T) {
	s := &testExtensionsSuite{}
	s.CreateStableAgent = func() (*AgentVersionManager, error) {
		oci, err := NewPackageConfig(
			WithName(consts.AgentPackage),
			WithVersion(stableExtensionsVersion),
			WithRegistry(consts.BetaS3OCIRegistry),
		)
		if err != nil {
			return nil, err
		}
		msi, err := windowsagent.NewPackage(
			windowsagent.WithProduct("datadog-agent"),
			windowsagent.WithArch("x86_64"),
			windowsagent.WithChannel("beta"),
			windowsagent.WithVersion(stableExtensionsVersion),
		)
		if err != nil {
			return nil, err
		}
		return NewAgentVersionManager(stableExtensionsAgentVersion, stableExtensionsVersion, oci, msi)
	}
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// runInstallerCommand runs a datadog-installer.exe command on the remote host.
func (s *testExtensionsSuite) runInstallerCommand(args string) (string, error) {
	binaryPath := fmt.Sprintf(`%s\%s`, consts.Path, consts.BinaryName)
	envVars := fmt.Sprintf(`$env:DD_API_KEY='%s'; $env:DD_SITE='datadoghq.com'; `, installer.GetAPIKey())
	cmd := fmt.Sprintf(`%s& "%s" %s`, envVars, binaryPath, args)
	return s.Env().RemoteHost.Execute(cmd)
}

// installExtension installs an extension using the datadog-installer.
func (s *testExtensionsSuite) installExtension(pkg TestPackageConfig, extensionName string) {
	output, err := s.runInstallerCommand(fmt.Sprintf("extension install %s %s", pkg.URL(), extensionName))
	s.Require().NoError(err, "Failed to install extension: %s", output)
}

// verifyDDOTRunningSCM asserts the legacy Windows SCM path: datadog-otel-agent is Running.
// Used for the stable baseline (pre–OCI-procmgr DDOT) where the collector runs as that service.
// If expectedVersion is non-empty, also checks the service PathName contains that version.
func (s *testExtensionsSuite) verifyDDOTRunningSCM(expectedVersion string) {
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
	if expectedVersion == "" {
		return
	}
	assert.Eventually(s.T(), func() bool {
		binaryPath, err := s.Env().RemoteHost.Execute(
			`(Get-WmiObject -Class Win32_Service -Filter "Name='datadog-otel-agent'").PathName`)
		if err != nil {
			return false
		}
		return strings.Contains(binaryPath, expectedVersion)
	}, 2*time.Minute, 2*time.Second, "DDOT SCM service should be running at version %s", expectedVersion)
}

// verifyDDOTRunningProcmgr waits for procmgr + stopped legacy otel, asserts DDOT under procmgr, then (if expectedVersion != "")
// that dd-procmgr describe Command contains that version (same convention as verifyDDOTRunningSCM).
func (s *testExtensionsSuite) verifyDDOTRunningProcmgr(expectedVersion string) {
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"dd-procmgr-service"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
	s.Require().NoError(s.WaitForServicesWithBackoff("Stopped", []string{"datadog-otel-agent"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
	ddot.AssertDDOTManagedByProcmgrWindows(s.T(), s.Env().RemoteHost)
	if expectedVersion == "" {
		return
	}
	installRoot, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	assert.Eventually(s.T(), func() bool {
		cmdLine, err := ddot.WindowsDescribeDDOTCommandLine(s.Env().RemoteHost, cli)
		if err != nil || cmdLine == "" {
			return false
		}
		return strings.Contains(cmdLine, expectedVersion)
	}, 2*time.Minute, 2*time.Second, "dd-procmgr describe Command should contain version %s", expectedVersion)
}

// pollWindowsServiceStatus runs Get-Service on the remote host and returns trimmed output:
// "NotFound" if the service is absent, otherwise the status string (e.g. Running, Stopped).
func (s *testExtensionsSuite) pollWindowsServiceStatus(serviceName string) (output string, ok bool) {
	cmd := fmt.Sprintf(
		`$svc = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $svc) { Write-Output "NotFound" } else { Write-Output $svc.Status }`,
		serviceName)
	out, err := s.Env().RemoteHost.Execute(cmd)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// verifyServiceNotFound waits until the named Windows service is absent from SCM.
func (s *testExtensionsSuite) verifyServiceNotFound(serviceName string) {
	assert.Eventually(s.T(), func() bool {
		out, ok := s.pollWindowsServiceStatus(serviceName)
		if !ok {
			return false
		}
		return strings.Contains(out, "NotFound")
	}, 5*time.Minute, 2*time.Second, "Windows service %q should be removed from SCM", serviceName)
}

// setAgentConfig creates the agent configuration with the given OCI registry URL.
func (s *testExtensionsSuite) setAgentConfig(registryURL string) {
	configPath := `C:\ProgramData\Datadog\datadog.yaml`
	s.Env().RemoteHost.MkdirAll(`C:\ProgramData\Datadog`)
	apiKey := installer.GetAPIKey()
	registryBlock := ""
	if registryURL != "" {
		registryBlock = `
installer:
  registry:
    url: ` + registryURL
	}
	s.Env().RemoteHost.WriteFile(configPath, []byte(`
api_key: `+apiKey+`
site: datadoghq.com
remote_updates: true
log_level: debug`+registryBlock+`
`))
}

// installPreviousAgentVersion installs the previous stable agent version.
func (s *testExtensionsSuite) installPreviousAgentVersion(opts ...MsiOption) {
	options := []MsiOption{
		WithOption(WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-previous-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

// installCurrentAgentVersion installs the current agent version.
func (s *testExtensionsSuite) installCurrentAgentVersion(opts ...MsiOption) {
	options := []MsiOption{
		WithOption(WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		WithMSILogFile("install-current-version.log"),
	}
	options = append(options, opts...)
	s.InstallWithDiagnostics(options...)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestExtensionPersistThroughMSIUpgrade tests that extensions survive a manual MSI upgrade.
//
// Scenario: Install previous MSI -> install extension -> upgrade MSI -> verify extension restored
func (s *testExtensionsSuite) TestExtensionPersistThroughMSIUpgrade() {
	s.setAgentConfig(consts.PipelineOCIRegistry)
	s.installPreviousAgentVersion()
	defer func() {
		if err := s.Installer().Uninstall(WithMSILogFile("teardown-uninstall-persist-through-upgrade.log")); err != nil {
			s.T().Logf("teardown: uninstall agent: %v", err)
		}
	}()
	s.installExtension(s.StableAgentVersion().OCIPackage(), "ddot")
	s.verifyDDOTRunningSCM(s.StableAgentVersion().Version())
	s.installCurrentAgentVersion()
	s.verifyDDOTRunningProcmgr(s.CurrentAgentVersion().Version())
}

// TestExtensionRestoredOnMSIRollback tests that extensions are restored when an MSI upgrade fails and rolls back.
//
// Scenario: Install previous MSI -> install extension -> upgrade with WIXFAILWHENDEFERRED=1
// -> verify rollback restores old version -> verify extension is restored
func (s *testExtensionsSuite) TestExtensionRestoredOnMSIRollback() {
	// Start with no registry override so the daemon and install-experiment subprocess
	// both start with RegistryOverride = "". This ensures StartExperiment successfully
	// downloads the pipeline package from the catalog URL (installtesting.datad0g.com).
	s.setAgentConfig("")
	s.installPreviousAgentVersion()
	defer func() {
		if err := s.Installer().Uninstall(WithMSILogFile("teardown-uninstall-msi-rollback.log")); err != nil {
			s.T().Logf("teardown: uninstall agent: %v", err)
		}
	}()
	s.installExtension(s.StableAgentVersion().OCIPackage(), "ddot")
	s.verifyDDOTRunningSCM(s.StableAgentVersion().Version())
	s.setExperimentMSIArgs([]string{"WIXFAILWHENDEFERRED=1"})

	// Override MSI args to include the pipeline registry URL for the experiment MSI.
	s.setExperimentMSIArgs([]string{"WIXFAILWHENDEFERRED=1", "DD_INSTALLER_REGISTRY_URL=" + consts.PipelineOCIRegistry})

	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")

		// Set installer.registry.url to BetaS3OCIRegistry so that restoreAgentExtensions
		// (called from the MSI postinst hook during stable reinstatement after rollback)
		// uses the correct registry for the stable beta package.
		// The experiment MSI's rollback postinst already has DD_INSTALLER_REGISTRY_URL=PipelineOCIRegistry
		// (from StartExperimentMSIArgs), which takes precedence over datadog.yaml for that subprocess.
		// The stable MSI's postinst has no DD_INSTALLER_REGISTRY_URL, so it reads from datadog.yaml
		// and uses BetaS3OCIRegistry to download agent-package:<stableVersion>.
		s.setAgentConfig(consts.BetaS3OCIRegistry)
	})

	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	err = s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)

	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	s.verifyDDOTRunningSCM(s.StableAgentVersion().Version())
}

// TestExtensionRemovedOnUninstall tests that extensions are cleaned up on uninstall.
//
// Scenario: Install MSI -> install extension -> uninstall MSI -> verify extension removed
func (s *testExtensionsSuite) TestExtensionRemovedOnUninstall() {
	s.setAgentConfig(consts.PipelineOCIRegistry)

	// 1. Install current agent version
	s.installCurrentAgentVersion()
	defer func() {
		if err := s.Installer().Uninstall(WithMSILogFile("teardown-uninstall-extension-removed-on-uninstall.log")); err != nil {
			s.T().Logf("teardown: uninstall agent: %v", err)
		}
	}()

	// 2. Install DDOT extension
	s.installExtension(s.CurrentAgentVersion().OCIPackage(), "ddot")

	// 3. Verify DDOT is running (OCI layout: under dd-procmgr-service)
	s.verifyDDOTRunningProcmgr(s.CurrentAgentVersion().Version())

	// 4. Uninstall agent
	err := s.Installer().Uninstall()
	s.Require().NoError(err, "Failed to uninstall agent")

	// 5. Legacy otel and procmgr SCM services must be gone
	s.verifyServiceNotFound("datadog-otel-agent")
	s.verifyServiceNotFound("dd-procmgr-service")
}
