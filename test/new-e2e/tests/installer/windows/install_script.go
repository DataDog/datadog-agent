// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/test-infra-definitions/components/os"
	goos "os"
)

// RunInstallScript runs the Datadog Installer install script on the remote host.
func RunInstallScript(env *environments.WindowsHost, extraEnvVars map[string]string) (string, error) {
	// Set the environment variables for the install script
	envVars := installer.InstallScriptEnv(os.AMD64Arch)
	for k, v := range extraEnvVars {
		envVars[k] = v
	}
	cmd := fmt.Sprintf(`Set-ExecutionPolicy Bypass -Scope Process -Force;
		[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072;
		iex ((New-Object System.Net.WebClient).DownloadString('https://installtesting.datad0g.com/%s/scripts/Install-Datadog.ps1'))`, goos.Getenv("CI_COMMIT_SHA"))
	return env.RemoteHost.Execute(cmd, client.WithEnvVariables(envVars))
}
