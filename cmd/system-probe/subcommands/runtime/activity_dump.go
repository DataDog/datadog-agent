// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(listActivityDumps,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(false),
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
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(false),
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
		"an containerID can be used to filter the activity dump.",
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
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(false),
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
		"1m",
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
		"/tmp/activity_dumps/",
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
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(false),
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
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(false),
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
		"output format",
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
		fmt.Print(buffer.String())
	case "protobuf":
		buffer, err := diff.EncodeSecDumpProtobuf()
		if err != nil {
			return err
		}
		fmt.Print(buffer.String())
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
	client, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	storage, err := parseStorageRequest(activityDumpArgs)
	if err != nil {
		return err
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
				backend, err := backend.NewActivityDumpRemoteBackend()
				if err != nil {
					return fmt.Errorf("couldn't instantiate remote storage backend: %w", err)
				}

				remoteStorage, remoteStorageErr = storage.NewActivityDumpRemoteStorageForwarder(backend)
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
	client, err := secagent.NewRuntimeSecurityCmdClient()
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
	client, err := secagent.NewRuntimeSecurityCmdClient()
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

func printStorageRequestMessage(prefix string, storage *api.StorageRequestMessage) {
	fmt.Printf("%s- file: %s\n", prefix, storage.GetFile())
	fmt.Printf("%s  format: %s\n", prefix, storage.GetFormat())
	fmt.Printf("%s  storage type: %s\n", prefix, storage.GetType())
	fmt.Printf("%s  compression: %v\n", prefix, storage.GetCompression())
}
