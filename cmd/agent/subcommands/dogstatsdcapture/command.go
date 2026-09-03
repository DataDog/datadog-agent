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
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
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

func findADPBinary() (string, error) {
	candidates := []string{
		filepath.Join(defaultpaths.GetEmbeddedBinPath(), "agent-data-plane"),
		filepath.Join(defaultpaths.GetInstallPath(), "bin", "agent", "agent-data-plane"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("agent-data-plane binary not found (tried: %v)", candidates)
}

func captureViaADP(cliParams *cliParams) error {
	adpBin, err := findADPBinary()
	if err != nil {
		return fmt.Errorf("cannot delegate dogstatsd-capture to agent-data-plane: %w", err)
	}

	args := []string{"dogstatsd", "capture", "--duration", cliParams.dsdCaptureDuration.String()}
	if cliParams.dsdCaptureFilePath != "" {
		args = append(args, "--path", cliParams.dsdCaptureFilePath)
	}
	if !cliParams.dsdCaptureCompressed {
		args = append(args, "--compressed", "false")
	}

	cmd := exec.Command(adpBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dogstatsdCapture(_ log.Component, _ config.Component, cliParams *cliParams, ipc ipc.Component) error {
	if pkgconfigsetup.Datadog().GetBool("data_plane.enabled") && pkgconfigsetup.Datadog().GetBool("data_plane.dogstatsd.enabled") {
		return captureViaADP(cliParams)
	}
	fmt.Printf("Starting a dogstatsd traffic capture session...\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	md := metadata.MD{
		"authorization": []string{"Bearer " + ipc.GetAuthToken()}, // TODO IPC: replace with GRPC Client
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
