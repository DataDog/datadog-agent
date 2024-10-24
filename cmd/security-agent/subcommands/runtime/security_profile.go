// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

type securityProfileCliParams struct {
	*command.GlobalParams

	includeCache bool
	file         string
	imageName    string
	imageTag     string
}

func securityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	securityProfileCmd := &cobra.Command{
		Use:   "security-profile",
		Short: "security profile commands",
	}

	securityProfileCmd.AddCommand(showSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(listSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(saveSecurityProfileCommands(globalParams)...)

	return []*cobra.Command{securityProfileCmd}
}

func showSecurityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileCliParams{
		GlobalParams: globalParams,
	}

	securityProfileShowCmd := &cobra.Command{
		Use:   "show",
		Short: "dump the content of a security-profile file",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(showSecurityProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	securityProfileShowCmd.Flags().StringVar(
		&cliParams.file,
		"input",
		"",
		"path to the security-profile file",
	)

	return []*cobra.Command{securityProfileShowCmd}
}

func showSecurityProfile(_ log.Component, _ config.Component, _ secrets.Component, args *securityProfileCliParams) error {
	pp, err := profile.LoadProtoFromFile(args.file)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(pp, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(b))

	return nil
}

func listSecurityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileCliParams{
		GlobalParams: globalParams,
	}

	securityProfileListCmd := &cobra.Command{
		Use:   "list",
		Short: "get the list of active security profiles",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(listSecurityProfiles,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	securityProfileListCmd.Flags().BoolVar(
		&cliParams.includeCache,
		"include-cache",
		false,
		"defines if the profiles in the Security Profile manager LRU cache should be returned",
	)

	return []*cobra.Command{securityProfileListCmd}
}

func listSecurityProfiles(_ log.Component, _ config.Component, _ secrets.Component, args *securityProfileCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.ListSecurityProfiles(args.includeCache)
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("security profile list request failed: %s", output.Error)
	}

	if len(output.Profiles) > 0 {
		fmt.Println("security profiles:")
		for _, d := range output.Profiles {
			printSecurityProfileMessage(d)
		}
	} else {
		fmt.Println("no security profile found")
	}

	return nil
}

func printActivityTreeStats(prefix string, msg *api.ActivityTreeStatsMessage) {
	fmt.Printf("%s  activity_tree_stats:\n", prefix)
	fmt.Printf("%s    approximate_size: %v\n", prefix, msg.GetApproximateSize())
	fmt.Printf("%s    process_nodes_count: %v\n", prefix, msg.GetProcessNodesCount())
	fmt.Printf("%s    file_nodes_count: %v\n", prefix, msg.GetFileNodesCount())
	fmt.Printf("%s    dns_nodes_count: %v\n", prefix, msg.GetDNSNodesCount())
	fmt.Printf("%s    socket_nodes_count: %v\n", prefix, msg.GetSocketNodesCount())
}

func printSecurityProfileMessage(msg *api.SecurityProfileMessage) {
	timeResolver, err := ktime.NewResolver()
	if err != nil {
		fmt.Printf("can't get new time resolver: %v\n", err)
		return
	}

	prefix := "  "
	fmt.Printf("%s## NAME: %s ##\n", prefix, msg.GetMetadata().GetName())
	fmt.Printf("%s  workload_selector:\n", prefix)
	fmt.Printf("%s    image_name: %v\n", prefix, msg.GetSelector().GetName())
	fmt.Printf("%s    image_tag: %v\n", prefix, msg.GetSelector().GetTag())
	fmt.Printf("%s  kernel_space:\n", prefix)
	fmt.Printf("%s    loaded: %v\n", prefix, msg.GetLoadedInKernel())
	if msg.GetLoadedInKernel() {
		fmt.Printf("%s    loaded_at: %v\n", prefix, msg.GetLoadedInKernelTimestamp())
		fmt.Printf("%s    cookie: %v - 0x%x\n", prefix, msg.GetProfileCookie(), msg.GetProfileCookie())
	}
	fmt.Printf("%s  event_types: %v\n", prefix, msg.GetEventTypes())
	fmt.Printf("%s  global_state: %v\n", prefix, msg.GetProfileGlobalState())
	fmt.Printf("%s  Versions:\n", prefix)
	for imageTag, ctx := range msg.GetProfileContexts() {
		fmt.Printf("%s  - %s:\n", prefix, imageTag)
		fmt.Printf("%s    tags: %v\n", prefix, ctx.GetTags())
		fmt.Printf("%s    first seen: %v\n", prefix, timeResolver.ResolveMonotonicTimestamp(ctx.GetFirstSeen()))
		fmt.Printf("%s    last seen: %v\n", prefix, timeResolver.ResolveMonotonicTimestamp(ctx.GetLastSeen()))
		for et, state := range ctx.GetEventTypeState() {
			fmt.Printf("%s    . %s: %s\n", prefix, et, state.GetEventProfileState())
			fmt.Printf("%s      last anomaly: %v\n", prefix, timeResolver.ResolveMonotonicTimestamp(state.GetLastAnomalyNano()))
		}
	}
	if len(msg.GetInstances()) > 0 {
		fmt.Printf("%s  instances:\n", prefix)
		for _, inst := range msg.GetInstances() {
			fmt.Printf("%s    . container_id: %s\n", prefix, inst.GetContainerID())
			fmt.Printf("%s      tags: %v\n", prefix, inst.GetTags())
		}
	}
	printActivityTreeStats(prefix, msg.GetStats())
}

func saveSecurityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileCliParams{
		GlobalParams: globalParams,
	}

	securityProfileSaveCmd := &cobra.Command{
		Use:   "save",
		Short: "saves the requested security profile to disk",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(saveSecurityProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	securityProfileSaveCmd.Flags().StringVar(
		&cliParams.imageName,
		"name",
		"",
		"image name of the workload selector used to lookup the profile",
	)
	_ = securityProfileSaveCmd.MarkFlagRequired("name")
	securityProfileSaveCmd.Flags().StringVar(
		&cliParams.imageTag,
		"tag",
		"",
		"image tag of the workload selector used to lookup the profile",
	)
	_ = securityProfileSaveCmd.MarkFlagRequired("tag")

	return []*cobra.Command{securityProfileSaveCmd}
}

func saveSecurityProfile(_ log.Component, _ config.Component, _ secrets.Component, args *securityProfileCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.SaveSecurityProfile(args.imageName, args.imageTag)
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.GetError()) > 0 {
		return fmt.Errorf("security profile save request failed: %s", output.Error)
	}

	if len(output.GetFile()) > 0 {
		fmt.Printf("security profile successfully saved at: %v\n", output.GetFile())
	} else {
		fmt.Println("security profile not found")
	}

	return nil
}
