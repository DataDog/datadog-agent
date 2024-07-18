// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"path"
	"strings"
)

const (
	AgentPackage        string = "agent-package"
	InstallerPath       string = "C:\\Program Files\\Datadog\\Datadog Installer"
	InstallerBinaryName string = "datadog-installer.exe"
)

type datadogInstaller struct {
	binaryPath string
	env        *environments.WindowsHost
}

// NewDatadogInstaller instantiates a new instance of the Datadog Installer running
// on a remote Windows host.
func NewDatadogInstaller(env *environments.WindowsHost) *datadogInstaller {
	return &datadogInstaller{
		binaryPath: path.Join(InstallerPath, InstallerBinaryName),
		env:        env,
	}
}

func (d *datadogInstaller) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Version returns the version of the Datadog Installer on the host.
func (d *datadogInstaller) Version() (string, error) {
	return d.execute("version")
}

// InstallPackage will attempt to use the Datadog Installer to install the package given in parameter.
// Note that this command is a direct command and won't go through the Daemon.
func (d *datadogInstaller) InstallPackage(packageName string) (string, error) {
	var packageUrl string
	switch packageName {
	case AgentPackage:
		// Note: the URL doesn't matter here, only the "image" name and the version:
		// `agent-package:pipeline-123456`
		// The rest is going to be overridden by the environment variables.
		packageUrl = fmt.Sprintf("oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-package:pipeline-%s", d.env.AwsEnvironment.PipelineID())
	default:
		return "", fmt.Errorf("installing package %s is not yet implemented", packageName)
	}

	envVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	fmt.Printf("displaying package environment variables\n")
	for key, value := range envVars {
		fmt.Printf("\t%s: %v\n", key, value)
	}

	// Don't display the API key in the output...
	apikey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err == nil {
		envVars["DD_API_KEY"] = apikey
	}
	return d.execute(fmt.Sprintf("install %s", packageUrl), client.WithEnvVariables(envVars))
}

// RemovePackage requests that the Datadog Installer removes a package on the remote host.
func (d *datadogInstaller) RemovePackage(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove %s", packageName))
}

type installerParams struct {
	installerUrl string
}

type installerOption func(*installerParams) error

// WithInstallerUrl uses a specific URL for the Datadog Installer instead of using the pipeline URL.
func WithInstallerUrl(installerUrl string) installerOption {
	return func(params *installerParams) error {
		params.installerUrl = installerUrl
		return nil
	}
}

// WithInstallerUrlFromInstallersJson uses a specific URL for the Datadog Installer from an installers_v2.json
// file.
// bucket: The S3 bucket to look for the installers_v2.json file, i.e. "dd-agent-mstesting"
// channel: The channel in the bucket, i.e. "stable"
// version: The artifact version to retrieve, i.e. "7.56.0-installer-0.4.5-1"
//
// Example: WithInstallerUrlFromInstallersJson("dd-agent-mstesting", "stable", "7.56.0-installer-0.4.5-1")
// will look into "https://s3.amazonaws.com/dd-agent-mstesting/builds/stable/installers_v2.json" for the Datadog Installer
// version "7.56.0-installer-0.4.5-1"
func WithInstallerUrlFromInstallersJson(bucket, channel, version string) installerOption {
	return func(params *installerParams) error {
		url, err := installers.GetProductURL(fmt.Sprintf("https://s3.amazonaws.com/%s/builds/%s/installers_v2.json", bucket, channel), "datadog-installer", version, "x86_64")
		if err != nil {
			return err
		}
		params.installerUrl = url
		return nil
	}
}

// Install will attempt to install the Datadog Installer on the remote host.
// By default, it will use the installer from the current pipeline.
func (d *datadogInstaller) Install(opts ...installerOption) error {
	params := installerParams{}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return nil
	}
	if params.installerUrl == "" {
		artifactUrl, err := pipeline.GetPipelineArtifact(d.env.AwsEnvironment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer")
		})
		if err != nil {
			return err
		}
		params.installerUrl = artifactUrl
	}
	return windowsCommon.InstallMSI(d.env.RemoteHost, params.installerUrl, "", "")
}
