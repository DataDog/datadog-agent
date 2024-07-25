// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

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
	// InstallerPath is the path where the Datadog Installer is installed on disk
	InstallerPath string = "C:\\Program Files\\Datadog\\Datadog Installer"
	// InstallerBinaryName is the name of the Datadog Installer binary on disk
	InstallerBinaryName string = "datadog-installer.exe"
	// InstallerServiceName the installer service name
	InstallerServiceName string = "Datadog Installer"
	// InstallerConfigPath is the location of the Datadog Installer's configuration on disk
	InstallerConfigPath string = "C:\\ProgramData\\Datadog\\datadog.yaml"
)

var (
	// InstallerBinaryPath is the path of the Datadog Installer binary on disk
	InstallerBinaryPath = path.Join(InstallerPath, InstallerBinaryName)
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
		binaryPath: path.Join(InstallerPath, InstallerBinaryName),
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

// InstallPackage will attempt to use the Datadog Installer to install the package given in parameter.
// Note that this command is a direct command and won't go through the Daemon.
func (d *DatadogInstaller) InstallPackage(packageName string) (string, error) {
	var packageURL string
	switch packageName {
	case AgentPackage:
		// Note: the URL doesn't matter here, only the "image" name and the version:
		// `agent-package:pipeline-123456`
		// The rest is going to be overridden by the environment variables.
		packageURL = fmt.Sprintf("oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-package:pipeline-%s", d.env.AwsEnvironment.PipelineID())
	default:
		return "", fmt.Errorf("installing package %s is not yet implemented", packageName)
	}

	envVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	return d.execute(fmt.Sprintf("install %s", packageURL), client.WithEnvVariables(envVars))
}

// RemovePackage requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemovePackage(packageName string) (string, error) {
	switch packageName {
	case AgentPackage:
		return d.execute("remove datadog-agent")
	default:
		return "", fmt.Errorf("removing package %s is not yet implemented", packageName)
	}
}

// InstallerParams contains the optional parameters for the Datadog Installer
type InstallerParams struct {
	installerURL string
}

// InstallerOption is an optional function parameter type for the Datadog Installer
type InstallerOption func(*InstallerParams) error

// WithInstallerURL uses a specific URL for the Datadog Installer instead of using the pipeline URL.
func WithInstallerURL(installerURL string) InstallerOption {
	return func(params *InstallerParams) error {
		params.installerURL = installerURL
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
func WithInstallerURLFromInstallersJSON(bucket, channel, version string) InstallerOption {
	return func(params *InstallerParams) error {
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
func (d *DatadogInstaller) Install(opts ...InstallerOption) error {
	params := InstallerParams{}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return nil
	}
	if params.installerURL == "" {
		artifactURL, err := pipeline.GetPipelineArtifact(d.env.AwsEnvironment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer")
		})
		if err != nil {
			return err
		}
		params.installerURL = artifactURL
	}
	logPath := d.logPath
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "install.log")
	}
	return windowsCommon.InstallMSI(d.env.RemoteHost, params.installerURL, "", logPath)
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
