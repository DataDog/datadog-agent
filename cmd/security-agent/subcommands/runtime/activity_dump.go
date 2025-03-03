// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/pathutils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type activityDumpCliParams struct {
	*command.GlobalParams

	name                     string
	containerID              string
	cgroupID                 string
	file                     string
	file2                    string
	timeout                  string
	format                   string
	differentiateArgs        bool
	localStorageDirectory    string
	localStorageFormats      []string
	localStorageCompression  bool
	remoteStorageFormats     []string
	remoteStorageCompression bool
	remoteRequest            bool
}

func activityDumpCommands(globalParams *command.GlobalParams) []*cobra.Command {
	activityDumpCmd := &cobra.Command{
		Use:   "activity-dump",
		Short: "activity dump command",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			model.SECLConstants()
		},
	}

	activityDumpCmd.AddCommand(generateCommands(globalParams)...)
	activityDumpCmd.AddCommand(listCommands(globalParams)...)
	activityDumpCmd.AddCommand(stopCommands(globalParams)...)
	activityDumpCmd.AddCommand(diffCommands(globalParams)...)
	activityDumpCmd.AddCommand(activityDumpToWorkloadPolicyCommands(globalParams)...)
	activityDumpCmd.AddCommand(activityDumpToSeccompProfileCommands(globalParams)...)
	return []*cobra.Command{activityDumpCmd}
}

