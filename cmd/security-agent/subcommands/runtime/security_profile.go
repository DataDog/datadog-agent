// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	timeResolver "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/wconfig"
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

	securityProfileCmd.AddCommand(showSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(listSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(saveSecurityProfileCommands(globalParams)...)
	securityProfileCmd.AddCommand(securityProfileToWorloadPolicyCommands(globalParams)...)

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
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
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
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
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
	timeResolver, err := timeResolver.NewResolver()
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
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
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

type securityProfileToWorloadPolicyCliParams struct {
	*command.GlobalParams

	input     string
	output    string
	kill      bool
	allowlist bool
	lineage   bool
	service   string
	imageName string
	imageTag  string
	fim       bool
}

func securityProfileToWorloadPolicyCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &securityProfileToWorloadPolicyCliParams{
		GlobalParams: globalParams,
	}

	securityProfileWorkloadPolicyCmd := &cobra.Command{
		Use:   "workload-policy",
		Short: "convert a security-profile to a workload policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(securityProfileToWorkloadPolicy,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	securityProfileWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.input,
		"input",
		"",
		"path to the security-profile file",
	)

	securityProfileWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.output,
		"output",
		"",
		"path to the generated workload policy file",
	)

	securityProfileWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.kill,
		"kill",
		false,
		"generate kill action with the workload policy",
	)

	securityProfileWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.fim,
		"fim",
		false,
		"generate fim rules with the workload policy",
	)

	securityProfileWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.allowlist,
		"allowlist",
		false,
		"generate allow list rules",
	)

	securityProfileWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.lineage,
		"lineage",
		false,
		"generate lineage rules",
	)

	securityProfileWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.service,
		"service",
		"",
		"apply on specified service",
	)

	securityProfileWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.imageTag,
		"image-tag",
		"",
		"apply on specified image tag",
	)

	securityProfileWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.imageName,
		"image-name",
		"",
		"apply on specified image name",
	)

	return []*cobra.Command{securityProfileWorkloadPolicyCmd}
}

func securityProfileToWorkloadPolicy(_ log.Component, _ config.Component, _ secrets.Component, args *securityProfileToWorloadPolicyCliParams) error {

	pps, err := profile.LoadProtoFromFiles(args.input)
	if err != nil {
		return err
	}
	var mergedRules []*rules.RuleDefinition
	switch pps.(type) {
	case *proto.SecurityProfile:
		rules, err := generateRules(pps, args)
		if err != nil {
			return err
		}
		mergedRules = rules
	case []*proto.SecurityProfile: // the provided path is a directory containing several profiles

		for _, pp := range pps.([]*proto.SecurityProfile) {

			rules, err := generateRules(pp, args)
			if err != nil {
				return err
			}
			mergedRules = append(mergedRules, rules...)

		}
		mergedRules = factorise(mergedRules, args)

	}

	wp := wconfig.WorkloadPolicy{
		ID:   "workload",
		Name: "workload",
		Kind: "secl",
		SECLPolicy: wconfig.SECLPolicy{
			Rules: mergedRules,
		},
	}

	b, err := yaml.Marshal(wp)
	if err != nil {
		return err
	}

	output := os.Stdout
	if args.output != "" && args.output != "-" {
		output, err = os.Create(args.output)
		if err != nil {
			return err
		}
		defer output.Close()
	}

	fmt.Fprint(output, string(b))

	return nil
}

func generateRules(pps interface{}, args *securityProfileToWorloadPolicyCliParams) ([]*rules.RuleDefinition, error) {
	sp := profile.NewSecurityProfile(model.WorkloadSelector{}, nil, nil)
	sp.LoadFromProto(pps.(*proto.SecurityProfile), profile.LoadOpts{})

	opts := profile.SECLRuleOpts{
		EnableKill: args.kill,
		AllowList:  args.allowlist,
		Lineage:    args.lineage,
		Service:    args.service,
		ImageName:  args.imageName,
		ImageTag:   args.imageTag,
		FIM:        args.fim,
	}

	rules, err := sp.ToSECLRules(opts)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// factorise function to combine rules with the same fields, operations, and paths
func factorise(ruleset []*rules.RuleDefinition, args *securityProfileToWorloadPolicyCliParams) []*rules.RuleDefinition {
	// Map to store combined paths by their field + operation keys
	expressionMap := make(map[string][]string)

	for _, rule := range ruleset {
		// Extract the field and operation as the key
		key := extractFieldAndOperation(rule.Expression)
		// Extract the paths from the expression
		paths := extractPaths(rule.Expression)

		// Add the paths to the corresponding key in the map
		expressionMap[key] = append(expressionMap[key], paths...)
	}

	// Clear the original rules slice
	var res []*rules.RuleDefinition

	// Rebuild the rules with combined paths
	for key, paths := range expressionMap {
		var combinedExpression string

		if strings.HasPrefix(key, "!") { // if it's lineage, just append it to the list of rules
			combinedExpression = key
		} else {

			// Remove duplicates and format the paths list
			uniquePaths := unique(paths)
			combinedPaths := strings.Join(uniquePaths, ", ")

			// Construct the combined expression
			combinedExpression = fmt.Sprintf("%s not in [%s]", key, combinedPaths)
		}
		// Add image name and image tag args
		if args.imageName != "" {
			combinedExpression = fmt.Sprintf("%s && container.tags == \"image_name:%s\"", combinedExpression, args.imageName)
		}
		if args.imageTag != "" {
			combinedExpression = fmt.Sprintf("%s && container.tags == \"image_tag:%s\"", combinedExpression, args.imageTag)
		}
		// Add the new combined rule to the rules slice

		tmp := rules.RuleDefinition{Expression: combinedExpression}
		if args.kill {
			tmp.Actions = []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			}
		}
		res = append(res, &tmp)
	}
	return res
}

// extractFieldAndOperation extracts the field and operation from an expression
func extractFieldAndOperation(expression string) string {
	// Extract the part of the expression before the "not in"
	re := regexp.MustCompile(`(\S+\.\S+)\snot\s+in`)
	match := re.FindStringSubmatch(expression)
	if len(match) > 1 {
		return match[1]
	}
	return expression
}

// extractPaths extracts the paths from the "not in [ ... ]" part of the expression
func extractPaths(expression string) []string {
	// Regular expression to match the paths inside the square brackets
	re := regexp.MustCompile(`\[(.*?)\]`)
	match := re.FindStringSubmatch(expression)
	if len(match) > 1 {
		// Split the matched paths by commas
		return strings.Split(match[1], ",")
	}
	return nil
}

// unique removes duplicate paths from a slice
func unique(paths []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, path := range paths {
		if _, ok := seen[path]; !ok {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}
