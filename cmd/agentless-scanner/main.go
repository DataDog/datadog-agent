// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements the agentless-scanner command.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/runner"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	// DataDog agent: config stuffs
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	_ "modernc.org/sqlite" // sqlite driver, used by github.com/knqyf263/go-rpmdb

	"github.com/spf13/cobra"
)

const (
	defaultWorkersCount = 15
	defaultScannersMax  = 8 // max number of child-process scanners spawned by a worker in parallel
)

var statsd *ddogstatsd.Client

var globalFlags struct {
	configFilePath string
	diskMode       types.DiskMode
	defaultActions []types.ScanAction
	noForkScanners bool
}

func runWithModules(run func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fxutil.OneShot(
			func(_ complog.Component, _ compconfig.Component) error {
				return run(cmd, args)
			},
			fx.Supply(compconfig.NewAgentParamsWithSecrets(globalFlags.configFilePath)),
			fx.Supply(complog.ForDaemon(runner.LoggerName, "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
			complog.Module,
			compconfig.Module,
		)
	}
}

func main() {
	flavor.SetFlavor(flavor.AgentlessScanner)

	signal.Ignore(syscall.SIGPIPE)

	cmd := rootCommand()
	cmd.SilenceErrors = true
	err := cmd.Execute()

	if err != nil {
		log.Flush()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}
	log.Flush()
	os.Exit(0)
}

func initStatsdClient() {
	statsdHost := pkgconfig.GetBindHost()
	statsdPort := pkgconfig.Datadog.GetInt("dogstatsd_port")
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	var err error
	statsd, err = ddogstatsd.New(statsdAddr)
	if err != nil {
		log.Warnf("could not init dogstatsd client: %s", err)
	}
}

func rootCommand() *cobra.Command {
	var flags struct {
		diskModeStr       string
		defaultActionsStr []string
	}
	cmd := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			initStatsdClient()
			diskMode, err := types.ParseDiskMode(flags.diskModeStr)
			if err != nil {
				return err
			}
			defaultActions, err := types.ParseScanActions(flags.defaultActionsStr)
			if err != nil {
				return err
			}
			globalFlags.diskMode = diskMode
			globalFlags.defaultActions = defaultActions
			return nil
		}),
	}

	cmd.AddCommand(runCommand())
	cmd.AddCommand(runScannerCommand())
	cmd.AddCommand(awsGroupCommand(cmd))
	cmd.AddCommand(localGroupCommand(cmd))

	defaultActions := []string{
		string(types.ScanActionVulnsHost),
		string(types.ScanActionVulnsContainers),
	}

	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&globalFlags.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&flags.diskModeStr, "disk-mode", string(types.DiskModeNBDAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", types.DiskModeNoAttach, types.DiskModeVolumeAttach, types.DiskModeNBDAttach))
	pflags.BoolVar(&globalFlags.noForkScanners, "no-fork-scanners", false, "disable spawning a dedicated process for launching scanners")
	pflags.StringSliceVar(&flags.defaultActionsStr, "actions", defaultActions, "disable spawning a dedicated process for launching scanners")
	return cmd
}

func runCommand() *cobra.Command {
	var flags struct {
		cloudProvider string
		pidfilePath   string
		workers       int
		scannersMax   int
	}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.workers <= 0 {
				return fmt.Errorf("workers must be greater than 0")
			}
			if flags.scannersMax <= 0 {
				return fmt.Errorf("scanners-max must be greater than 0")
			}
			provider, err := detectCloudProvider(flags.cloudProvider)
			if err != nil {
				return fmt.Errorf("could not detect cloud provider: %w", err)
			}
			return runCmd(provider, flags.pidfilePath, flags.workers, flags.scannersMax, globalFlags.defaultActions, globalFlags.noForkScanners)
		},
	}
	cmd.Flags().StringVarP(&flags.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	cmd.Flags().StringVar(&flags.cloudProvider, "cloud-provider", "auto", fmt.Sprintf("cloud provider to use (auto, %q or %q)", types.CloudProviderAWS, types.CloudProviderNone))
	cmd.Flags().IntVar(&flags.workers, "workers", defaultWorkersCount, "number of snapshots running in parallel")
	cmd.Flags().IntVar(&flags.scannersMax, "scanners-max", defaultScannersMax, "maximum number of scanner processes in parallel")
	return cmd
}

func runCmd(provider types.CloudProvider, pidfilePath string, workers, scannersMax int, defaultActions []types.ScanAction, noForkScanners bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return fmt.Errorf("could not write PID file, exiting: %w", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		CloudProvider:  provider,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        workers,
		ScannersMax:    scannersMax,
		PrintResults:   false,
		NoFork:         noForkScanners,
		DefaultActions: defaultActions,
		DefaultRoles:   getDefaultRolesMapping(provider),
		Statsd:         statsd,
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

func runScannerCommand() *cobra.Command {
	var sock string
	cmd := &cobra.Command{
		Use:   "run-scanner",
		Short: "Runs a scanner (fork/exec model)",
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			return runScannerCmd(sock)
		}),
	}
	cmd.Flags().StringVar(&sock, "sock", "", "path to unix socket for IPC")
	_ = cmd.MarkFlagRequired("sock")
	return cmd
}

func runScannerCmd(sock string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var opts types.ScannerOptions

	conn, err := net.Dial("unix", sock)
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

	result := runner.LaunchScannerInSameProcess(ctx, opts)
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
	if err := enc.Encode(result); err != nil {
		return err
	}
	return nil
}

func getDefaultRolesMapping(provider types.CloudProvider) types.RolesMapping {
	roles := pkgconfig.Datadog.GetStringSlice("agentless_scanner.default_roles")
	return types.ParseRolesMapping(provider, roles)
}

func detectCloudProvider(s string) (types.CloudProvider, error) {
	if s == "auto" {
		// Amazon EC2 T4g
		boardVendor, err := os.ReadFile("/sys/devices/virtual/dmi/id/board_vendor")
		if err == nil && bytes.Equal(boardVendor, []byte("Amazon EC2\n")) {
			return types.CloudProviderAWS, nil
		}
		// Amazon EC2 M4
		productVersion, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_version")
		if err == nil && bytes.Contains(productVersion, []byte("amazon")) {
			return types.CloudProviderAWS, nil
		}
		return "", fmt.Errorf("could not detect cloud provider automatically, please specify one using --cloud-provider flag")
	}
	return types.ParseCloudProvider(s)
}
