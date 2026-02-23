// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HostAgentOutput struct {
	components.JSONImporter

	Host        remoteComp.HostOutput `json:"host"`
	FIPSEnabled bool                  `json:"fipsEnabled"`
}

// HostAgent is an installer for the Agent on a remote host
type HostAgent struct {
	pulumi.ResourceState
	components.Component

	namer   namer.Namer
	manager agentOSManager

	Host        *remoteComp.Host  `pulumi:"host"`
	FIPSEnabled pulumi.BoolOutput `pulumi:"fipsEnabled"`
}

func (h *HostAgent) Export(ctx *pulumi.Context, out *HostAgentOutput) error {
	return components.Export(ctx, h, out)
}

// NewHostAgent creates a new instance of a on-host Agent
func NewHostAgent(e config.Env, host *remoteComp.Host, options ...agentparams.Option) (*HostAgent, error) {
	hostInstallComp, err := components.NewComponent(e, host.Name(), func(comp *HostAgent) error {
		comp.namer = e.CommonNamer().WithPrefix(comp.Name())
		comp.Host = host
		comp.manager = getOSManager(host)

		params, err := agentparams.NewParams(e, options...)
		if err != nil {
			return err
		}

		comp.FIPSEnabled = pulumi.Bool(e.AgentFIPS()).ToBoolOutput()

		deps := append(params.ResourceOptions, pulumi.Parent(comp))
		err = comp.installAgent(e, params, deps...)
		if err != nil {
			return err
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return hostInstallComp, nil
}

func (h *HostAgent) installScriptInstallation(env config.Env, params *agentparams.Params, baseOpts ...pulumi.ResourceOption) (command.Command, error) {
	installCmdStr, err := h.manager.getInstallCommand(params.Version, env.AgentAPIKey(), params.AdditionalInstallParameters)
	if err != nil {
		return nil, err
	}

	uninstallCmd, err := h.manager.ensureAgentUninstalled(params.Version, baseOpts...)
	if err != nil {
		return nil, err
	}
	baseOpts = utils.MergeOptions(baseOpts, utils.PulumiDependsOn(uninstallCmd))

	installCmd, err := h.Host.OS.Runner().Command(
		h.namer.ResourceName("install-agent"),
		&command.Args{
			Create: installCmdStr,
		}, baseOpts...)
	if err != nil {
		return nil, err
	}
	return installCmd, nil
}

func (h *HostAgent) directInstallInstallation(env config.Env, params *agentparams.Params, baseOpts ...pulumi.ResourceOption) (command.Command, error) {
	packagePath, err := GetPackagePath(params.Version.LocalPath, h.Host.OS.Descriptor().Flavor, params.Version.Flavor, h.Host.OS.Descriptor().Architecture, env.PipelineID())
	if err != nil {
		return nil, err
	}

	env.Ctx().Log.Info(fmt.Sprintf("Found local package to install %s", packagePath), nil)
	uploadCmd, err := h.Host.OS.FileManager().CopyToRemoteFile("copy-agent-package", pulumi.String(packagePath), pulumi.String("./"), baseOpts...)
	if err != nil {
		return nil, err
	}

	installCmd, err := h.manager.directInstallCommand(env, path.Base(packagePath), params.Version, params.AdditionalInstallParameters, utils.MergeOptions(baseOpts, utils.PulumiDependsOn(uploadCmd))...)
	if err != nil {
		return nil, err
	}
	return installCmd, nil
}

func (h *HostAgent) installAgent(env config.Env, params *agentparams.Params, baseOpts ...pulumi.ResourceOption) error {
	var installCmd pulumi.Resource
	var err error
	if params.Version.LocalPath != "" {
		installCmd, err = h.directInstallInstallation(env, params, baseOpts...)
		if err != nil {
			return err
		}
	} else {
		installCmd, err = h.installScriptInstallation(env, params, baseOpts...)
		if err != nil {
			return err
		}
	}

	afterInstallOpts := utils.MergeOptions(baseOpts, utils.PulumiDependsOn(installCmd))
	configFiles := make(map[string]pulumi.StringInput)

	// Update core Agent
	_, content, err := h.updateCoreAgentConfig(env, "datadog.yaml", pulumi.String(params.AgentConfig), params.ExtraAgentConfig, params.SkipAPIKeyInConfig, afterInstallOpts...)
	if err != nil {
		return err
	}
	configFiles["datadog.yaml"] = content

	// Update other Agents
	for _, input := range []struct{ path, content string }{
		{"system-probe.yaml", params.SystemProbeConfig},
		{"security-agent.yaml", params.SecurityAgentConfig},
	} {
		contentPulumiStr := pulumi.String(input.content)
		_, err := h.updateConfig(input.path, contentPulumiStr, afterInstallOpts...)
		if err != nil {
			return err
		}

		configFiles[input.path] = contentPulumiStr
	}

	_, intgHash, err := h.installIntegrationConfigsAndFiles(params.Integrations, params.Files, afterInstallOpts...)
	if err != nil {
		return err
	}

	// Restart the agent when the HostInstall itself is done, which is normally when all children are done
	// Behind the scene `DependOn(h)` is transformed into `DependOn(<children>)`, the ComponentResource is skipped in the process.
	// With resources, Pulumi works in the following order:
	// Create -> Replace -> Delete.
	// The `DependOn` order is evaluated separately for each of these phases.
	// Thus, when an integration is deleted, the `Create` of `restartAgentServices` is done as there's no other `Create` from other resources to wait for.
	// Then the `Delete` of `restartAgentServices` is done, which is not waiting for the `Delete` of the integration as the dependency on `Delete` is in reverse order.
	//
	// For this reason we have another `restartAgentServices` in `installIntegrationConfigsAndFiles` that is triggered when an integration is deleted.
	_, err = h.manager.restartAgentServices(
		// Transformer used to add triggers to the restart command
		func(name string, cmdArgs command.RunnerCommandArgs) (string, command.RunnerCommandArgs) {
			args := *cmdArgs.Arguments()
			args.Triggers = pulumi.Array{configFiles["datadog.yaml"], configFiles["system-probe.yaml"], configFiles["security-agent.yaml"], pulumi.String(intgHash), pulumi.String(params.Version.Major), pulumi.String(params.Version.Minor), pulumi.String(params.Version.PipelineID), pulumi.String(params.Version.Flavor), pulumi.String(params.Version.Channel)}
			return name, &args
		},
		utils.PulumiDependsOn(h),
	)
	return err
}

func (h *HostAgent) updateCoreAgentConfig(
	env config.Env,
	configPath string,
	configContent pulumi.StringInput,
	extraAgentConfig []pulumi.StringInput,
	skipAPIKeyInConfig bool,
	opts ...pulumi.ResourceOption,
) (pulumi.Resource, pulumi.StringInput, error) {
	var convertedArgs []interface{}
	convertedArgs = append(convertedArgs, configContent)
	convertedArgs = append(convertedArgs, env.AgentAPIKey())
	for _, c := range extraAgentConfig {
		convertedArgs = append(convertedArgs, c)
	}

	mergedConfig := pulumi.All(convertedArgs...).ApplyT(func(args []interface{}) (string, error) {
		baseConfig := args[0].(string)
		apiKey := args[1].(string)
		extraConfigs := make([]string, 0, len(args))

		if len(args) > 2 {
			for _, extraConfig := range args[2:] {
				extraConfigs = append(extraConfigs, extraConfig.(string))
			}
		}

		if !skipAPIKeyInConfig {
			extraConfigs = append(extraConfigs, fmt.Sprintf("api_key: %s", apiKey))
		}

		var err error
		for _, extraConfig := range extraConfigs {
			// recursively merge the extra config into the base config
			baseConfig, err = utils.MergeYAMLWithSlices(baseConfig, extraConfig)
			if err != nil {
				return "", err
			}
		}

		return baseConfig, err
	}).(pulumi.StringOutput)

	cmd, err := h.updateConfig(configPath, mergedConfig, opts...)
	return cmd, configContent, err
}

func (h *HostAgent) updateConfig(
	configPath string,
	configContent pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (pulumi.Resource, error) {
	var err error

	configFullPath := path.Join(h.manager.getAgentConfigFolder(), configPath)

	copyCmd, err := h.Host.OS.FileManager().CopyInlineFile(configContent, configFullPath, opts...)
	if err != nil {
		return nil, err
	}

	return copyCmd, nil
}

func (h *HostAgent) installIntegrationConfigsAndFiles(
	integrations map[string]*agentparams.FileDefinition,
	files map[string]*agentparams.FileDefinition,
	opts ...pulumi.ResourceOption,
) ([]pulumi.Resource, string, error) {
	allCommands := make([]pulumi.Resource, 0)
	var parts []string

	// Build hash beforehand as we need to pass it to the restart command
	for filePath, fileDef := range integrations {
		parts = append(parts, filePath, fileDef.Content)
	}
	for fullPath, fileDef := range files {
		parts = append(parts, fullPath, fileDef.Content)
	}
	hash := utils.StrHash(parts...)

	// Restart the agent when an integration is removed
	// See longer comment in `installAgent` for more details
	restartCmd, err := h.manager.restartAgentServices(
		// Use a transformer to inject triggers on intg hash and move `restart` command from `Create` to `Delete`
		// so that it's run after the `Delete` commands of the integrations.
		func(name string, cmdArgs command.RunnerCommandArgs) (string, command.RunnerCommandArgs) {
			args := *cmdArgs.Arguments()
			args.Triggers = pulumi.Array{pulumi.String(hash)}
			args.Delete = args.Create
			args.Create = nil
			return name + "-on-intg-removal", &args
		})
	if err != nil {
		return nil, "", err
	}

	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(restartCmd))

	// filePath is absolute path from params.WithFile but relative from params.WithIntegration
	for filePath, fileDef := range integrations {
		configFolder := h.manager.getAgentConfigFolder()
		fullPath := path.Join(configFolder, filePath)

		file, err := h.writeFileDefinition(fullPath, fileDef.Content, fileDef.UseSudo, fileDef.Permissions, opts...)
		if err != nil {
			return nil, "", err
		}
		allCommands = append(allCommands, file)
	}

	for fullPath, fileDef := range files {
		if !h.Host.OS.FileManager().IsPathAbsolute(fullPath) {
			return nil, "", fmt.Errorf("failed to write file: \"%s\" is not an absolute filepath", fullPath)
		}

		cmd, err := h.writeFileDefinition(fullPath, fileDef.Content, fileDef.UseSudo, fileDef.Permissions, opts...)
		if err != nil {
			return nil, "", err
		}
		allCommands = append(allCommands, cmd)
	}

	return allCommands, hash, nil
}

func (h *HostAgent) writeFileDefinition(
	fullPath string,
	content string,
	useSudo bool,
	perms option.Option[perms.FilePermissions],
	opts ...pulumi.ResourceOption,
) (pulumi.Resource, error) {
	// create directory, if it does not exist
	dirCommand, err := h.Host.OS.FileManager().CreateDirectoryForFile(fullPath, useSudo, opts...)
	if err != nil {
		return nil, err
	}

	copyCmd, err := h.Host.OS.FileManager().CopyInlineFile(pulumi.String(content), fullPath, utils.MergeOptions(opts, utils.PulumiDependsOn(dirCommand))...)
	if err != nil {
		return nil, err
	}

	// Set permissions if any
	if value, found := perms.Get(); found {
		if cmd := value.SetupPermissionsCommand(fullPath); cmd != "" {
			return h.Host.OS.Runner().Command(
				h.namer.ResourceName("set-permissions-"+fullPath, utils.StrHash(cmd)),
				&command.Args{
					Create: pulumi.String(cmd),
					Delete: pulumi.String(value.ResetPermissionsCommand(fullPath)),
					Update: pulumi.String(cmd),
				},
				utils.PulumiDependsOn(copyCmd))
		}
	}

	return copyCmd, nil
}
