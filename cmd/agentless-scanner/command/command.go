// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package command implements the agentless-scanner command.
package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/subcommands/aws"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/subcommands/azure"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/subcommands/local"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	_ "modernc.org/sqlite" // sqlite driver, used by github.com/knqyf263/go-rpmdb

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

const (
	defaultWorkersCount = 15
	defaultScannersMax  = 8 // max number of child-process scanners spawned by a worker in parallel
)

var defaultActions = []string{
	string(types.ScanActionVulnsHostOS),
	string(types.ScanActionVulnsContainersOS),
}

type runParams struct {
	cloudProvider string
	pidfilePath   string
	workers       int
	scannersMax   int
}

type runScannerParams struct {
	sock string
}

// RootCommand returns the root commands
func RootCommand() *cobra.Command {
	parent := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
	}

	pflags := parent.PersistentFlags()
	pflags.StringVarP(&common.GlobalParams.ConfigFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&common.GlobalParams.DiskMode, "disk-mode", string(types.DiskModeNBDAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", types.DiskModeNoAttach, types.DiskModeVolumeAttach, types.DiskModeNBDAttach))
	pflags.BoolVar(&common.GlobalParams.NoForkScanners, "no-fork-scanners", false, "disable spawning a dedicated process for launching scanners")
	pflags.StringSliceVar(&common.GlobalParams.DefaultActions, "actions", defaultActions, "disable spawning a dedicated process for launching scanners")

	{
		var params runParams
		cmd := &cobra.Command{
			Use:   "run",
			Short: "Runs the agentless-scanner",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					runCmd,
					common.Bundle(),
					fx.Supply(&params),
				)
			},
		}
		cmd.Flags().StringVarP(&params.pidfilePath, "pidfile", "p", "", "path to the pidfile")
		cmd.Flags().StringVar(&params.cloudProvider, "cloud-provider", "auto", fmt.Sprintf("cloud provider to use (auto, %q or %q)", types.CloudProviderAWS, types.CloudProviderNone))
		cmd.Flags().IntVar(&params.workers, "workers", defaultWorkersCount, "number of snapshots running in parallel")
		cmd.Flags().IntVar(&params.scannersMax, "scanners-max", defaultScannersMax, "maximum number of scanner processes in parallel")
		parent.AddCommand(cmd)
	}

	{
		var params runScannerParams
		cmd := &cobra.Command{
			Use:   "run-scanner",
			Short: "Runs a scanner (fork/exec model)",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					runScannerCmd,
					common.Bundle(),
					fx.Supply(&params),
				)
			},
		}
		cmd.Flags().StringVar(&params.sock, "sock", "", "path to unix socket for IPC")
		_ = cmd.MarkFlagRequired("sock")
		parent.AddCommand(cmd)
	}

	parent.AddCommand(aws.Commands()...)
	parent.AddCommand(azure.Commands()...)
	parent.AddCommand(local.Commands()...)

	return parent
}

func runCmd(_ complog.Component, sc *types.ScannerConfig, params *runParams, evp eventplatform.Component) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)

	if params.workers <= 0 {
		return fmt.Errorf("workers must be greater than 0")
	}
	if params.scannersMax <= 0 {
		return fmt.Errorf("scanners-max must be greater than 0")
	}
	provider, err := detectCloudProvider(params.cloudProvider)
	if err != nil {
		return fmt.Errorf("could not detect cloud provider: %w", err)
	}

	if params.pidfilePath != "" {
		err := pidfile.WritePID(params.pidfilePath)
		if err != nil {
			return fmt.Errorf("could not write PID file, exiting: %w", err)
		}
		defer os.Remove(params.pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), params.pidfilePath)
	}

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	scannerID := types.NewScannerID(provider, hostname)
	scanner, err := runner.New(*sc, runner.Options{
		ScannerID:      scannerID,
		DdEnv:          sc.Env,
		Workers:        params.workers,
		ScannersMax:    params.scannersMax,
		PrintResults:   false,
		Statsd:         statsd,
		EventForwarder: evp,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(); err != nil {
		log.Error(err)
	}
	if err := scanner.SubscribeRemoteConfig(ctx); err != nil {
		return fmt.Errorf("could not accept configs from Remote Config: %w", err)
	}
	scanner.Start(ctx)
	return nil
}

func runScannerCmd(_ complog.Component, sc *types.ScannerConfig, params *runScannerParams) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)
	var opts types.ScannerOptions

	conn, err := net.Dial("unix", params.sock)
	if err != nil {
		return err
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	if err := dec.Decode(&opts); err != nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	result := runner.LaunchScannerInSameProcess(ctx, statsd, sc, opts)
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
	if err := enc.Encode(result); err != nil {
		return err
	}
	return nil
}

func detectCloudProvider(s string) (types.CloudProvider, error) {
	if s == "auto" {
		// Amazon EC2 T4g
		boardVendor, errBoardVendor := os.ReadFile("/sys/devices/virtual/dmi/id/board_vendor")
		if errBoardVendor == nil && bytes.Equal(boardVendor, []byte("Amazon EC2\n")) {
			return types.CloudProviderAWS, nil
		}
		// Amazon EC2 M4
		productVersion, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_version")
		if err == nil && bytes.Contains(productVersion, []byte("amazon")) {
			return types.CloudProviderAWS, nil
		}

		// Azure
		boardName, errBoardName := os.ReadFile("/sys/devices/virtual/dmi/id/board_name")
		if errBoardVendor == nil && bytes.Equal(boardVendor, []byte("Microsoft Corporation\n")) &&
			errBoardName == nil && bytes.Equal(boardName, []byte("Virtual Machine\n")) {
			// This detects Hyper-V VMs. To be sure we are running on Azure, we would need to check the IMDS.
			return types.CloudProviderAzure, nil
		}

		return "", fmt.Errorf("could not detect cloud provider automatically, please specify one using --cloud-provider flag")
	}
	return types.ParseCloudProvider(s)
}
