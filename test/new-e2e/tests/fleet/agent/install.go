// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"
	"strings"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
)

const (
	linuxInstallScriptURL   = "https://install.datadoghq.com/scripts/install_script_agent7.sh"
	windowsInstallScriptURL = "https://install.datadoghq.com/Install-Datadog.ps1"
)

// InstallOption is an optional function parameter type for InstallParams options
type InstallOption func(*installParams)

type installParams struct {
	remoteUpdates         bool
	stablePackages        bool
	stagingPackages       string
	pipelineID            string
	otelCollectorEnabled  bool
	processManagerDisable bool
}

var defaultInstallParams = &installParams{
	remoteUpdates:         false,
	stablePackages:        false,
	pipelineID:            os.Getenv("E2E_PIPELINE_ID"),
	processManagerDisable: ProcessManagerDisabled(),
}

// processManagerEnvVar selects the Linux service manager the fleet tests install with, so every
// fleet test can run under both dd-procmgrd and plain systemd.
const processManagerEnvVar = "E2E_FLEET_PROCESS_MANAGER"

// ProcessManagerEnabled reports whether the agent is installed with dd-procmgrd as its service
// manager. Defaults to true, matching the product default, so an unset variable reproduces a stock
// install. Every Install honours it, so test bodies do not have to pass an option.
func ProcessManagerDisabled() bool {
	return strings.EqualFold(os.Getenv(processManagerEnvVar), "false")
}

// WithRemoteUpdates enables remote updates.
func WithRemoteUpdates() InstallOption {
	return func(p *installParams) {
		p.remoteUpdates = true
	}
}

// WithStablePackages uses the stable packages.
func WithStablePackages() InstallOption {
	return func(p *installParams) {
		p.stablePackages = true
	}
}

// WithStagingPackages uses the staging packages.
func WithStagingPackages(version string) InstallOption {
	return func(p *installParams) {
		p.stagingPackages = version
	}
}

// WithPipelineID overrides the pipeline ID of the agent to install.
func WithPipelineID(pipelineID string) InstallOption {
	return func(p *installParams) {
		p.pipelineID = pipelineID
	}
}

// WithOTelCollectorEnabled sets DD_OTELCOLLECTOR_ENABLED=true during installation,
// causing the DDOT extension to be installed automatically in the postinstall hook.
func WithOTelCollectorEnabled() InstallOption {
	return func(p *installParams) {
		p.otelCollectorEnabled = true
	}
}

// WithProcessManagerDisabled installs with the plain systemd service manager instead of dd-procmgrd.
func WithProcessManagerDisabled() InstallOption {
	return func(p *installParams) {
		p.processManagerDisable = true
	}
}

