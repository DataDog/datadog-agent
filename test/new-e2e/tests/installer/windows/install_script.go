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

// baseInstaller represents common functionality for Datadog installers.
// It provides shared methods for handling installer URLs, environment variables,
// and file operations that are common across different installer types.
type baseInstaller struct {
	env    *environments.WindowsHost
	params Params
}

// newBaseInstaller creates a new base installer with common initialization.
// It sets up the default parameters and applies any provided options.
// Panics if any option application fails, as this is a programming error.
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

// getInstallerURL gets the installer URL, either from params or by fetching from pipeline.
// If no URL is provided in params, it attempts to fetch the latest installer from the pipeline.
// Returns an error if the URL cannot be determined or if the pipeline fetch fails.
func (b *baseInstaller) getInstallerURL(params Params) (string, error) {
	if params.installerURL != "" {
		return params.installerURL, nil
	}

	artifactURL, err := pipeline.GetPipelineArtifact(b.env.Environment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
		return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
	})
	if err != nil {
		return "", err
	}
	return artifactURL, nil
}

// getBaseEnvVars returns the common environment variables for installation.
// It combines the default environment variables with any extra variables provided in params.
// Always sets DD_REMOTE_UPDATES to true to ensure remote updates are enabled.
func (b *baseInstaller) getBaseEnvVars() map[string]string {
	envVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	for k, v := range b.params.extraEnvVars {
		envVars[k] = v
	}
	envVars["DD_REMOTE_UPDATES"] = "true"
	return envVars
}

// DatadogInstallScript represents an interface to the Datadog Install script on the remote host.
// It handles the installation process using a PowerShell script approach.
type DatadogInstallScript struct {
	baseInstaller
}

// NewDatadogInstallScript instantiates a new instance of the Datadog Install Script running on
// a remote Windows host. It initializes the base installer with the provided options.
func NewDatadogInstallScript(env *environments.WindowsHost, opts ...Option) *DatadogInstallScript {
	return &DatadogInstallScript{
		baseInstaller: newBaseInstaller(env, opts...),
	}
}

// copyFileURLsToHost handles a file:// URL by copying it to the remote host and returning the remote path.
// If the URL is not a file:// URL, it returns the original URL unchanged.
// Reuses the file extension of the original file since executables need .exe extension on Windows.
func copyFileURLsToHost(host *components.RemoteHost, url string) (string, error) {
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

// prepareInstaller prepares the installer for execution by handling local file URLs.
// If the URL is a local file (starts with file://), it copies the file to the remote host.
// Returns the path to the installer on the remote host or the original URL if it's not a local file.
func (b *baseInstaller) prepareInstaller(params Params) (string, error) {
	installerURL, err := b.getInstallerURL(params)
	if err != nil {
		return "", err
	}

	// Handle local installer URL
	installerPath, err := copyFileURLsToHost(b.env.RemoteHost, installerURL)
	if err != nil {
		return "", err
	}

	return installerPath, nil
}

// prepareEnvVars prepares the environment variables for the installation.
// It combines the base environment variables with any extra variables provided in params.
// This is the base implementation that can be extended by specific installer types.
func (b *baseInstaller) prepareEnvVars(params Params) map[string]string {
	envVars := b.getBaseEnvVars()
	for k, v := range params.extraEnvVars {
		envVars[k] = v
	}
	return envVars
}

// prepareScript prepares the installation script for execution.
// If no script URL is provided, it generates a default URL based on the pipeline ID.
// Handles local file URLs by copying the script to the remote host.
func (d *DatadogInstallScript) prepareScript(params Params) (string, error) {
	if params.installerScript == "" {
		params.installerScript = fmt.Sprintf("https://installtesting.datad0g.com/pipeline-%s/scripts/Install-Datadog.ps1", d.env.Environment.PipelineID())
	}

	// Handle local script URL
	scriptPath, err := copyFileURLsToHost(d.env.RemoteHost, params.installerScript)
	if err != nil {
		return "", err
	}

	return scriptPath, nil
}

// Run runs the Datadog Installer install script on the remote host.
func (d *DatadogInstallScript) Run(opts ...Option) (string, error) {
	// Start with a copy of the base params
	params := d.params
	params.extraEnvVars = make(map[string]string)
	for k, v := range d.params.extraEnvVars {
		params.extraEnvVars[k] = v
	}

	// Apply method-specific options
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return "", err
	}

	installerPath, err := d.prepareInstaller(params)
	if err != nil {
		return "", err
	}

	scriptPath, err := d.prepareScript(params)
	if err != nil {
		return "", err
	}

	// Prepare environment variables
	envVars := d.baseInstaller.prepareEnvVars(params)
	if installerPath != "" {
		envVars["DD_INSTALLER_URL"] = installerPath
	}

	// Build the PowerShell command
	var cmd string
	if strings.HasPrefix(params.installerScript, "file://") {
		cmd = fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
			& "%s"`, scriptPath)
	} else {
		cmd = fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
			[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
			iex ((New-Object System.Net.WebClient).DownloadString('%s'))`, scriptPath)
	}

	return d.env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}

// InstallScriptRunner represents an interface for installing Datadog on Windows.
type InstallScriptRunner interface {
	Run(opts ...Option) (string, error)
}

// DatadogInstallExe represents an interface to the Datadog Installer exe on the remote host.
// It handles the installation process using a direct executable approach.
type DatadogInstallExe struct {
	baseInstaller
}

// NewDatadogInstallExe instantiates a new instance of the Datadog Installer exe running on
// a remote Windows host. It initializes the base installer with the provided options.
func NewDatadogInstallExe(env *environments.WindowsHost, opts ...Option) *DatadogInstallExe {
	return &DatadogInstallExe{
		baseInstaller: newBaseInstaller(env, opts...),
	}
}

// Run runs the Datadog Installer exe on the remote host.
func (d *DatadogInstallExe) Run(opts ...Option) (string, error) {
	// Start with a copy of the base params
	params := d.params
	params.extraEnvVars = make(map[string]string)
	for k, v := range d.params.extraEnvVars {
		params.extraEnvVars[k] = v
	}

	// Apply method-specific options
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return "", err
	}

	installerPath, err := d.prepareInstaller(params)
	if err != nil {
		return "", err
	}

	// Prepare environment variables
	envVars := d.baseInstaller.prepareEnvVars(params)
	// TODO: exe explicitly fails when this env var is set, but it doesn't appear used anywhere
	delete(envVars, "DD_INSTALLER")

	// Build the PowerShell command
	var cmd string
	if strings.HasPrefix(params.installerURL, "file://") {
		cmd = fmt.Sprintf(`& "%s" setup --flavor default`, installerPath)
	} else {
		cmd = fmt.Sprintf(`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
			$tempFile = [System.IO.Path]::GetTempFileName() + ".exe";
			(New-Object System.Net.WebClient).DownloadFile("%s", $tempFile);
			& $tempFile setup --flavor default`, installerPath)
	}

	return d.env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}
