// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package remoteconfig implements 'agent remote-config'.
package remoteconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	remoteConfigCmd := &cobra.Command{
		Use:   "remote-config",
		Short: "Remote configuration state command",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(state,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfFilePath:      globalParams.ConfFilePath,
					ConfigLoadSecrets: false,
				}),
				core.Bundle,
			)
		},
		Hidden: true,
	}

	return []*cobra.Command{remoteConfigCmd}
}

func state(cliParams *cliParams, config config.Component) error {
	if !config.GetBool("remote_configuration.enabled") {
		return fmt.Errorf("Remote configuration is not enabled")
	}
	fmt.Println("Fetching the configuration and director repos state..")
	// Call GRPC endpoint returning state tree

	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("Couldn't get auth token: %v", err)
	}
	ctx, close := context.WithCancel(context.Background())
	defer close()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("Couldn't get the repositories state: %v", err)
	}

	fmt.Println("\nConfiguration repository")
	fmt.Println(strings.Repeat("-", 25))
	printTUFRepo(s.ConfigState)

	fmt.Println("\nDirector repository")
	fmt.Println(strings.Repeat("-", 20))
	printTUFRepo(s.DirectorState)
	keys := make([]string, 0, len(s.TargetFilenames))
	for k := range s.TargetFilenames {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Printf("    |- %s - Hash: %s\n", name, s.TargetFilenames[name])
	}

	return nil
}

func getStateString(state *pbgo.FileMetaState, padding int) string {
	if state == nil {
		return color.YellowString(fmt.Sprintf("%*s\n", padding, "- Not found"))
	}
	return fmt.Sprintf("%*s: %9d - Hash: %s\n", padding, "- Version", state.Version, state.Hash)
}

func printAndRemoveFile(repo map[string]*pbgo.FileMetaState, name string, prefix string, padding int) {
	file, found := repo[name]
	fmt.Printf("%s%s%s", prefix, name, getStateString(file, padding))
	if found {
		delete(repo, name)
	}
}

func printTUFRepo(repo map[string]*pbgo.FileMetaState) {
	printAndRemoveFile(repo, "root.json", "", 20)
	printAndRemoveFile(repo, "timestamp.json", "|- ", 12)
	printAndRemoveFile(repo, "snapshot.json", "|- ", 13)
	printAndRemoveFile(repo, "targets.json", "|- ", 14)

	// Sort the keys to display the delegated targets in order
	keys := make([]string, 0, len(repo))
	for k := range repo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Printf("    |- %s %s\n", name, getStateString(repo[name], 4))
	}
}
