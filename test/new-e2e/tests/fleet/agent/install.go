// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
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

// MacOSLocalDMGEnvVar names a .dmg on the machine running the test. When it is set, the macOS
// install path uploads that file and installs from it instead of downloading a pipeline build, and
// it uses this repository's copy of the install script rather than the published one.
//
// This is what makes the macOS install path testable against uncommitted work. Everything the
// install actually exercises on this platform -- the postinstall script, the installer binary it
// invokes, the launchd definitions -- lives inside the .dmg, so a run against a pipeline build
// tests the commit CI built, never the working tree. Build one locally with
// `.gitlab/build/package_build/build_agent_dmg.sh`'s unsigned path (see the comment on
// installMacOSLocalDMG for why unsigned is fine) and point this at omnibus/pkg/*.dmg.
//
// The .dmg must be for the host's architecture, and the host's architecture is not a free choice:
// macOS hosts come from the shared pool (test/e2e-framework/resources/aws/ec2/pool), which hands
// back whichever instance is registered and idle rather than provisioning one to order. Today that
// is mac1.metal, so the .dmg has to be x86_64 -- a build from an Apple Silicon laptop will not run
// on it, and there is no macOS cross-build (tasks/omnibus.py's --arch is Linux-only).
const MacOSLocalDMGEnvVar = "E2E_MACOS_LOCAL_DMG"

// InstallOption is an optional function parameter type for InstallParams options
type InstallOption func(*installParams)

