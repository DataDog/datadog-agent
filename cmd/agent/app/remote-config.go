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
	Use:   "remote-config [command]",
	Short: "Remote configuration state command",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		return state(cmd, args)
	},
}

func state(cmd *cobra.Command, args []string, dialOpts ...grpc.DialOption) error {
	fmt.Println("Fetching the configuration and director repos state..")
	// Call GRPC endpoint returning state tree
	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		return fmt.Errorf("Unable to set up global agent configuration: %v", err)
	}
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
	fmt.Println(strings.Repeat("-", 25))
	printTUFRepo(s.DirectorState)

	return nil
}

func printTUFRepo(repo map[string]*pbgo.FileMetaState) error {
	root, found := repo["root.json"]
	fmt.Print("root.json")
	if found {
		fmt.Printf("%20s: %9d - Hash: %s\n", "- Version", root.Version, root.Hash)
		delete(repo, "root.json")
	} else {
		fmt.Printf(color.YellowString("%20s\n", "- Not found"))
	}

	timestamp, found := repo["timestamp.json"]
	fmt.Print("|- timestamp.json")
	if found {
		fmt.Printf("%12s: %9d - Hash: %s\n", "- Version", timestamp.Version, timestamp.Hash)
		delete(repo, "timestamp.json")
	} else {
		fmt.Printf(color.YellowString("%14s\n", "- Not found"))
	}

	snapshot, found := repo["snapshot.json"]
	fmt.Print("|- snapshot.json")
	if found {
		fmt.Printf("%13s: %9d - Hash: %s\n", "- Version", snapshot.Version, snapshot.Hash)
		delete(repo, "snapshot.json")
	} else {
		fmt.Printf(color.YellowString("%15s\n", "- Not found"))
	}

	targets, found := repo["targets.json"]
	fmt.Print("|- targets.json")
	if found {
		fmt.Printf("%14s: %9d - Hash: %s\n", "- Version", targets.Version, targets.Hash)
		delete(repo, "targets.json")
	} else {
		fmt.Printf(color.YellowString("%16s\n", "- Not found"))
	}

	for name, state := range repo {
		fmt.Printf("    |- %s - Version: %9d - Hash: %s\n", name, state.Version, state.Hash)
	}

	return nil
}