// Install installs the agent.
func (a *Agent) Install(options ...InstallOption) error {
	paramsCopy := *defaultInstallParams
	params := &paramsCopy
	for _, option := range options {
		option(params)
	}
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return a.installLinuxInstallScript(params)
	case e2eos.WindowsFamily:
		return a.installWindowsInstallScript(params)
	default:
		return fmt.Errorf("unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

// MustInstall installs the agent and panics if it fails.
func (a *Agent) MustInstall(options ...InstallOption) {
	err := a.Install(options...)
	require.NoError(a.t(), err)
}

func (a *Agent) installLinuxInstallScript(params *installParams) error {
	// bugfix for https://major.io/p/systemd-in-fedora-22-failed-to-restart-service-access-denied/
	if a.host.RemoteHost.OSFlavor == e2eos.CentOS && a.host.RemoteHost.OSVersion == e2eos.CentOS7.Version {
		_, err := a.host.RemoteHost.Execute("sudo systemctl daemon-reexec")
		if err != nil {
			return fmt.Errorf("error reexecuting systemd: %w", err)
		}
	}
	// reset failure from previous tests (try up to 3 times)
	var err error
	for i := 0; i < 3; i++ {
		_, err = a.host.RemoteHost.Execute(`sudo systemctl list-units --type=service --all --no-legend --no-pager --output=json | jq -r '.[] | .unit | select(test("^datadog-.*\\.service$"))' | xargs -r -n1 sudo systemctl reset-failed`)
		if err == nil {
			break
		}
		if i < 2 { // Don't sleep after the last attempt
			time.Sleep(time.Second)
		}
	}
	if err != nil {
		return fmt.Errorf("error resetting failed units after 3 attempts: %w", err)
	}

	env := map[string]string{
		"DD_API_KEY": apiKey(),
		"DD_SITE":    "datadoghq.com",
	}
	if params.remoteUpdates {
		env["DD_REMOTE_UPDATES"] = "true"
	}
	if params.otelCollectorEnabled {
		env["DD_OTELCOLLECTOR_ENABLED"] = "true"
	}
	if params.processManagerDisable {
		env[processManagerDisableEnvVar] = "true"
		// Persisting it is not redundant: datadog-agent-installer.service only picks the setting up
		// through EnvironmentFile, so without this a daemon-driven update would revert to
		// dd-procmgrd and the systemd half of the matrix would test nothing.
		if err := a.SetProcessManagerDisable(true); err != nil {
			return err
		}
	}
	if !params.stablePackages && params.stagingPackages == "" {
		env["TESTING_KEYS_URL"] = "apttesting.datad0g.com/test-keys"
		env["TESTING_APT_URL"] = fmt.Sprintf("s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a7", params.pipelineID)
		env["TESTING_APT_REPO_VERSION"] = fmt.Sprintf("stable-%s 7", a.host.RemoteHost.Architecture)
		env["TESTING_YUM_URL"] = "s3.amazonaws.com/yumtesting.datad0g.com"
		env["TESTING_YUM_VERSION_PATH"] = fmt.Sprintf("testing/pipeline-%s-a7/7", params.pipelineID)
		env["DD_APM_INSTRUMENTATION_PIPELINE_ID"] = params.pipelineID
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "installtesting.datad0g.com.internal.dda-testing.com"
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = "pipeline-" + params.pipelineID
		env["DD_INSTALLER_REGISTRY_URL"] = "installtesting.datad0g.com.internal.dda-testing.com"
	} else if params.stagingPackages != "" {
		env["DD_REPO_URL"] = "datad0g.com"
		env["DD_AGENT_MAJOR_VERSION"] = "7"
		env["DD_AGENT_MINOR_VERSION"] = strings.TrimPrefix(params.stagingPackages, "7.")
		env["DD_AGENT_DIST_CHANNEL"] = "beta"
	}
	_, err = a.host.RemoteHost.Execute(fmt.Sprintf(`bash -c "$(curl -L %s)"`, linuxInstallScriptURL), client.WithEnvVariables(env))
	return err
}

func (a *Agent) installWindowsInstallScript(params *installParams) error {
	env := map[string]string{
		"DD_API_KEY": apiKey(),
		"DD_SITE":    "datadoghq.com",
	}
	if params.remoteUpdates {
		env["DD_REMOTE_UPDATES"] = "true"
	}
	if params.otelCollectorEnabled {
		env["DD_OTELCOLLECTOR_ENABLED"] = "true"
	}
	scriptURL := windowsInstallScriptURL
	if !params.stablePackages && params.stagingPackages == "" {
		artifactURL, err := pipeline.GetPipelineArtifact(params.pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
		})
		if err != nil {
			return err
		}
		env["DD_SITE"] = "datad0g.com"
		env["DD_INSTALLER_URL"] = artifactURL
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = "pipeline-" + params.pipelineID
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "installtesting.datad0g.com.internal.dda-testing.com"
		scriptURL = fmt.Sprintf("https://installtesting.datad0g.com/pipeline-%s/scripts/Install-Datadog.ps1", os.Getenv("E2E_PIPELINE_ID"))
	} else if params.stagingPackages != "" {
		env["DD_SITE"] = "datad0g.com"
		env["DD_INSTALLER_URL"] = fmt.Sprintf("https://install.datad0g.com/builds/beta/datadog-installer-%s-1-x86_64.exe", strings.ReplaceAll(params.stagingPackages, "~", "-"))
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = strings.ReplaceAll(params.stagingPackages, "~", "-") + "-1"
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "install.datad0g.com.internal.dda-testing.com"
		scriptURL = fmt.Sprintf("https://install.datad0g.com/builds/beta/Install-Datadog-%s-1.ps1", strings.ReplaceAll(params.stagingPackages, "~", "-"))
	}
	_, err := a.host.RemoteHost.Execute(fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
	[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
	iex ((New-Object System.Net.WebClient).DownloadString('%s'))`, scriptURL), client.WithEnvVariables(env))
	return err
}

// Uninstall uninstalls the agent.
func (a *Agent) Uninstall() error {
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return a.uninstallLinux()
	case e2eos.WindowsFamily:
		return a.uninstallWindows()
	default:
		return fmt.Errorf("unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

// MustUninstall uninstalls the agent and panics if it fails.
func (a *Agent) MustUninstall() {
	err := a.Uninstall()
	require.NoError(a.t(), err)
}

func (a *Agent) uninstallLinux() error {
	_, err := a.host.RemoteHost.Execute("sudo apt-get remove -y --purge datadog-agent || sudo yum remove -y datadog-agent || sudo zypper remove -y datadog-agent")
	if err != nil {
		return err
	}
	_, err = a.host.RemoteHost.Execute("sudo rm -rf /etc/datadog-agent")
	return err
}

func (a *Agent) uninstallWindows() error {
	_, err := a.host.RemoteHost.Execute(`$productCode = (@(Get-ChildItem -Path "HKLM:SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall" -Recurse) | Where {$_.GetValue("DisplayName") -like "Datadog Agent" }).PSChildName;
start-process msiexec -Wait -ArgumentList ('/log', 'C:\uninst.log', '/q', '/x', "$productCode", 'REBOOT=ReallySuppress')`)
	if err != nil {
		return err
	}
	_, err = a.host.RemoteHost.Execute(`cmd /c rmdir /s /q "C:\ProgramData\Datadog"`)
	return err
}

const (
	// agentEnvironmentFile is sourced by datadog-agent-installer.service via EnvironmentFile=-, so it
	// is how an operator makes a service manager choice survive a Fleet-driven update.
	agentEnvironmentFile = "/etc/datadog-agent/environment"
	// processManagerDisableEnvVar is an opt-out, so it is only ever written to disable dd-procmgrd.
	processManagerDisableEnvVar = "DD_PROCESS_MANAGER_DISABLE"
)

// SetProcessManagerEnabled persists the service manager choice on the host. Enabling clears the
// variable rather than writing a value, mirroring the installer: procmgr is the default.
func (a *Agent) SetProcessManagerDisable(disable bool) error {
	if a.host.RemoteHost.OSFamily != e2eos.LinuxFamily {
		return nil
	}
	cmd := fmt.Sprintf(
		`sudo mkdir -p %s && sudo sed -i '/^%s=/d' %s 2>/dev/null; true`,
		"/etc/datadog-agent", processManagerDisableEnvVar, agentEnvironmentFile,
	)
	if _, err := a.host.RemoteHost.Execute(cmd); err != nil {
		return fmt.Errorf("could not reset the process manager setting: %w", err)
	}
	if disable {
		return nil
	}
	_, err := a.host.RemoteHost.Execute(fmt.Sprintf(
		`echo '%s=true' | sudo tee -a %s > /dev/null`, processManagerDisableEnvVar, agentEnvironmentFile,
	))
	if err != nil {
		return fmt.Errorf("could not persist the process manager setting: %w", err)
	}
	return nil
}

// MustSetProcessManagerEnabled persists the service manager choice and fails the test on error.
func (a *Agent) MustSetProcessManagerDisable(disable bool) {
	require.NoError(a.t(), a.SetProcessManagerDisable(disable))
}

// SwitchProcessManager persists the service manager choice and restarts the Agent so that it takes
// effect on a running host.
//
// The restart is required, not tidiness: the installer daemon reads the environment file only when
// it starts, and hooks inherit the daemon's environment, so an edit is invisible to a Fleet-driven
// update until the daemon has been recycled.
func (a *Agent) SwitchProcessManager(enabled bool) error {
	if err := a.SetProcessManagerEnabled(enabled); err != nil {
		return err
	}
	if a.host.RemoteHost.OSFamily != e2eos.LinuxFamily {
		return nil
	}
	if _, err := a.host.RemoteHost.Execute("sudo systemctl restart datadog-agent.service"); err != nil {
		return fmt.Errorf("could not restart the Agent to pick up the process manager setting: %w", err)
	}
	return nil
}

// MustSwitchProcessManager switches the service manager and fails the test on error.
func (a *Agent) MustSwitchProcessManager(enabled bool) {
	require.NoError(a.t(), a.SwitchProcessManager(enabled))
}

func apiKey() string {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if apiKey == "" || err != nil {
		apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	return apiKey
}
