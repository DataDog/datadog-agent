// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"strings"
)

// DatadogInstallScript represents an interface to the Datadog Install script on the remote host.
type DatadogInstallScript struct {
	env *environments.WindowsHost
}

// NewDatadogInstallScript instantiates a new instance of the Datadog Install Script running on
// a remote Windows host.
func NewDatadogInstallScript(env *environments.WindowsHost) *DatadogInstallScript {
	return &DatadogInstallScript{
		env: env,
	}
}

// Run runs the Datadog Installer install script on the remote host.
func (d *DatadogInstallScript) Run(opts ...Option) (string, error) {
	params := Params{
		extraEnvVars: make(map[string]string),
	}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return "", err
	}
	if params.installerURL == "" {
		artifactURL, err := pipeline.GetPipelineArtifact(d.env.Environment.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
		})
		if err != nil {
			return "", err
		}
		// update URL
		params.installerURL = artifactURL
	}

	// Set the environment variables for the install script
	envVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	for k, v := range params.extraEnvVars {
		envVars[k] = v
	}
	envVars["DD_INSTALLER_URL"] = params.installerURL
	envVars["DD_REMOTE_UPDATES"] = "true"

	cmd := fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
		[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
		iex ((New-Object System.Net.WebClient).DownloadString('https://installtesting.datad0g.com/pipeline-%s/scripts/Install-Datadog.ps1'))`, d.env.Environment.PipelineID())
	return d.env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}
