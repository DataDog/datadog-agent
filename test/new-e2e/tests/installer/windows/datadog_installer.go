// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer_windows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"strings"
)

type datadogInstaller struct {
	binaryPath string
	host       *components.RemoteHost
}

func NewInstaller(host *components.RemoteHost) *datadogInstaller {
	return &datadogInstaller{
		binaryPath: "C:\\Program Files\\Datadog\\Datadog Installer\\datadog-installer.exe",
		host:       host,
	}
}

func (d *datadogInstaller) execute(cmd string, options ...client.ExecuteOption) string {
	return strings.TrimSpace(d.host.MustExecute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...))
}

// Version returns the version of the installer on the host.
func (d *datadogInstaller) Version() string {
	return d.execute("version")
}

func (d *datadogInstaller) Install(params string) string {
	// install oci://registry.ddbuild.io/ci/remote-updates/datadog-agent:pipeline-34898077
	defaultEnvVars := installer.InstallScriptEnv(e2eos.AMD64Arch)
	fmt.Printf("env vars: %v\n", defaultEnvVars)
	return d.execute(fmt.Sprintf("install %s", params), client.WithEnvVariables(defaultEnvVars))
}
