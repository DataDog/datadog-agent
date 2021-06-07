// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/replay"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	dsdReplayFilePath string
	dsdTaggerFilePath string
)

func init() {
	AgentCmd.AddCommand(dogstatsdReplayCmd)
	dogstatsdReplayCmd.Flags().StringVarP(&dsdReplayFilePath, "file", "f", "", "Input file with TCP traffic to replay.")
	dogstatsdReplayCmd.Flags().StringVarP(&dsdTaggerFilePath, "tagger", "t", "", "Input file with TCP traffic to replay.")
}

var dogstatsdReplayCmd = &cobra.Command{
	Use:   "dogstatsd-replay",
	Short: "Replay dogstatsd traffic",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return dogstatsdReplay()
	},
}

func dogstatsdReplay() error {

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
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	cli := pb.NewAgentSecureClient(apiconn)

	depth := 10
	reader, err := replay.NewTrafficCaptureReader(dsdReplayFilePath, depth)
	if reader != nil {
		defer reader.Close()
	}

	if err != nil {
		return err
	}

	s := config.Datadog.GetString("dogstatsd_socket")
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
		config.Datadog.GetInt("dogstatsd_buffer_size"))
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

	// enable reading at natural rate
	go reader.Read()

	// wait for go routine to start processing...
	time.Sleep(time.Second)

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
			fmt.Printf("Sent Payload: %d bytes, and OOB: %d bytes\n", n, oobn)
		case <-reader.Done:
			break replay
		case <-done:
			break replay
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