func listCommands(globalParams *command.GlobalParams) []*cobra.Command {
	activityDumpListCmd := &cobra.Command{
		Use:   "list",
		Short: "get the list of running activity dumps",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(listActivityDumps,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{activityDumpListCmd}
}

func stopCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpCliParams{
		GlobalParams: globalParams,
	}

	activityDumpStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "stops the first activity dump that matches the provided selector",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(stopActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	activityDumpStopCmd.Flags().StringVar(
		&cliParams.name,
		"name",
		"",
		"an activity dump name can be used to filter the activity dump.",
	)
	activityDumpStopCmd.Flags().StringVar(
		&cliParams.containerID,
		"container-id",
		"",
		"a containerID can be used to filter the activity dump.",
	)
	activityDumpStopCmd.Flags().StringVar(
		&cliParams.cgroupID,
		"cgroup-id",
		"",
		"a cgroup ID can be used to filter the activity dump.",
	)

	return []*cobra.Command{activityDumpStopCmd}
}

func generateCommands(globalParams *command.GlobalParams) []*cobra.Command {
	activityDumpGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "generate command for activity dumps",
	}

	activityDumpGenerateCmd.AddCommand(generateDumpCommands(globalParams)...)
	activityDumpGenerateCmd.AddCommand(generateEncodingCommands(globalParams)...)

	return []*cobra.Command{activityDumpGenerateCmd}
}

func generateDumpCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpCliParams{
		GlobalParams: globalParams,
	}

	activityDumpGenerateDumpCmd := &cobra.Command{
		Use:   "dump",
		Short: "generate an activity dump",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(generateActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.containerID,
		"container-id",
		"",
		"a container identifier can be used to filter the activity dump from a specific container.",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.cgroupID,
		"cgroup-id",
		"",
		"a cgroup identifier can be used to filter the activity dump from a specific cgroup.",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.timeout,
		"timeout",
		"",
		"timeout for the activity dump",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.differentiateArgs,
		"differentiate-args",
		true,
		"add the arguments in the process node merge algorithm",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.localStorageDirectory,
		"output",
		"",
		"local storage output directory",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.localStorageCompression,
		"compression",
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&cliParams.localStorageFormats,
		"format",
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.remoteStorageCompression,
		"remote-compression",
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&cliParams.remoteStorageFormats,
		"remote-format",
		[]string{},
		fmt.Sprintf("remote storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)

	return []*cobra.Command{activityDumpGenerateDumpCmd}
}

func generateEncodingCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpCliParams{
		GlobalParams: globalParams,
	}

	activityDumpGenerateEncodingCmd := &cobra.Command{
		Use:   "encoding",
		Short: "encode an activity dump to the requested formats",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(generateEncodingFromActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&cliParams.file,
		"input",
		"",
		"path to the activity dump file",
	)
	_ = activityDumpGenerateEncodingCmd.MarkFlagRequired("input")
	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&cliParams.localStorageDirectory,
		"output",
		"/tmp/activity_dumps/",
		"local storage output directory",
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.localStorageCompression,
		"compression",
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&cliParams.localStorageFormats,
		"format",
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.remoteStorageCompression,
		"remote-compression",
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&cliParams.remoteStorageFormats,
		"remote-format",
		[]string{},
		fmt.Sprintf("remote storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.remoteRequest,
		"remote",
		false,
		"when set, the transcoding will be done by system-probe instead of the current security-agent instance",
	)

	return []*cobra.Command{activityDumpGenerateEncodingCmd}
}

func diffCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpCliParams{
		GlobalParams: globalParams,
	}

	activityDumpDiffCmd := &cobra.Command{
		Use:   "diff",
		Short: "compute the diff between two activity dumps",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(diffActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	activityDumpDiffCmd.Flags().StringVar(
		&cliParams.file,
		"origin",
		"",
		"path to the first activity dump file",
	)

	activityDumpDiffCmd.Flags().StringVar(
		&cliParams.file2,
		"target",
		"",
		"path to the second activity dump file",
	)

	activityDumpDiffCmd.Flags().StringVar(
		&cliParams.format,
		"format",
		"json",
		"output formeat",
	)

	return []*cobra.Command{activityDumpDiffCmd}
}

const (
	addedADNode   activity_tree.NodeGenerationType = 100
	removedADNode activity_tree.NodeGenerationType = 101
)

func markADSubtree(n *activity_tree.ProcessNode, state activity_tree.NodeGenerationType) {
	n.GenerationType = state
	for _, child := range n.Children {
		markADSubtree(child, state)
	}
}

func diffADDNSNodes(p1, p2 map[string]*activity_tree.DNSNode, states map[string]bool, processID utils.GraphID) (nodes map[string]*activity_tree.DNSNode) {
	nodes = make(map[string]*activity_tree.DNSNode)

	for domain, n := range p2 {
		if p1[domain] != nil {
			newNode := *n
			nodes[domain] = &newNode
			continue
		}

		states[processID.Derive(utils.NewNodeIDFromPtr(n)).String()] = true
		nodes[domain] = n
	}

	for domain, n := range p1 {
		if p2[domain] != nil {
			continue
		}

		id := processID.Derive(utils.NewNodeIDFromPtr(n)).String()
		states[id] = false
		nodes[domain] = n
	}

	return nodes
}

func diffADSubtree(p1, p2 []*activity_tree.ProcessNode, states map[string]bool) (nodes []*activity_tree.ProcessNode) {
NEXT:
	for _, n := range p2 {
		for _, n2 := range p1 {
			if n.Matches(&n2.Process, false, false) {
				newNode := *n
				processID := utils.NewGraphID(utils.NewNodeIDFromPtr(&newNode))
				newNode.Children = diffADSubtree(n.Children, n2.Children, states)
				newNode.DNSNames = diffADDNSNodes(n.DNSNames, n2.DNSNames, states, processID)
				nodes = append(nodes, &newNode)
				continue NEXT
			}
		}

		nodes = append(nodes, n)
		markADSubtree(n, addedADNode)
		states[utils.NewGraphID(utils.NewNodeIDFromPtr(n)).String()] = true
	}

NEXT2:
	for _, n := range p1 {
		for _, n2 := range p2 {
			if n.Matches(&n2.Process, false, false) {
				continue NEXT2
			}
		}

		nodes = append(nodes, n)
		markADSubtree(n, removedADNode)
		states[utils.NewGraphID(utils.NewNodeIDFromPtr(n)).String()] = false
	}

	return
}

func computeActivityDumpDiff(p1, p2 *profile.Profile, states map[string]bool) *profile.Profile {
	p := profile.New()
	p.ActivityTree = &activity_tree.ActivityTree{
		ProcessNodes: diffADSubtree(p1.ActivityTree.ProcessNodes, p2.ActivityTree.ProcessNodes, states),
	}
	return p
}

func diffActivityDump(_ log.Component, _ config.Component, _ secrets.Component, args *activityDumpCliParams) error {
	p := profile.New()
	if err := p.Decode(args.file); err != nil {
		return err
	}

	p2 := profile.New()
	if err := p2.Decode(args.file2); err != nil {
		return err
	}

	states := make(map[string]bool)
	diff := computeActivityDumpDiff(p, p2, states)

	switch args.format {
	case "dot":
		graph := diff.ToGraph()
		for i := range graph.Nodes {
			n := graph.Nodes[i]
			if state, found := states[n.ID.String()]; found {
				if state {
					n.FillColor = "green"
				} else {
					n.FillColor = "red"
				}
			}
		}
		buffer, err := graph.EncodeDOT(profile.ActivityDumpGraphTemplate)
		if err != nil {
			return err
		}
		os.Stdout.Write(buffer.Bytes())
	case "protobuf":
		buffer, err := diff.EncodeSecDumpProtobuf()
		if err != nil {
			return err
		}
		os.Stdout.Write(buffer.Bytes())
	case "json":
		buffer, err := diff.EncodeJSON("  ")
		if err != nil {
			return err
		}
		fmt.Println(buffer.String())
	default:
		return fmt.Errorf("unknown format '%s'", args.format)
	}

	return nil
}

func generateActivityDump(_ log.Component, _ config.Component, _ secrets.Component, activityDumpArgs *activityDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	storage, err := parseStorageRequest(activityDumpArgs)
	if err != nil {
		return err
	}

	if activityDumpArgs.timeout != "" {
		if _, err = time.ParseDuration(activityDumpArgs.timeout); err != nil {
			return err
		}
	}

	output, err := client.GenerateActivityDump(&api.ActivityDumpParams{
		ContainerID:       activityDumpArgs.containerID,
		CGroupID:          activityDumpArgs.cgroupID,
		Timeout:           activityDumpArgs.timeout,
		DifferentiateArgs: activityDumpArgs.differentiateArgs,
		Storage:           storage,
	})
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump generation request failed: %s", output.Error)
	}

	printSecurityActivityDumpMessage("", output)
	return nil
}

func generateEncodingFromActivityDump(_ log.Component, _ config.Component, _ secrets.Component, activityDumpArgs *activityDumpCliParams) error {
	var output *api.TranscodingRequestMessage

	if activityDumpArgs.remoteRequest {
		// send the encoding request to system-probe
		client, err := secagent.NewRuntimeSecurityClient()
		if err != nil {
			return fmt.Errorf("encoding generation failed: %w", err)
		}
		defer client.Close()

		// parse encoding request
		storage, err := parseStorageRequest(activityDumpArgs)
		if err != nil {
			return err
		}

		output, err = client.GenerateEncoding(&api.TranscodingRequestParams{
			ActivityDumpFile: activityDumpArgs.file,
			Storage:          storage,
		})
		if err != nil {
			return fmt.Errorf("couldn't send request to system-probe: %w", err)
		}

	} else {
		// encoding request will be handled locally
		p := profile.New()

		// open and parse input file
		if err := p.Decode(activityDumpArgs.file); err != nil {
			return err
		}
		parsedRequests, err := parseStorageRequest(activityDumpArgs)
		if err != nil {
			return err
		}

		storageRequests, err := secconfig.ParseStorageRequests(parsedRequests)
		if err != nil {
			return fmt.Errorf("couldn't parse transcoding request for [%s]: %v", p.GetSelectorStr(), err)
		}

		cfg, err := secconfig.NewConfig()
		if err != nil {
			return fmt.Errorf("couldn't load configuration: %w", err)

		}

		output = &api.TranscodingRequestMessage{
			Storage: make([]*api.StorageRequestMessage, 0, len(storageRequests)),
		}

		var localStorage storage.ActivityDumpStorage
		var localStorageErr error
		var remoteStorage storage.ActivityDumpStorage
		var remoteStorageErr error
		for _, storageRequest := range storageRequests {
			switch storageRequest.Type {
			case secconfig.LocalStorage:
				if localStorage == nil && localStorageErr == nil {
					localStorage, localStorageErr = storage.NewDirectory(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory, cfg.RuntimeSecurity.ActivityDumpLocalStorageMaxDumpsCount)
					if localStorageErr != nil {
						return fmt.Errorf("couldn't instantiate local storage: %w", localStorageErr)
					}
				}
				data, err := p.Encode(storageRequest.Format)
				if err != nil {
					return fmt.Errorf("couldn't encode activity dump: %w", err)
				}
				if err := localStorage.Persist(storageRequest, p, data); err != nil {
					return fmt.Errorf("couldn't persist dump from %s to local storage: %w", activityDumpArgs.file, err)
				}
				output.Storage = append(output.Storage, storageRequest.ToStorageRequestMessage(p.Metadata.Name))
			case secconfig.RemoteStorage:
				if remoteStorage == nil && remoteStorageErr == nil {
					remoteStorage, remoteStorageErr = storage.NewActivityDumpRemoteStorage()
					if remoteStorageErr != nil {
						return fmt.Errorf("couldn't instantiate remote storage: %w", remoteStorageErr)
					}
				}
				data, err := p.Encode(storageRequest.Format)
				if err != nil {
					return fmt.Errorf("couldn't encode activity dump: %w", err)
				}
				if err := remoteStorage.Persist(storageRequest, p, data); err != nil {
					return fmt.Errorf("couldn't persist dump from %s to remote storage: %w", activityDumpArgs.file, err)
				}
				output.Storage = append(output.Storage, storageRequest.ToStorageRequestMessage(p.Metadata.Name))
			}
		}
	}

	if len(output.GetError()) > 0 {
		return fmt.Errorf("encoding generation failed: %s", output.GetError())
	}
	if len(output.GetStorage()) > 0 {
		fmt.Printf("encoding generation succeeded:\n")
		for _, storage := range output.GetStorage() {
			printStorageRequestMessage("\t", storage)
		}
	} else {
		fmt.Println("encoding generation succeeded: empty output")
	}
	return nil
}

func listActivityDumps(_ log.Component, _ config.Component, _ secrets.Component) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.ListActivityDumps()
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump list request failed: %s", output.Error)
	}

	if len(output.Dumps) > 0 {
		fmt.Println("active dumps:")
		for _, d := range output.Dumps {
			printSecurityActivityDumpMessage("\t", d)
		}
	} else {
		fmt.Println("no active dumps found")
	}

	return nil
}

func parseStorageRequest(activityDumpArgs *activityDumpCliParams) (*api.StorageRequestParams, error) {
	// parse local storage formats
	_, err := secconfig.ParseStorageFormats(activityDumpArgs.localStorageFormats)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse local storage formats %v: %v", activityDumpArgs.localStorageFormats, err)
	}

	// parse remote storage formats
	_, err = secconfig.ParseStorageFormats(activityDumpArgs.remoteStorageFormats)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse remote storage formats %v: %v", activityDumpArgs.remoteStorageFormats, err)
	}
	return &api.StorageRequestParams{
		LocalStorageDirectory:    activityDumpArgs.localStorageDirectory,
		LocalStorageCompression:  activityDumpArgs.localStorageCompression,
		LocalStorageFormats:      activityDumpArgs.localStorageFormats,
		RemoteStorageCompression: activityDumpArgs.remoteStorageCompression,
		RemoteStorageFormats:     activityDumpArgs.remoteStorageFormats,
	}, nil
}

