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
	"io"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
)

const (
	defaultCaptureDuration = time.Duration(1) * time.Minute
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	dsdCaptureDuration   time.Duration
	dsdCaptureFilePath   string
	dsdCaptureCompressed bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	dogstatsdCaptureCmd := &cobra.Command{
		Use:   "dogstatsd-capture",
		Short: "Start a dogstatsd UDS traffic capture",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dogstatsdCapture,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}

	dogstatsdCaptureCmd.Flags().DurationVarP(&cliParams.dsdCaptureDuration, "duration", "d", defaultCaptureDuration, "Duration traffic capture should span.")
	dogstatsdCaptureCmd.Flags().StringVarP(&cliParams.dsdCaptureFilePath, "path", "p", "", "Directory path to write the capture to.")
	dogstatsdCaptureCmd.Flags().BoolVarP(&cliParams.dsdCaptureCompressed, "compressed", "z", true, "Should capture be zstd compressed.")

	// shut up grpc client!
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))

	return []*cobra.Command{dogstatsdCaptureCmd}
}

func dogstatsdCapture(log log.Component, config config.Component, cliParams *cliParams) error {
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
		fmt.Sprintf(":%v", pkgconfig.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	cli := pb.NewAgentSecureClient(conn)

	resp, err := cli.DogstatsdCaptureTrigger(ctx, &pb.CaptureTriggerRequest{
		Duration:   cliParams.dsdCaptureDuration.String(),
		Path:       cliParams.dsdCaptureFilePath,
		Compressed: cliParams.dsdCaptureCompressed,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Capture started, capture file being written to: %s\n", resp.Path)

	return nil
}
