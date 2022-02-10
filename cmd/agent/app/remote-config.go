// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	AgentCmd.AddCommand(remoteConfigCmd)
}

var remoteConfigCmd = &cobra.Command{
	Use:   "remote-config",
	Short: "Remote configuration state command",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("Unable to set up global agent configuration: %v", err)
		}

		if !config.Datadog.GetBool("remote_configuration.enabled") {
			return fmt.Errorf("Remote configuration is not enabled")
		}
		return state(cmd, args)
	},
}

func state(cmd *cobra.Command, args []string, dialOpts ...grpc.DialOption) error {
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

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx, dialOpts...)
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigFullState(ctx, in)
	if err != nil {
		return fmt.Errorf("Couldn't get the repositories state: %v", err)
	}

	fmt.Println("\nConfiguration repository")
	fmt.Println(strings.Repeat("-", 25))
	printTUFRepo(s.ConfigState)

	fmt.Println("\nDirector repository")
	fmt.Println(strings.Repeat("-", 20))
	printTUFRepo(s.DirectorState)

	return nil
}

func getStateString(state *pbgo.FileMetaState, padding int) string {
	if state == nil {
		return fmt.Sprintf(color.YellowString("%*s\n", padding, "- Not found"))
	}
	return fmt.Sprintf("%*s: %9d - Hash: %s\n", padding, "- Version", state.Version, state.Hash)
}

func printTUFRepo(repo map[string]*pbgo.FileMetaState) error {
	root, found := repo["root.json"]
	fmt.Print("root.json")
	if found {
		delete(repo, "root.json")
	}
	fmt.Print(getStateString(root, 20))

	timestamp, found := repo["timestamp.json"]
	fmt.Print("|- timestamp.json")
	if found {
		delete(repo, "timestamp.json")
	}
	fmt.Print(getStateString(timestamp, 12))

	snapshot, found := repo["snapshot.json"]
	fmt.Print("|- snapshot.json")
	if found {
		delete(repo, "snapshot.json")
	}
	fmt.Print(getStateString(snapshot, 13))

	targets, found := repo["targets.json"]
	fmt.Print("|- targets.json")
	if found {
		delete(repo, "targets.json")
	}
	fmt.Print(getStateString(targets, 14))

	for name, state := range repo {
		fmt.Printf("    |- %s %s\n", name, getStateString(state, 4))
	}

	return nil
}
