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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type agentLinuxManager struct {
	targetOS os.OS
}

func newLinuxManager(host *remoteComp.Host) agentOSManager {
	return &agentLinuxManager{targetOS: host.OS}
}

func (am *agentLinuxManager) directInstallCommand(_ config.Env, packagePath string, _ agentparams.PackageVersion, _ []string, opts ...pulumi.ResourceOption) (command.Command, error) {
	return am.targetOS.PackageManager().Ensure("./"+packagePath, nil, "", os.AllowUnsignedPackages(true), os.WithPulumiResourceOptions(opts...))
}

func (am *agentLinuxManager) getInstallCommand(version agentparams.PackageVersion, apiKey pulumi.StringInput, _ []string) (pulumi.StringOutput, error) {
	var commandLine string
	testEnvVars := []string{}

	if version.PipelineID != "" {
		testEnvVars = append(testEnvVars, fmt.Sprintf("TESTING_APT_URL=apttesting.datad0g.com/datadog-agent/pipeline-%v-a%v", version.PipelineID, version.Major))
		// apt testing repo
		// TESTING_APT_REPO_VERSION="pipeline-xxxxx-a7 7"
		testEnvVars = append(testEnvVars, fmt.Sprintf(`TESTING_APT_REPO_VERSION="stable-%[1]s %[2]v"`, am.targetOS.Descriptor().Architecture, version.Major))
		testEnvVars = append(testEnvVars, "TESTING_YUM_URL=yumtesting.datad0g.com")
		// yum testing repo
		// TESTING_YUM_VERSION_PATH="testing/pipeline-xxxxx-a7/7"
		testEnvVars = append(testEnvVars, fmt.Sprintf("TESTING_YUM_VERSION_PATH=testing/pipeline-%[1]v-a%[2]v/%[2]v", version.PipelineID, version.Major))
		// target testing keys
		testEnvVars = append(testEnvVars, fmt.Sprintf("TESTING_KEYS_URL=apttesting.datad0g.com/test-keys"))
	} else {
		testEnvVars = append(testEnvVars, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%v", version.Major))

		if version.Minor != "" {
			testEnvVars = append(testEnvVars, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%v", version.Minor))
		}

		if version.Channel != "" && version.Channel != agentparams.StableChannel {
			testEnvVars = append(testEnvVars, "REPO_URL=datad0g.com")
			testEnvVars = append(testEnvVars, fmt.Sprintf("DD_AGENT_DIST_CHANNEL=%s", version.Channel))
		}
	}

	if version.Flavor != "" {
		testEnvVars = append(testEnvVars, fmt.Sprintf("DD_AGENT_FLAVOR=%s", version.Flavor))
	}

	commandLine = strings.Join(testEnvVars, " ")

	commandLine = fmt.Sprintf(
		`for i in 1 2 3 4 5; do curl -fsSL https://s3.amazonaws.com/dd-agent/scripts/%v -o install-script.sh && break || sleep $((2**$i)); done &&  for i in 1 2 3; do DD_API_KEY=%%s %v DD_INSTALL_ONLY=true bash install-script.sh  && exit 0 || sleep $((2**$i)); done; exit 1`,
		fmt.Sprintf("install_script_agent%s.sh", version.Major),
		commandLine)
	return pulumi.Sprintf(commandLine, apiKey), nil
}

func (am *agentLinuxManager) getAgentConfigFolder() string {
	return "/etc/datadog-agent"
}

func (am *agentLinuxManager) restartAgentServices(transform command.Transformer, opts ...pulumi.ResourceOption) (command.Command, error) {
	return am.targetOS.ServiceManger().EnsureRestarted("datadog-agent", transform, opts...)
}

func (am *agentLinuxManager) ensureAgentUninstalled(version agentparams.PackageVersion, opts ...pulumi.ResourceOption) (command.Command, error) {
	uninstallCmd, err := am.targetOS.PackageManager().EnsureUninstalled(version.Flavor, func(name string, cmdArgs command.RunnerCommandArgs) (string, command.RunnerCommandArgs) {
		args := *cmdArgs.Arguments()
		args.Triggers = pulumi.Array{
			pulumi.String(version.Major),
			pulumi.String(version.Minor),
			pulumi.String(version.PipelineID),
			pulumi.String(version.Flavor),
			pulumi.String(version.Channel),
		}
		args.Update = nil
		return name, &args
	}, version.Flavor, os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}
	return uninstallCmd, nil
}
