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
	linuxInstallScriptURL     = "https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh"
	windowsInstallerLatestURL = "https://s3.amazonaws.com/dd-agent/datadog-installer-x86_64.exe"
	macOSInstallScriptURL     = "https://install.datadoghq.com/scripts/install_mac_os.sh"
	// macOSTestingBucket is the bucket deploy_dmg_testing-a7_<arch> publishes pipeline .dmg builds
	// to (.gitlab/deploy/e2e_testing_deploy/e2e_deploy.yml), and the same one
	// test/e2e-framework/components/datadog/agent/host_macos.go points DD_REPO_URL at.
	macOSTestingBucket = "https://dd-agent-macostesting.s3.amazonaws.com"
)

// InstallOption is an optional function parameter type for InstallParams options
type InstallOption func(*installParams)

type installParams struct {
	remoteUpdates        bool
	stablePackages       bool
	stagingPackages      string
	pipelineID           string
	otelCollectorEnabled bool
}

var defaultInstallParams = &installParams{
	remoteUpdates:  false,
	stablePackages: false,
	pipelineID:     os.Getenv("E2E_PIPELINE_ID"),
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
		return a.installWindowsInstallExe(params)
	case e2eos.MacOSFamily:
		return a.installMacOSPipeline(params)
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

func (a *Agent) installWindowsInstallExe(params *installParams) error {
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
	installerURL := windowsInstallerLatestURL
	if !params.stablePackages && params.stagingPackages == "" {
		artifactURL, err := pipeline.GetPipelineArtifact(params.pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
		})
		if err != nil {
			return err
		}
		installerURL = artifactURL
		env["DD_SITE"] = "datad0g.com"
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = "pipeline-" + params.pipelineID
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "installtesting.datad0g.com.internal.dda-testing.com"
	} else if params.stagingPackages != "" {
		env["DD_SITE"] = "datad0g.com"
		installerURL = fmt.Sprintf("https://install.datad0g.com/builds/beta/datadog-installer-%s-1-x86_64.exe", strings.ReplaceAll(params.stagingPackages, "~", "-"))
		env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"] = strings.ReplaceAll(params.stagingPackages, "~", "-") + "-1"
		env["DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"] = "install.datad0g.com.internal.dda-testing.com"
	}
	// Download the installer exe and run it directly, retrying the download since e2e VMs
	// occasionally hit transient network errors right after boot.
	_, err := a.host.RemoteHost.Execute(fmt.Sprintf(`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
	$tempFile = [System.IO.Path]::GetTempFileName() + ".exe";
	$wc = New-Object System.Net.WebClient;
	for ($i=0; $i -lt 3; $i++) {
		try {
			$wc.DownloadFile("%s", $tempFile);
			break
		} catch {
			if ($i -eq 2) { throw }
			Start-Sleep -Seconds 5
		}
	};
	& $tempFile`, installerURL), client.WithEnvVariables(env))
	return err
}

// macOSPipelineArch maps the host's architecture onto the segment deploy_dmg_testing-a7_<arch>
// uses in the macOS testing bucket's pipeline prefix: the host reports "x86_64", but the bucket
// prefix (and host_macos.go's DD_REPO_URL) uses "x64". arm64 matches on both sides.
func macOSPipelineArch(arch e2eos.Architecture) string {
	if arch == e2eos.AMD64Arch {
		return "x64"
	}
	return string(arch)
}

// installMacOSPipeline installs the pipeline build the same way host_macos.go's Pulumi-level
// install does -- the official install script with DD_REPO_URL pointing at the pipeline's
// prefix in the macOS testing bucket -- and then exercises postInstallDatadogAgent's
// PackageType == PackageTypeOCI branch (installWrappedPackage,
// pkg/fleet/installer/packages/datadog_agent_darwin.go), which nothing else does today: macOS
// still ships as a .dmg, and no CI job publishes a real macOS OCI artifact.
//
// It does so by synthesizing, locally on the host, the on-disk shape doInstall's Create() call
// would leave behind for a genuine OCI artifact -- a version directory in the package repository
// holding the .pkg payload, with stable/experiment symlinks pointing at it -- extracted from the
// same pipeline .dmg, then driving the installer's generic `hooks` CLI command
// (pkg/fleet/installer/commands/hooks.go) against it directly. As a side effect this also
// registers the OCI package repository (/opt/datadog-packages/datadog-agent/{stable,experiment}),
// which the real .dmg's postinst does not do yet (see
// priv_notes/fix-macos-dmg-register-package-repository.md) and which configMacOSSuite's
// SetupSuite requires.
func (a *Agent) installMacOSPipeline(params *installParams) error {
	arch := macOSPipelineArch(a.host.RemoteHost.Architecture)
	repoURL := fmt.Sprintf("%s/ci/datadog-agent/pipeline-%s-%s", macOSTestingBucket, params.pipelineID, arch)

	// DD_API_KEY/DD_SITE are set only on the bash invocation (a simple command), not on the
	// whole script: a shell VAR=val prefix cannot apply to a compound statement like the `for`
	// loop below it, so passing them via client.WithEnvVariables (which prepends them to the
	// entire command string) breaks with "parse error near `for'".
	installScript := fmt.Sprintf(
		`for i in 1 2 3 4 5; do curl -fsSL %s -o /tmp/e2e-install-mac-os.sh && break || sleep $i; done && `+
			`DD_API_KEY=%s DD_SITE=datadoghq.com DD_REPO_URL=%s DD_INSTALL_ONLY=true bash /tmp/e2e-install-mac-os.sh`,
		macOSInstallScriptURL, apiKey(), repoURL)
	if _, err := a.host.RemoteHost.Execute(installScript); err != nil {
		return fmt.Errorf("failed to run the pipeline install script: %w", err)
	}

	const (
		dmgPath    = "/tmp/e2e-oci-payload.dmg"
		mountPoint = "/tmp/e2e-oci-payload-mount"
		packageDir = "/opt/datadog-packages/datadog-agent"
		versionDir = packageDir + "/e2e-oci-test"
		installer  = "/opt/datadog-agent/embedded/bin/installer"
	)
	defer a.host.RemoteHost.Execute(fmt.Sprintf(`sudo hdiutil detach %s || true`, mountPoint))

	synthesizeOCIShape := fmt.Sprintf(
		`sudo rm -f %s && sudo curl -fsSL --retry 3 -o %s %s/datadog-agent-7-latest.dmg && `+
			`sudo hdiutil attach %s -mountpoint %s -nobrowse && `+
			`sudo mkdir -p %s && sudo cp "$(find %s -name '*.pkg' | head -n1)" %s/ && `+
			`sudo hdiutil detach %s && `+
			`sudo ln -sfn %s %s/stable && sudo ln -sfn %s %s/experiment`,
		dmgPath, dmgPath, repoURL,
		dmgPath, mountPoint,
		versionDir, mountPoint, versionDir,
		mountPoint,
		versionDir, packageDir, versionDir, packageDir)
	if _, err := a.host.RemoteHost.Execute(synthesizeOCIShape); err != nil {
		return fmt.Errorf("failed to synthesize the on-disk OCI package shape: %w", err)
	}

	hookContext := fmt.Sprintf(`{"hook":"postInstall","package":"datadog-agent","package_type":"oci","package_path":%q}`, versionDir)
	if _, err := a.host.RemoteHost.Execute(fmt.Sprintf(`sudo %s hooks '%s'`, installer, hookContext)); err != nil {
		return fmt.Errorf("failed to run postInstall through the OCI package_type branch: %w", err)
	}

	return nil
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
