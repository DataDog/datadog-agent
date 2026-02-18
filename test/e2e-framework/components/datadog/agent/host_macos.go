// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type agentMacOSManager struct {
	host *remoteComp.Host
}

func newMacOSManager(host *remoteComp.Host) agentOSManager {
	return &agentMacOSManager{host: host}
}

// directInstallCommand expects a locally provided .dmg or .pkg uploaded to the host; it will install it with installer
func (am *agentMacOSManager) directInstallCommand(_ config.Env, _ string, _ agentparams.PackageVersion, _ []string, _ ...pulumi.ResourceOption) (command.Command, error) {
	// Unsupported for now.
	return nil, fmt.Errorf("installing directly from a dmg without the install script requires way too many step that would imply duplicating the install script code in there")
}

// getInstallCommand downloads appropriate pkg and installs it
func (am *agentMacOSManager) getInstallCommand(version agentparams.PackageVersion, apiKey pulumi.StringInput, _ []string) (pulumi.StringOutput, error) {
	// For macOS, use the official install script which supports DD_API_KEY and version envs,
	// mirroring Linux flow but using the macOS path. The script detects OS and uses pkg.
	// If pipeline is specified, we cannot use public script; we assume local package will be provided in that case.

	exports := []string{}
	if version.Major != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", version.Major))
	}
	if version.Minor != "" {
		exports = append(exports, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%s", version.Minor))
	}

	if version.PipelineID != "" {
		exports = append(exports, fmt.Sprintf("DD_REPO_URL=https://dd-agent-macostesting.s3.amazonaws.com/ci/datadog-agent/pipeline-%s-%s", version.PipelineID, am.host.OS.Descriptor().Architecture))
	}

	env := strings.Join(exports, " ")
	// Retry curl few times
	cmd := fmt.Sprintf(`for i in 1 2 3 4 5; do curl -fsSL https://install.datadoghq.com/scripts/install_mac_os.sh -o install-script.sh && break || sleep $((2**$i)); done && for i in 1 2 3; do DD_API_KEY=%%s %%s %[1]s DD_INSTALL_ONLY=true bash install-script.sh && exit 0 || sleep $((2**$i)); done; exit 1`, env)
	// Only the systemdaemon install is supported on macOS, because single user requires to interact with the pop-up.
	pulumiCmdStr := pulumi.Sprintf(cmd, apiKey, pulumi.Sprintf("DD_SYSTEMDAEMON_INSTALL=true DD_SYSTEMDAEMON_USER_GROUP=%s:staff", am.host.Username))
	return pulumiCmdStr, nil
}

func (am *agentMacOSManager) getAgentConfigFolder() string {
	// macOS Agent config default
	return "/opt/datadog-agent/etc"
}

func (am *agentMacOSManager) restartAgentServices(transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	// On macOS, the launchd service is "com.datadoghq.agent"
	cmdName := am.host.Name() + "-restart-agent"
	var cmdArgs command.RunnerCommandArgs = &command.Args{
		Sudo:   true,
		Create: pulumi.String("launchctl kickstart -k system/com.datadoghq.agent"),
	}
	if transform != nil {
		cmdName, cmdArgs = transform(cmdName, cmdArgs)
	}
	return am.host.OS.Runner().Command(cmdName, cmdArgs, opts...)
}

func (am *agentMacOSManager) ensureAgentUninstalled(_ agentparams.PackageVersion, opts ...pulumi.ResourceOption) (command.Command, error) {
	// No-op the install script should support installing again when the agent is already installed
	return am.host.OS.Runner().Command("no-op-uninstall-agent", &command.Args{
		Create: pulumi.String("true"),
	}, opts...)
}
