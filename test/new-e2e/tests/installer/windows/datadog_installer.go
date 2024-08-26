// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains code for the E2E tests for the Datadog installer on Windows
package installer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// AgentPackage is the name of the Datadog Agent package
	// We use a constant to make it easier for calling code, because depending on the context
	// the Agent package can be referred to as "agent-package" (like in the OCI registry) or "datadog-agent" (in the
	// local database once the Agent is installed).
	AgentPackage string = "datadog-agent"
	// Path is the path where the Datadog Installer is installed on disk
	Path string = "C:\\Program Files\\Datadog\\Datadog Installer"
	// BinaryName is the name of the Datadog Installer binary on disk
	BinaryName string = "datadog-installer.exe"
	// ServiceName the installer service name
	ServiceName string = "Datadog Installer"
	// ConfigPath is the location of the Datadog Installer's configuration on disk
	ConfigPath string = "C:\\ProgramData\\Datadog\\datadog.yaml"
)

var (
	// BinaryPath is the path of the Datadog Installer binary on disk
	BinaryPath = path.Join(Path, BinaryName)
)

// DatadogInstaller represents an interface to the Datadog Installer on the remote host.
type DatadogInstaller struct {
	binaryPath string
	env        *environments.WindowsHost
	logPath    string
}

// NewDatadogInstaller instantiates a new instance of the Datadog Installer running
// on a remote Windows host.
func NewDatadogInstaller(env *environments.WindowsHost, logPath string) *DatadogInstaller {
	return &DatadogInstaller{
		binaryPath: path.Join(Path, BinaryName),
		env:        env,
		logPath:    logPath,
	}
}

func (d *DatadogInstaller) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Version returns the version of the Datadog Installer on the host.
func (d *DatadogInstaller) Version() (string, error) {
	return d.execute("version")
}

func (d *DatadogInstaller) runCommand(command, packageName string, opts ...installer.PackageOption) (string, error) {
	var packageConfigFound = false
	var packageConfig installer.TestPackageConfig
	for _, packageConfig = range installer.PackagesConfig {
		if packageConfig.Name == packageName {
			packageConfigFound = true
			break
		}
	}

	if !packageConfigFound {
		return "", fmt.Errorf("unknown package %s", packageName)
	}

	err := optional.ApplyOptions(&packageConfig, opts)
	if err != nil {
		return "", nil
	}

	registryTag := packageName
	if packageConfig.Alias != "" {
		registryTag = packageConfig.Alias
	}

	envVars := installer.InstallScriptEnvWithPackages(e2eos.AMD64Arch, []installer.TestPackageConfig{packageConfig})
	packageURL := fmt.Sprintf("oci://%s/%s:%s", packageConfig.Registry, registryTag, packageConfig.Version)

	return d.execute(fmt.Sprintf("%s %s", command, packageURL), client.WithEnvVariables(envVars))
}

// InstallPackage will attempt to use the Datadog Installer to install the package given in parameter.
// version: A function that returns the version of the package to install. By default, it will install
// the package matching the current pipeline. This is a function so that it can be combined with
// Note that this command is a direct command and won't go through the Daemon.
func (d *DatadogInstaller) InstallPackage(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install", packageName, opts...)
}

// InstallExperiment will attempt to use the Datadog Installer to start an experiment for the package given in parameter.
func (d *DatadogInstaller) InstallExperiment(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install-experiment", packageName, opts...)
}

// RemovePackage requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemovePackage(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove %s", packageName))
}

// RemoveExperiment requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemoveExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove-experiment %s", packageName))
}

// Params contains the optional parameters for the Datadog Installer Install command
type Params struct {
	installerURL string
	msiArgs      []string
}

// Option is an optional function parameter type for the Datadog Installer Install command
type Option func(*Params) error

// WithInstallerURL uses a specific URL for the Datadog Installer Install command instead of using the pipeline URL.
func WithInstallerURL(installerURL string) Option {
	return func(params *Params) error {
		params.installerURL = installerURL
		return nil
	}
}

// WithMSIArg uses a specific URL for the Datadog Installer Install command instead of using the pipeline URL.
func WithMSIArg(arg string) Option {
	return func(params *Params) error {
		params.msiArgs = append(params.msiArgs, arg)
		return nil
	}
}

// WithInstallerURLFromInstallersJSON uses a specific URL for the Datadog Installer from an installers_v2.json
// file.
// bucket: The S3 bucket to look for the installers_v2.json file, i.e. "dd-agent-mstesting"
// channel: The channel in the bucket, i.e. "stable"
// version: The artifact version to retrieve, i.e. "7.56.0-installer-0.4.5-1"
//
// Example: WithInstallerURLFromInstallersJSON("dd-agent-mstesting", "stable", "7.56.0-installer-0.4.5-1")
// will look into "https://s3.amazonaws.com/dd-agent-mstesting/builds/stable/installers_v2.json" for the Datadog Installer
// version "7.56.0-installer-0.4.5-1"
func WithInstallerURLFromInstallersJSON(bucket, channel, version string) Option {
	return func(params *Params) error {
		url, err := installers.GetProductURL(fmt.Sprintf("https://s3.amazonaws.com/%s/builds/%s/installers_v2.json", bucket, channel), "datadog-installer", version, "x86_64")
		if err != nil {
			return err
		}
		params.installerURL = url
		return nil
	}
}

// Install will attempt to install the Datadog Installer on the remote host.
// By default, it will use the installer from the current pipeline.
func (d *DatadogInstaller) Install(opts ...Option) error {
	params := Params{}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return nil
	}
	msiPath := ""
	if localMSIPath, exists := os.LookupEnv("DD_INSTALLER_MSI_URL"); exists {
		// developer provided a local file, put it on the remote host
		msiPath, err = windowsCommon.GetTemporaryFile(d.env.RemoteHost)
		if err != nil {
			return err
		}
		d.env.RemoteHost.CopyFile(localMSIPath, msiPath)
	} else if params.installerURL == "" {
		artifactURL, err := pipeline.GetPipelineArtifact(d.env.Environment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".msi")
		})
		if err != nil {
			return err
		}
		params.installerURL = artifactURL
		// MSI will handle downloading from the URL
		msiPath = params.installerURL
	}
	logPath := d.logPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "install.log")
	}
	msiArgs := ""
	if params.msiArgs != nil {
		msiArgs = strings.Join(params.msiArgs, " ")
	}
	return windowsCommon.InstallMSI(d.env.RemoteHost, msiPath, msiArgs, logPath)
}

// Uninstall will attempt to uninstall the Datadog Installer on the remote host.
func (d *DatadogInstaller) Uninstall() error {
	productCode, err := windowsCommon.GetProductCodeByName(d.env.RemoteHost, "Datadog Installer")
	if err != nil {
		return err
	}

	logPath := d.logPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "uninstall.log")
	}
	return windowsCommon.UninstallMSI(d.env.RemoteHost, productCode, logPath)
}

// GetExperimentDirFor is the path to the experiment symbolic link on disk
func GetExperimentDirFor(packageName string) string {
	return fmt.Sprintf("C:\\ProgramData\\Datadog Installer\\packages\\%s\\experiment", packageName)
}

// GetStableDirFor is the path to the stable symbolic link on disk
func GetStableDirFor(packageName string) string {
	return fmt.Sprintf("C:\\ProgramData\\Datadog Installer\\packages\\%s\\stable", packageName)
}
