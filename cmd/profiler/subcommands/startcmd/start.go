// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package startcmd holds the start command of CWS injector
package startcmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activitytree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

const (
	gRPCAddr          = "grpc-addr"
	gRPCAddrDefault   = "localhost:5678"
	outputfile        = "output-file"
	outputfileDefault = "my-workload"
)

type profileCliParams struct {
	GRPCAddr   string
	OutputFile string
}

// Command returns the commands for the profile subcommand
func Command() []*cobra.Command {
	var params profileCliParams

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "start learning a workload from the ptracer and generate a profile when finished",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Start(args, params.GRPCAddr, params.OutputFile)
		},
	}

	startCmd.Flags().StringVar(&params.GRPCAddr, gRPCAddr, gRPCAddrDefault, "ptracer GRPC addr")
	startCmd.Flags().StringVar(&params.OutputFile, outputfile, outputfileDefault, "enable verbose output")

	return []*cobra.Command{startCmd}
}

type ProfilerContext struct {
	ebpfless.UnimplementedSyscallMsgStreamServer
	server          *grpc.Server
	seqNum          uint64
	processResolver *process.EBPFLessResolver
	dump            *dump.ActivityDump
}

func Start(args []string, gRPCAddr string, outputFile string) error {
	fmt.Printf("Starting to learn a new workload\n")

	// init sig handler for ^-C
	done := make(chan bool, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	// init process resolver
	processResolver, err := process.NewEBPFLessResolver(nil /*config*/, statsdclient.NewStatsdClient(), nil /*scrubber*/, &process.ResolverOpts{})
	if err != nil {
		return err
	}

	// init activity tree
	dump := dump.NewEmptyActivityDump(activitytree.NewPathsReducer())
	dump.LoadConfig = &model.ActivityDumpLoadConfig{
		TracedEventTypes: []model.EventType{
			model.ExecEventType,
			model.FileOpenEventType,
		},
	}

	// init and launch server
	lis, err := net.Listen("tcp", gRPCAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var grpcOpts []grpc.ServerOption
	ctx := &ProfilerContext{
		server:          grpc.NewServer(grpcOpts...),
		processResolver: processResolver,
		dump:            dump,
	}
	ebpfless.RegisterSyscallMsgStreamServer(ctx.server, ctx)

	go ctx.server.Serve(lis)

	<-done

	if len(dump.ActivityTree.ProcessNodes) == 0 {
		fmt.Printf("\nThe profile is empty, exiting.\n")
		return nil
	}

	fmt.Printf("\nEncoding the dump to profile\n")
	buf, err := dump.Encode(config.Profile)
	if err != nil {
		return err
	}
	file, err := os.Create(outputFile + ".profile")
	if err != nil {
		return fmt.Errorf("couldn't persist to file: %w", err)
	}
	defer file.Close()
	if _, err = file.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("couldn't write to file: %w", err)
	}

	fmt.Printf("Encoding the dump to dot\n")
	buf, err = dump.Encode(config.Dot)
	if err != nil {
		return err
	}
	file, err = os.Create(outputFile + ".dot")
	if err != nil {
		return fmt.Errorf("couldn't persist to file: %w", err)
	}
	defer file.Close()
	if _, err = file.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("couldn't write to file: %w", err)
	}

	cmd := exec.Command("sh", "-c", "dot -Tsvg "+outputFile+".dot > "+outputFile+".svg")
	_, err = cmd.Output()
	if err != nil {
		fmt.Println("could not run command: ", err)
	}
	return nil
}
