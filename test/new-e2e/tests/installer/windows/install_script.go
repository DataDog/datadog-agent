// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"path/filepath"
	"strings"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
)

// baseInstaller represents common functionality for Datadog installers
type baseInstaller struct {
	env    *environments.WindowsHost
	params Params
}

// newBaseInstaller creates a new base installer with common initialization
func newBaseInstaller(env *environments.WindowsHost, opts ...Option) baseInstaller {
	params := Params{
		extraEnvVars: make(map[string]string),
	}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		panic(err)
	}
	return baseInstaller{
		env:    env,
		params: params,
	}
}

// getInstallerURL gets the installer URL, either from params or by fetching from pipeline
func (b *baseInstaller) getInstallerURL() (string, error) {
	if b.params.installerURL != "" {
		return b.params.installerURL, nil
	}

	artifactURL, err := pipeline.GetPipelineArtifact(b.env.Environment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
		return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
	})
	if err != nil {
		return "", err
	}
	return artifactURL, nil
}

// getBaseEnvVars returns the common environment variables for installation
func (b *baseInstaller) getBaseEnvVars() map[string]string {
	envVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	for k, v := range b.params.extraEnvVars {
		envVars[k] = v
	}
	envVars["DD_REMOTE_UPDATES"] = "true"
	return envVars
}

// DatadogInstallScript represents an interface to the Datadog Install script on the remote host.
type DatadogInstallScript struct {
	baseInstaller
}

// NewDatadogInstallScript instantiates a new instance of the Datadog Install Script running on
// a remote Windows host.
func NewDatadogInstallScript(env *environments.WindowsHost, opts ...Option) *DatadogInstallScript {
	return &DatadogInstallScript{
		baseInstaller: newBaseInstaller(env, opts...),
	}
}

// handleLocalFile handles a file:// URL by copying it to the remote host and returning the remote path
//
// Reuses the file extension of the original file since executables need .exe extension on Windows.
func handleLocalFile(host *components.RemoteHost, url string) (string, error) {
	if !strings.HasPrefix(url, "file://") {
		return url, nil
	}
	localPath := strings.TrimPrefix(url, "file://")
	// Get the file extension from the local path
	ext := filepath.Ext(localPath)
	remotePath, err := common.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	// Add the extension to the remote path
	remotePath = remotePath + ext
	host.CopyFile(localPath, remotePath)
	return remotePath, nil
}

// Run runs the Datadog Installer install script on the remote host.
func (d *DatadogInstallScript) Run(opts ...Option) (string, error) {
	err := optional.ApplyOptions(&d.params, opts)
	if err != nil {
		return "", err
	}

	installerURL, err := d.getInstallerURL()
	if err != nil {
		return "", err
	}

	// Handle local installer URL
	installerPath, err := handleLocalFile(d.env.RemoteHost, installerURL)
	if err != nil {
		return "", err
	}

	if d.params.installerScript == "" {
		d.params.installerScript = fmt.Sprintf("https://installtesting.datad0g.com/pipeline-%s/scripts/Install-Datadog.ps1", d.env.Environment.PipelineID())
	}

	// Handle local script URL
	scriptPath, err := handleLocalFile(d.env.RemoteHost, d.params.installerScript)
	if err != nil {
		return "", err
	}

	// Set the environment variables for the install script
	envVars := d.getBaseEnvVars()
	envVars["DD_INSTALLER_URL"] = installerPath

	// Use different commands for local vs remote script
	var cmd string
	if strings.HasPrefix(d.params.installerScript, "file://") {
		cmd = fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
			& "%s"`, scriptPath)
	} else {
		cmd = fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
			[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
			iex ((New-Object System.Net.WebClient).DownloadString('%s'))`, scriptPath)
	}
	return d.env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}

// SetupScriptRunner represents an interface for installing Datadog on Windows
type SetupScriptRunner interface {
	Run(opts ...Option) (string, error)
}

// DatadogInstallExe represents an interface to the Datadog Installer exe on the remote host.
type DatadogInstallExe struct {
	baseInstaller
}

// NewDatadogInstallExe instantiates a new instance of the Datadog Installer exe running on
// a remote Windows host.
func NewDatadogInstallExe(env *environments.WindowsHost, opts ...Option) *DatadogInstallExe {
	return &DatadogInstallExe{
		baseInstaller: newBaseInstaller(env, opts...),
	}
}

// Run runs the Datadog Installer exe on the remote host.
func (d *DatadogInstallExe) Run(opts ...Option) (string, error) {
	err := optional.ApplyOptions(&d.params, opts)
	if err != nil {
		return "", err
	}

	installerURL, err := d.getInstallerURL()
	if err != nil {
		return "", err
	}

	// Handle local installer URL
	installerPath, err := handleLocalFile(d.env.RemoteHost, installerURL)
	if err != nil {
		return "", err
	}

	// Set the environment variables for the install script
	envVars := d.getBaseEnvVars()
	// TODO: exe explicitly fails when this env var is set, but it doesn't appear used anywhere
	delete(envVars, "DD_INSTALLER")

	// Use different commands for local vs remote installer
	var cmd string
	if strings.HasPrefix(installerURL, "file://") {
		cmd = fmt.Sprintf(`& "%s" setup --flavor default`, installerPath)
	} else {
		cmd = fmt.Sprintf(`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
			$tempFile = [System.IO.Path]::GetTempFileName() + ".exe";
			(New-Object System.Net.WebClient).DownloadFile("%s", $tempFile);
			& $tempFile setup --flavor default`, installerPath)
	}
	return d.env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}
