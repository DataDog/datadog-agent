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
	remoteUpdates  bool
	stablePackages bool
}

var defaultInstallParams = &installParams{
	remoteUpdates:  false,
	stablePackages: false,
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

// Install installs the agent.
func (a *Agent) Install(options ...InstallOption) error {
	params := defaultInstallParams
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
	if !params.stablePackages {
		env["TESTING_KEYS_URL"] = "apttesting.datad0g.com/test-keys"
		env["TESTING_APT_URL"] = fmt.Sprintf("s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a7", os.Getenv("E2E_PIPELINE_ID"))
		env["TESTING_APT_REPO_VERSION"] = fmt.Sprintf("stable-%s 7", a.host.RemoteHost.Architecture)
		env["TESTING_YUM_URL"] = "s3.amazonaws.com/yumtesting.datad0g.com"
		env["TESTING_YUM_VERSION_PATH"] = fmt.Sprintf("testing/pipeline-%s-a7/7", os.Getenv("E2E_PIPELINE_ID"))
		env["DD_APM_INSTRUMENTATION_PIPELINE_ID"] = os.Getenv("E2E_PIPELINE_ID")
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
	scriptURL := windowsInstallScriptURL
	if !params.stablePackages {
		artifactURL, err := pipeline.GetPipelineArtifact(os.Getenv("E2E_PIPELINE_ID"), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
		})
		if err != nil {
			return err
		}
		env["DD_SITE"] = "datad0g.com"
		env["DD_INSTALLER_URL"] = artifactURL
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = "pipeline-" + os.Getenv("E2E_PIPELINE_ID")
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "installtesting.datad0g.com.internal.dda-testing.com"
		scriptURL = fmt.Sprintf("https://installtesting.datad0g.com/pipeline-%s/scripts/Install-Datadog.ps1", os.Getenv("E2E_PIPELINE_ID"))
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

func apiKey() string {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if apiKey == "" || err != nil {
		apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	return apiKey
}
