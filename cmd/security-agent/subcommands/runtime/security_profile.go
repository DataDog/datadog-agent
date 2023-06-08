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
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

	securityProfileCmd.AddCommand(securityProfileShowCommands(globalParams)...)
	securityProfileCmd.AddCommand(listSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(saveSecurityProfileCommands(globalParams)...)

	return []*cobra.Command{securityProfileCmd}
}

func securityProfileShowCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileCliParams{
		GlobalParams: globalParams,
	}

	securityProfileShowCmd := &cobra.Command{
		Use:   "show",
		Short: "dump the content of a security-profile file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(showSecurityProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	securityProfileShowCmd.Flags().StringVar(
		&cliParams.file,
		flags.SecurityProfileInput,
		"",
		"path to the activity dump file",
	)

	return []*cobra.Command{securityProfileShowCmd}
}

func showSecurityProfile(log log.Component, config config.Component, args *securityProfileCliParams) error {
	prof, err := profile.LoadProfileFromFile(args.file)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(prof, "", "  ")
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(listSecurityProfiles,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	securityProfileListCmd.Flags().BoolVar(
		&cliParams.includeCache,
		flags.IncludeCache,
		false,
		"defines if the profiles in the Security Profile manager LRU cache should be returned",
	)

	return []*cobra.Command{securityProfileListCmd}
}

func listSecurityProfiles(log log.Component, config config.Component, args *securityProfileCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.ListSecurityProfiles(args.includeCache)
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
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
	prefix := "  "
	fmt.Printf("%s- name: %s\n", prefix, msg.GetMetadata().GetName())
	fmt.Printf("%s  workload_selector:\n", prefix)
	fmt.Printf("%s    image_name: %v\n", prefix, msg.GetSelector().GetName())
	fmt.Printf("%s    image_tag: %v\n", prefix, msg.GetSelector().GetTag())
	fmt.Printf("%s  version: %v\n", prefix, msg.GetVersion())
	fmt.Printf("%s  status: %v\n", prefix, msg.GetStatus())
	fmt.Printf("%s  kernel_space:\n", prefix)
	fmt.Printf("%s    loaded: %v\n", prefix, msg.GetLoadedInKernel())
	if msg.GetLoadedInKernel() {
		fmt.Printf("%s    loaded_at: %v\n", prefix, msg.GetLoadedInKernelTimestamp())
		fmt.Printf("%s    cookie: %v - 0x%x\n", prefix, msg.GetProfileCookie(), msg.GetProfileCookie())
	}
	fmt.Printf("%s  anomaly_detection_events: %v\n", prefix, msg.GetAnomalyDetectionEvents())
	if len(msg.GetLastAnomalies()) > 0 {
		fmt.Printf("%s  last_anomalies:\n", prefix)
		for _, ano := range msg.GetLastAnomalies() {
			fmt.Printf("%s    - event_type: %s\n", prefix, ano.GetEventType())
			fmt.Printf("%s      timestamp: %s\n", prefix, ano.GetTimestamp())
			fmt.Printf("%s      is_stable: %v\n", prefix, ano.GetIsStableEventType())
		}
	}
	if len(msg.GetInstances()) > 0 {
		fmt.Printf("%s  instances:\n", prefix)
		for _, inst := range msg.GetInstances() {
			fmt.Printf("%s    - container_id: %s\n", prefix, inst.GetContainerID())
			fmt.Printf("%s      tags: %v\n", prefix, inst.GetTags())
		}
	}
	fmt.Printf("%s  tags: %v\n", prefix, msg.GetTags())
	printActivityTreeStats(prefix, msg.GetStats())
}

func saveSecurityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileCliParams{
		GlobalParams: globalParams,
	}

	securityProfileSaveCmd := &cobra.Command{
		Use:   "save",
		Short: "saves the requested security profile to disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(saveSecurityProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	securityProfileSaveCmd.Flags().StringVar(
		&cliParams.imageName,
		flags.ImageName,
		"",
		"image name of the workload selector used to lookup the profile",
	)
	_ = securityProfileSaveCmd.MarkFlagRequired(flags.ImageName)
	securityProfileSaveCmd.Flags().StringVar(
		&cliParams.imageTag,
		flags.ImageTag,
		"",
		"image tag of the workload selector used to lookup the profile",
	)
	_ = securityProfileSaveCmd.MarkFlagRequired(flags.ImageTag)

	return []*cobra.Command{securityProfileSaveCmd}
}

func saveSecurityProfile(log log.Component, config config.Component, args *securityProfileCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.SaveSecurityProfile(args.imageName, args.imageTag)
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
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
