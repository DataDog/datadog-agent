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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"strings"
)

const (
	AgentPackage string = "agent-package"
)

type datadogInstaller struct {
	binaryPath string
	env        *environments.WindowsHost
}

// NewDatadogInstaller instantiates a new instance of the Datadog Installer running
// on a remote Windows host.
func NewDatadogInstaller(env *environments.WindowsHost) *datadogInstaller {
	return &datadogInstaller{
		binaryPath: "C:\\Program Files\\Datadog\\Datadog Installer\\datadog-installer.exe",
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
	apikey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err == nil {
		envVars["DD_API_KEY"] = apikey
	}
	fmt.Printf("displaying package environment variables")
	for key, value := range envVars {
		fmt.Printf("\t%s: %v\n", key, value)
	}
	return d.execute(fmt.Sprintf("install %s", packageUrl), client.WithEnvVariables(envVars))
}

// Install will attempt to install the Datadog Installer on the remote host.
// By default, it will use the installer from the current pipeline.
func (d *datadogInstaller) Install() error {
	artifactUrl, err := pipeline.GetArtifact(d.env.AwsEnvironment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
		return strings.Contains(artifact, "datadog-installer")
	})
	if err != nil {
		return err
	}
	return windowsCommon.InstallMSI(d.env.RemoteHost, artifactUrl, "", "")
}

// Purge purges the Datadog Installer of all installed packages
func (d *datadogInstaller) Purge() (string, error) {
	return d.execute(fmt.Sprintf("purge"))
}
