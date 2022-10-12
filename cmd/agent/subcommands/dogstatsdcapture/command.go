// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

// Package dogstatsdcapture implements 'agent dogstasd-capture'.
package dogstatsdcapture

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"

	"github.com/spf13/cobra"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
)

var (
	dsdCaptureDuration   time.Duration
	dsdCaptureFilePath   string
	dsdCaptureCompressed bool
)

const (
	defaultCaptureDuration = time.Duration(1) * time.Minute
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	dogstatsdCaptureCmd := &cobra.Command{
		Use:   "dogstatsd-capture",
		Short: "Start a dogstatsd UDS traffic capture",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := common.SetupConfigWithoutSecrets(globalParams.ConfFilePath, "")
			if err != nil {
				return fmt.Errorf("unable to set up global agent configuration: %v", err)
			}

			err = config.SetupLogger(config.CoreLoggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			return dogstatsdCapture()
		},
	}

	dogstatsdCaptureCmd.Flags().DurationVarP(&dsdCaptureDuration, "duration", "d", defaultCaptureDuration, "Duration traffic capture should span.")
	dogstatsdCaptureCmd.Flags().StringVarP(&dsdCaptureFilePath, "path", "p", "", "Directory path to write the capture to.")
	dogstatsdCaptureCmd.Flags().BoolVarP(&dsdCaptureCompressed, "compressed", "z", true, "Should capture be zstd compressed.")

	// shut up grpc client!
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard))

	return []*cobra.Command{dogstatsdCaptureCmd}
}

func dogstatsdCapture() error {
	fmt.Printf("Starting a dogstatsd traffic capture session...\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	cli := pb.NewAgentSecureClient(conn)

	resp, err := cli.DogstatsdCaptureTrigger(ctx, &pb.CaptureTriggerRequest{
		Duration:   dsdCaptureDuration.String(),
		Path:       dsdCaptureFilePath,
		Compressed: dsdCaptureCompressed,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Capture started, capture file being written to: %s\n", resp.Path)

	return nil
}
