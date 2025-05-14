// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

// Package dogstatsdcapture implements 'agent dogstasd-capture'.
package dogstatsdcapture

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(dogstatsdCapture,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
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

//nolint:revive // TODO(AML) Fix revive linter
func dogstatsdCapture(_ log.Component, config config.Component, cliParams *cliParams, ipc ipc.Component) error {
	fmt.Printf("Starting a dogstatsd traffic capture session...\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", ipc.GetAuthToken())}, // TODO IPC: replace with GRPC Client
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		ctx,
		fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")),
		grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())),
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
