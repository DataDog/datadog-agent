// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type activityDumpCliParams struct {
	*command.GlobalParams

	name                     string
	containerID              string
	comm                     string
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
	}

	activityDumpCmd.AddCommand(generateCommands(globalParams)...)
	activityDumpCmd.AddCommand(listCommands(globalParams)...)
	activityDumpCmd.AddCommand(stopCommands(globalParams)...)
	activityDumpCmd.AddCommand(diffCommands(globalParams)...)
	return []*cobra.Command{activityDumpCmd}
}

func listCommands(globalParams *command.GlobalParams) []*cobra.Command {
	activityDumpListCmd := &cobra.Command{
		Use:   "list",
		Short: "get the list of running activity dumps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(listActivityDumps,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(stopActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	activityDumpStopCmd.Flags().StringVar(
		&cliParams.name,
		flags.Name,
		"",
		"an activity dump name can be used to filter the activity dump.",
	)
	activityDumpStopCmd.Flags().StringVar(
		&cliParams.containerID,
		flags.ContainerID,
		"",
		"an containerID can be used to filter the activity dump.",
	)
	activityDumpStopCmd.Flags().StringVar(
		&cliParams.comm,
		flags.Comm,
		"",
		"a process command can be used to filter the activity dump from a specific process.",
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(generateActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.comm,
		flags.Comm,
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.containerID,
		flags.ContainerID,
		"",
		"a container identifier can be used to filter the activity dump from a specific container.",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.timeout,
		flags.Timeout,
		"1m",
		"timeout for the activity dump",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.differentiateArgs,
		flags.DifferentiateArgs,
		true,
		"add the arguments in the process node merge algorithm",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&cliParams.localStorageDirectory,
		flags.Output,
		"/tmp/activity_dumps/",
		"local storage output directory",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.localStorageCompression,
		flags.Compression,
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&cliParams.localStorageFormats,
		flags.Format,
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&cliParams.remoteStorageCompression,
		flags.RemoteCompression,
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&cliParams.remoteStorageFormats,
		flags.RemoteFormat,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(generateEncodingFromActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&cliParams.file,
		flags.Input,
		"",
		"path to the activity dump file",
	)
	_ = activityDumpGenerateEncodingCmd.MarkFlagRequired(flags.Input)
	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&cliParams.localStorageDirectory,
		flags.Output,
		"/tmp/activity_dumps/",
		"local storage output directory",
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.localStorageCompression,
		flags.Compression,
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&cliParams.localStorageFormats,
		flags.Format,
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.remoteStorageCompression,
		flags.RemoteCompression,
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&cliParams.remoteStorageFormats,
		flags.RemoteFormat,
		[]string{},
		fmt.Sprintf("remote storage output formats. Available options are %v.", secconfig.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&cliParams.remoteRequest,
		flags.Remote,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(diffActivityDump,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	activityDumpDiffCmd.Flags().StringVar(
		&cliParams.file,
		flags.Origin,
		"",
		"path to the first activity dump file",
	)

	activityDumpDiffCmd.Flags().StringVar(
		&cliParams.file2,
		flags.Target,
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

func computeActivityDumpDiff(p1, p2 *dump.ActivityDump, states map[string]bool) *dump.ActivityDump {
	return &dump.ActivityDump{
		Mutex: new(sync.Mutex),
		ActivityTree: &activity_tree.ActivityTree{
			ProcessNodes: diffADSubtree(p1.ActivityTree.ProcessNodes, p2.ActivityTree.ProcessNodes, states),
		},
	}
}

func diffActivityDump(log log.Component, config config.Component, args *activityDumpCliParams) error {
	ad := dump.NewEmptyActivityDump(nil)
	if err := ad.Decode(args.file); err != nil {
		return err
	}

	ad2 := dump.NewEmptyActivityDump(nil)
	if err := ad2.Decode(args.file2); err != nil {
		return err
	}

	states := make(map[string]bool)
	diff := computeActivityDumpDiff(ad, ad2, states)

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
		buffer, err := graph.EncodeDOT(dump.ActivityDumpGraphTemplate)
		if err != nil {
			return err
		}
		os.Stdout.Write(buffer.Bytes())
	case "protobuf":
		buffer, err := diff.EncodeProtobuf()
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

func generateActivityDump(log log.Component, config config.Component, activityDumpArgs *activityDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	storage, err := parseStorageRequest(activityDumpArgs)
	if err != nil {
		return err
	}

	output, err := client.GenerateActivityDump(&api.ActivityDumpParams{
		Comm:              activityDumpArgs.comm,
		ContainerID:       activityDumpArgs.containerID,
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

func generateEncodingFromActivityDump(log log.Component, config config.Component, activityDumpArgs *activityDumpCliParams) error {
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
		ad := dump.NewEmptyActivityDump(nil)

		// open and parse input file
		if err := ad.Decode(activityDumpArgs.file); err != nil {
			return err
		}
		parsedRequests, err := parseStorageRequest(activityDumpArgs)
		if err != nil {
			return err
		}

		storageRequests, err := secconfig.ParseStorageRequests(parsedRequests)
		if err != nil {
			return fmt.Errorf("couldn't parse transcoding request for [%s]: %v", ad.GetSelectorStr(), err)
		}
		for _, request := range storageRequests {
			ad.AddStorageRequest(request)
		}

		cfg, err := secconfig.NewConfig()
		if err != nil {
			return fmt.Errorf("couldn't load configuration: %w", err)

		}
		storage, err := dump.NewSecurityAgentCommandStorageManager(cfg)
		if err != nil {
			return fmt.Errorf("couldn't instantiate storage manager: %w", err)
		}

		err = storage.Persist(ad)
		if err != nil {
			return fmt.Errorf("couldn't persist dump from %s: %w", activityDumpArgs.file, err)
		}

		output = ad.ToTranscodingRequestMessage()
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

func listActivityDumps(log log.Component, config config.Component) error {
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

func stopActivityDump(log log.Component, config config.Component, activityDumpArgs *activityDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.StopActivityDump(activityDumpArgs.name, activityDumpArgs.containerID, activityDumpArgs.comm)
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump stop request failed: %s", output.Error)
	}

	fmt.Println("done!")
	return nil
}