func stopActivityDump(_ log.Component, _ config.Component, _ secrets.Component, activityDumpArgs *activityDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.StopActivityDump(activityDumpArgs.name, activityDumpArgs.containerID, activityDumpArgs.cgroupID)
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump stop request failed: %s", output.Error)
	}

	fmt.Println("done!")
	return nil
}

type activityDumpToWorkloadPolicyCliParams struct {
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

func activityDumpToWorkloadPolicyCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpToWorkloadPolicyCliParams{
		GlobalParams: globalParams,
	}

	ActivityDumpWorkloadPolicyCmd := &cobra.Command{
		Use:    "workload-policy",
		Hidden: true,
		Short:  "convert an activity dump to a workload policy",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(activityDumpToWorkloadPolicy,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	ActivityDumpWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.input,
		"input",
		"",
		"path to the activity-dump file",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.output,
		"output",
		"",
		"path to the generated workload policy file",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.kill,
		"kill",
		false,
		"generate kill action with the workload policy",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.fim,
		"fim",
		false,
		"generate fim rules with the workload policy",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.allowlist,
		"allowlist",
		false,
		"generate allow list rules",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().BoolVar(
		&cliParams.lineage,
		"lineage",
		false,
		"generate lineage rules",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.service,
		"service",
		"",
		"apply on specified service",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.imageTag,
		"image-tag",
		"",
		"apply on specified image tag",
	)

	ActivityDumpWorkloadPolicyCmd.Flags().StringVar(
		&cliParams.imageName,
		"image-name",
		"",
		"apply on specified image name",
	)

	return []*cobra.Command{ActivityDumpWorkloadPolicyCmd}
}

func activityDumpToWorkloadPolicy(_ log.Component, _ config.Component, _ secrets.Component, args *activityDumpToWorkloadPolicyCliParams) error {

	opts := securityprofile.SECLRuleOpts{
		EnableKill: args.kill,
		AllowList:  args.allowlist,
		Lineage:    args.lineage,
		Service:    args.service,
		ImageName:  args.imageName,
		ImageTag:   args.imageTag,
		FIM:        args.fim,
	}

	profiles, err := securityprofile.LoadActivityDumpsFromFiles(args.input)
	if err != nil {
		return err
	}

	generatedRules := securityprofile.GenerateRules(profiles, opts)
	generatedRules = pathutils.BuildPatterns(generatedRules)

	policyDef := rules.PolicyDef{
		Rules: generatedRules,
	}

	// Verify policy syntax
	var policyName string
	if len(args.imageName) > 0 {
		policyName = fmt.Sprintf("%s_policy", args.imageName)
	} else {
		policyName = "workload_policy"
	}
	policy, err := rules.LoadPolicyFromDefinition(policyName, "workload", rules.InternalPolicyType, &policyDef, nil, nil)

	if err != nil {
		return fmt.Errorf("error in generated ruleset's syntax: '%s'", err)
	}

	b, err := yaml.Marshal(policy)
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

type activityDumpToSeccompProfileCliParams struct {
	*command.GlobalParams

	input  string
	output string
	format string
}

func activityDumpToSeccompProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpToSeccompProfileCliParams{
		GlobalParams: globalParams,
	}

	ActivityDumpToSeccompProfileCmd := &cobra.Command{
		Use:    "workload-seccomp",
		Hidden: true,
		Short:  "convert an activity dump to a seccomp profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(activityDumpToSeccompProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}

	ActivityDumpToSeccompProfileCmd.Flags().StringVar(
		&cliParams.input,
		"input",
		"",
		"path to the activity-dump file",
	)

	ActivityDumpToSeccompProfileCmd.Flags().StringVar(
		&cliParams.output,
		"output",
		"",
		"path to the generated seccomp profile file",
	)

	ActivityDumpToSeccompProfileCmd.Flags().StringVar(
		&cliParams.format,
		"format",
		"json",
		"format of the generated seccomp profile file",
	)

	return []*cobra.Command{ActivityDumpToSeccompProfileCmd}
}
func activityDumpToSeccompProfile(_ log.Component, _ config.Component, _ secrets.Component, args *activityDumpToSeccompProfileCliParams) error {

	profiles, err := securityprofile.LoadActivityDumpsFromFiles(args.input)
	if err != nil {
		return err
	}

	seccompProfile := securityprofile.GenerateSeccompProfile(profiles)

	var b []byte
	if args.format == "yaml" {
		b, err = yaml.Marshal(seccompProfile)
	} else {
		b, err = json.Marshal(seccompProfile)
	}

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