type installParams struct {
	remoteUpdates        bool
	remoteConfig         bool
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

// WithRemoteConfig points the Agent and the installer daemon at the environment's fakeintake for
// Remote Config, so a suite can drive Fleet requests by pushing configurations to it.
//
// The provisioner normally does this through agentparams.WithFakeintake
// (test/e2e-framework/components/datadog/agentparams/params.go). A suite that provisions a bare
// host with ec2.WithoutAgent() and installs the Agent here never reaches that option, and would
// otherwise end up with a daemon polling the real Datadog backend, blind to everything the suite
// pushes. macOS only for now -- it is the only platform installed from this package.
func WithRemoteConfig() InstallOption {
	return func(p *installParams) {
		p.remoteConfig = true
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

// macOS paths and identifiers the install path below touches. The Agent, the installer binary and
// the installer daemon all live under the single install root on this platform
// (pkg/fleet/installer/paths/installer_paths_darwin.go), and both the Agent and the daemon read
// their configuration from the same datadog.yaml -- the daemon's launchd definition passes
// `-c /opt/datadog-agent/etc` (embedded/tmpl/com.datadoghq.installer.plist.tmpl).
const (
	macOSAgentConfigFile   = "/opt/datadog-agent/etc/datadog.yaml"
	macOSInstallerRCDB     = "/opt/datadog-agent/run/remote-config-installer.db"
	macOSInstallerJobLabel = "system/com.datadoghq.installer"
)

// installMacOSPipeline installs the pipeline build exactly the way a customer does -- the official
// install script with DD_REPO_URL pointing at the pipeline's prefix in the macOS testing bucket,
// the same thing host_macos.go's Pulumi-level install runs -- and then applies whatever
// configuration the options asked for.
//
// Deliberately nothing more. macOS ships as a .dmg and the product being delivered manages
// configuration only, not Agent versions, so there is no OCI artifact to install from and no
// upgrade path to exercise. Everything a Fleet configuration experiment needs on disk -- the
// install root, the managed configuration directory, the launchd jobs, and the placeholder OCI
// package repository that the shared configuration code reads unconditionally -- is created by the
// .dmg's own postinstall script (omnibus/package-scripts/agent-dmg/postinst). A test that built
// that state itself would be testing its own scaffolding.
func (a *Agent) installMacOSPipeline(params *installParams) error {
	if localDMG := os.Getenv(MacOSLocalDMGEnvVar); localDMG != "" {
		if err := a.installMacOSLocalDMG(localDMG); err != nil {
			return err
		}
		return a.configureMacOS(params)
	}

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

	return a.configureMacOS(params)
}

// installMacOSLocalDMG installs a .dmg built on the machine running the test, using this
// repository's install script rather than the published one.
//
// Same shape as the pipeline install -- the customer's script, driven by environment variables --
// with the two remote inputs swapped for local ones: DD_DMG_PATH instead of DD_REPO_URL, and the
// working tree's cmd/agent/macos/install_mac_os.sh instead of a curl from install.datadoghq.com.
// That makes the whole install chain branch code: the script, the .dmg, its postinstall script, and
// the installer binary the postinstall script invokes.
//
// The .dmg is unsigned, which is not a compromise: agent_dmg-*-a7 signs only on main, release
// branches and tags (.gitlab/build/package_build/dmg.yml), so every pipeline build this suite has
// ever installed was unsigned too.
func (a *Agent) installMacOSLocalDMG(localDMG string) error {
	if _, err := os.Stat(localDMG); err != nil {
		return fmt.Errorf("%s does not name a readable .dmg: %w", MacOSLocalDMGEnvVar, err)
	}
	installScript, err := repoFile("cmd", "agent", "macos", "install_mac_os.sh")
	if err != nil {
		return err
	}

	const (
		remoteDMG    = "/tmp/e2e-local-agent.dmg"
		remoteScript = "/tmp/e2e-install-mac-os.sh"
	)
	a.host.RemoteHost.CopyFile(localDMG, remoteDMG)
	a.host.RemoteHost.CopyFile(installScript, remoteScript)

	// Same shape as the pipeline invocation below: the variables prefix the `bash` command rather
	// than the whole compound statement, which a shell prefix cannot apply to.
	if _, err := a.host.RemoteHost.Execute(fmt.Sprintf(
		`DD_API_KEY=%s DD_SITE=datadoghq.com DD_DMG_PATH=%s DD_INSTALL_ONLY=true bash %s`,
		apiKey(), remoteDMG, remoteScript,
	)); err != nil {
		return fmt.Errorf("failed to install the local .dmg %s: %w", localDMG, err)
	}
	return nil
}

// repoFile resolves a path relative to the repository root, which the test binary's working
// directory (test/new-e2e) is not.
func repoFile(elem ...string) (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the repository root: runtime.Caller failed")
	}
	// <root>/test/new-e2e/tests/fleet/agent/install.go
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
	path := filepath.Join(append([]string{root}, elem...)...)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot read %s from the repository: %w", filepath.Join(elem...), err)
	}
	return path, nil
}

// configureMacOS applies the option-driven parts of the macOS install by appending to the Agent
// configuration the install script just wrote, then restarting the installer daemon so it picks
// them up.
//
// These are written here rather than passed to the install script as DD_REMOTE_UPDATES and
// friends because the macOS script has no such option: cmd/agent/macos/install_mac_os.sh reads
// DD_API_KEY, DD_SITE, DD_REPO_URL, DD_DMG_PATH and a handful of Agent settings, and nothing about
// remote updates or Remote Config. Passing one would be silently dropped and surface much later as
// an unexplained timeout waiting on a daemon that had already exited.
func (a *Agent) configureMacOS(params *installParams) error {
	var extraConfig strings.Builder

	if params.remoteUpdates {
		// Without this the installer daemon exits as soon as launchd starts it (startDaemon,
		// cmd/installer/subcommands/daemon/run_nix.go), so its local API socket never appears and
		// every installer command against it fails.
		extraConfig.WriteString("remote_updates: true\n")
	}

	if params.remoteConfig {
		if a.host.FakeIntake == nil {
			return errors.New("agent.WithRemoteConfig needs a fakeintake in the environment")
		}
		// The TUF root is derived from fakeintake's fixed signing seed, so it is the same value
		// agentparams.WithFakeintake writes for a provisioner-installed Agent.
		rootJSON, err := fakeintake.RCRootJSON()
		if err != nil {
			return fmt.Errorf("failed to build the fakeintake Remote Config root JSON: %w", err)
		}
		fmt.Fprintf(&extraConfig, `remote_configuration.enabled: true
remote_configuration.rc_dd_url: %s
remote_configuration.no_tls: true
remote_configuration.refresh_interval: 5s
remote_configuration.config_root: '%s'
remote_configuration.director_root: '%s'
`, a.host.FakeIntake.URL, rootJSON, rootJSON)
	}

	if extraConfig.Len() == 0 {
		return nil
	}

	// Staged through a file the unprivileged SSH user can write, then concatenated onto the target
	// under sudo. Appending in place keeps datadog.yaml's _dd-agent ownership and mode -- a
	// redirect run as root would reset both -- and routing the content through a file rather than
	// the command line keeps the single-quoted TUF root JSON out of the shell entirely.
	const stagedConfig = "/tmp/e2e-extra-agent-config.yaml"
	if _, err := a.host.RemoteHost.WriteFile(stagedConfig, []byte(extraConfig.String())); err != nil {
		return fmt.Errorf("failed to stage the extra Agent configuration: %w", err)
	}
	if _, err := a.host.RemoteHost.Execute(
		fmt.Sprintf(`sudo sh -c 'cat %s >> %s'`, stagedConfig, macOSAgentConfigFile),
	); err != nil {
		return fmt.Errorf("failed to append the extra Agent configuration to %s: %w", macOSAgentConfigFile, err)
	}

	// The daemon caches Remote Config state in a database keyed by the backend it was talking to,
	// so a restart alone would have it resume against the previous endpoint. Dropping the database
	// first makes the restart start from an empty client against the endpoint just configured.
	if _, err := a.host.RemoteHost.Execute(fmt.Sprintf(
		`sudo rm -f %s && sudo launchctl kickstart -k %s`, macOSInstallerRCDB, macOSInstallerJobLabel,
	)); err != nil {
		return fmt.Errorf("failed to restart the installer daemon: %w", err)
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
