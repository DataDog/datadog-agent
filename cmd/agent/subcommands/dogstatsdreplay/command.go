// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

// Package dogstatsdreplay implements 'agent dogstatsd-replay'.
package dogstatsdreplay

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

const (
	defaultIterations = 1
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// subcommand-specific flags

	dsdReplayFilePath   string
	dsdVerboseReplay    bool
	dsdMmapReplay       bool
	dsdReplayIterations int
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	dogstatsdReplayCmd := &cobra.Command{
		Use:   "dogstatsd-replay",
		Short: "Replay dogstatsd traffic",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dogstatsdReplay,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}
	dogstatsdReplayCmd.Flags().StringVarP(&cliParams.dsdReplayFilePath, "file", "f", "", "Input file with traffic captured with dogstatsd-capture.")
	dogstatsdReplayCmd.Flags().BoolVarP(&cliParams.dsdVerboseReplay, "verbose", "v", false, "Verbose replay.")
	dogstatsdReplayCmd.Flags().BoolVarP(&cliParams.dsdMmapReplay, "mmap", "m", true, "Mmap file for replay. Set to false to load the entire file into memory instead")
	dogstatsdReplayCmd.Flags().IntVarP(&cliParams.dsdReplayIterations, "loops", "l", defaultIterations, "Number of iterations to replay.")

	return []*cobra.Command{dogstatsdReplayCmd}
}

func dogstatsdReplay(log log.Component, config config.Component, cliParams *cliParams) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup sig handlers
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		_ = <-sigs
		done <- true
	}()

	fmt.Printf("Replaying dogstatsd traffic...\n\n")

	// TODO: refactor all the instantiation of the SecureAgentClient to a helper
	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("unable to fetch authentication token: %w", err)
	}

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	apiconn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", pkgconfig.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	cli := pb.NewAgentSecureClient(apiconn)

	depth := 10
	reader, err := replay.NewTrafficCaptureReader(cliParams.dsdReplayFilePath, depth, cliParams.dsdMmapReplay)
	if reader != nil {
		defer reader.Close()
	}

	if err != nil {
		fmt.Printf("could not open: %s\n", cliParams.dsdReplayFilePath)
		return err
	}

	s := pkgconfig.Datadog.GetString("dogstatsd_socket")
	if s == "" {
		return fmt.Errorf("Dogstatsd UNIX socket disabled")
	}

	addr, err := net.ResolveUnixAddr("unixgram", s)
	if err != nil {
		return err
	}

	sk, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(sk)

	err = syscall.SetsockoptInt(sk, syscall.SOL_SOCKET, syscall.SO_SNDBUF,
		pkgconfig.Datadog.GetInt("dogstatsd_buffer_size"))
	if err != nil {
		return err
	}

	dsdSock := os.NewFile(uintptr(sk), "dogstatsd_socket")
	conn, err := net.FileConn(dsdSock)
	if err != nil {
		return err
	}
	defer conn.Close()

	// let's read state before proceeding
	pidmap, state, err := reader.ReadState()
	if err != nil {
		fmt.Printf("Unable to load state from file, tag enrichment will be unavailable for this capture: %v\n", err)
	}

	resp, err := cli.DogstatsdSetTaggerState(ctx, &pb.TaggerState{State: state, PidMap: pidmap})
	if err != nil {
		fmt.Printf("Unable to load state API error, tag enrichment will be unavailable for this capture: %v\n", err)
	} else if !resp.GetLoaded() {
		fmt.Printf("API refused to set the tagger state, tag enrichment will be unavailable for this capture.\n")
	}

	breaker := false
	for i := 0; (i < cliParams.dsdReplayIterations || cliParams.dsdReplayIterations == 0) && !breaker; i++ {

		// enable reading at natural rate
		ready := make(chan struct{})
		go reader.Read(ready)

		// wait for go routine to start processing...
		<-ready

	replay:
		for {
			select {
			case msg := <-reader.Traffic:
				// The cadence is enforced by the reader. The reader will only write to
				// the traffic channel when it estimates the payload should be submitted.
				n, oobn, err := conn.(*net.UnixConn).WriteMsgUnix(
					msg.Payload[:msg.PayloadSize], replay.GetUcredsForPid(msg.Pid), addr)
				if err != nil {
					return err
				}

				if cliParams.dsdVerboseReplay {
					fmt.Printf("Sent Payload: %d bytes, and OOB: %d bytes\n", n, oobn)
				}
			case <-reader.Done:
				break replay
			case <-done:
				breaker = true
				break replay
			}
		}
	}

	fmt.Println("clearing agent replay states...")
	resp, err = cli.DogstatsdSetTaggerState(ctx, &pb.TaggerState{})
	if err != nil {
		fmt.Printf("Unable to load state API error, tag enrichment will be unavailable for this capture: %v\n", err)
	} else if resp.GetLoaded() {
		fmt.Printf("The capture state and pid map have been successfully cleared from the agent\n")
	}

	err = reader.Shutdown()
	if err != nil {
		fmt.Printf("There was an issue shutting down the replay: %v\n", err)
	}

	fmt.Println("replay done")
	return err
}
