// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// internal interface to be able to provide the different OS-specific commands
type agentOSManager interface {
	directInstallCommand(env config.Env, packagePath string, version agentparams.PackageVersion, additionalInstallParameters []string, opts ...pulumi.ResourceOption) (command.Command, error)
	getInstallCommand(version agentparams.PackageVersion, apiKey pulumi.StringInput, additionalInstallParameters []string) (pulumi.StringOutput, error)
	getAgentConfigFolder() string
	restartAgentServices(transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error)
	ensureAgentUninstalled(version agentparams.PackageVersion, opts ...pulumi.ResourceOption) (command.Command, error)
}

func getOSManager(host *remoteComp.Host) agentOSManager {
	switch host.OS.Descriptor().Family() {
	case os.LinuxFamily:
		return newLinuxManager(host)
	case os.WindowsFamily:
		return newWindowsManager(host)
	case os.MacOSFamily:
		return newMacOSManager(host)
	case os.UnknownFamily:
		fallthrough
	default:
		panic(fmt.Sprintf("unsupported OS: %v", host.OS.Descriptor().Family()))
	}
}
