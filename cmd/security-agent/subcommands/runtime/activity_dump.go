// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package runtime

import (
	"fmt"

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
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type activityDumpCliParams struct {
	*command.GlobalParams

	name                     string
	containerID              string
	comm                     string
	file                     string
	timeout                  int
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
	activityDumpGenerateDumpCmd.Flags().IntVar(
		&cliParams.timeout,
		flags.Timeout,
		60,
		"timeout for the activity dump in minutes",
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
		Timeout:           int32(activityDumpArgs.timeout),
		DifferentiateArgs: activityDumpArgs.differentiateArgs,
		Storage:           storage,
	})
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
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
		ad := dump.NewEmptyActivityDump()

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
		storage, err := dump.NewActivityDumpStorageManager(cfg, nil, nil)
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
		return fmt.Errorf("unable send request to system-probe: %w", err)
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
		return fmt.Errorf("unable send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump stop request failed: %s", output.Error)
	}

	fmt.Println("done!")
	return nil
}
