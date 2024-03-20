// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package command implements the agentless-scanner command.
package command

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

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/flags"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/subcommands/aws"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/subcommands/local"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	// DataDog agent: config stuffs
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

func initStatsdClient(sc types.ScannerConfig) ddogstatsd.Client {
	statsdHost := pkgconfig.GetBindHost()
	statsdPort := sc.DogstatsdPort
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	statsd, err := ddogstatsd.New(statsdAddr)
	if err != nil {
		log.Warnf("could not init dogstatsd client: %s", err)
	}
	return *statsd //nolint:govet
}

// RootCommand returns the root commands
func RootCommand() *cobra.Command {
	var statsd ddogstatsd.Client
	var sc types.ScannerConfig
	var evp eventplatform.Component
	var localFlags struct {
		diskModeStr       string
		defaultActionsStr []string
	}
	cmd := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, config compconfig.Component, eventForwarder eventplatform.Component) error {
					sc = getScannerConfig(config)
					evp = eventForwarder
					statsd = initStatsdClient(sc)
					diskMode, err := types.ParseDiskMode(localFlags.diskModeStr)
					if err != nil {
						return err
					}
					defaultActions, err := types.ParseScanActions(localFlags.defaultActionsStr)
					if err != nil {
						return err
					}
					flags.GlobalFlags.DiskMode = diskMode
					flags.GlobalFlags.DefaultActions = defaultActions
					return nil
				},
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewAgentParams(flags.GlobalFlags.ConfigFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    logimpl.ForDaemon(runner.LoggerName, "log_file", pkgconfigsetup.DefaultAgentlessScannerLogFile),
				}),
				core.Bundle(),
				eventplatformimpl.Module(),
				fx.Supply(eventplatformimpl.NewDefaultParams()),
				eventplatformreceiverimpl.Module(),
			)
		},
	}

	cmd.AddCommand(runCommand(&statsd, &sc, &evp))
	cmd.AddCommand(runScannerCommand(&statsd, &sc))
	cmd.AddCommand(aws.GroupCommand(cmd, &statsd, &sc, &evp))
	cmd.AddCommand(local.GroupCommand(cmd, &statsd, &sc))

	defaultActions := []string{
		string(types.ScanActionVulnsHostOS),
		string(types.ScanActionVulnsContainersOS),
	}

	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&flags.GlobalFlags.ConfigFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&localFlags.diskModeStr, "disk-mode", string(types.DiskModeNBDAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", types.DiskModeNoAttach, types.DiskModeVolumeAttach, types.DiskModeNBDAttach))
	pflags.BoolVar(&flags.GlobalFlags.NoForkScanners, "no-fork-scanners", false, "disable spawning a dedicated process for launching scanners")
	pflags.StringSliceVar(&localFlags.defaultActionsStr, "actions", defaultActions, "disable spawning a dedicated process for launching scanners")
	return cmd
}

func runCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
	var localFlags struct {
		cloudProvider string
		pidfilePath   string
		workers       int
		scannersMax   int
	}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			if localFlags.workers <= 0 {
				return fmt.Errorf("workers must be greater than 0")
			}
			if localFlags.scannersMax <= 0 {
				return fmt.Errorf("scanners-max must be greater than 0")
			}
			provider, err := detectCloudProvider(localFlags.cloudProvider)
			if err != nil {
				return fmt.Errorf("could not detect cloud provider: %w", err)
			}
			return runCmd(statsd, sc, evp, provider, localFlags.pidfilePath, localFlags.workers, localFlags.scannersMax, flags.GlobalFlags.DefaultActions, flags.GlobalFlags.NoForkScanners)
		},
	}
	cmd.Flags().StringVarP(&localFlags.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	cmd.Flags().StringVar(&localFlags.cloudProvider, "cloud-provider", "auto", fmt.Sprintf("cloud provider to use (auto, %q or %q)", types.CloudProviderAWS, types.CloudProviderNone))
	cmd.Flags().IntVar(&localFlags.workers, "workers", defaultWorkersCount, "number of snapshots running in parallel")
	cmd.Flags().IntVar(&localFlags.scannersMax, "scanners-max", defaultScannersMax, "maximum number of scanner processes in parallel")
	return cmd
}

func runCmd(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, provider types.CloudProvider, pidfilePath string, workers, scannersMax int, defaultActions []types.ScanAction, noForkScanners bool) error {
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

	scannerID := types.NewScannerID(provider, hostname)
	scanner, err := runner.New(runner.Options{
		ScannerID:      scannerID,
		DdEnv:          sc.Env,
		Workers:        workers,
		ScannersMax:    scannersMax,
		PrintResults:   false,
		NoFork:         noForkScanners,
		DefaultActions: defaultActions,
		DefaultRoles:   common.GetDefaultRolesMapping(sc, provider),
		Statsd:         statsd,
		EventForwarder: *evp,
		ScannerConfig:  sc,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(statsd, sc); err != nil {
		log.Error(err)
	}
	if err := scanner.SubscribeRemoteConfig(ctx); err != nil {
		return fmt.Errorf("could not accept configs from Remote Config: %w", err)
	}
	scanner.Start(ctx, statsd, sc)
	return nil
}

func runScannerCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig) *cobra.Command {
	var sock string
	cmd := &cobra.Command{
		Use:   "run-scanner",
		Short: "Runs a scanner (fork/exec model)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScannerCmd(statsd, sc, sock)
		},
	}
	cmd.Flags().StringVar(&sock, "sock", "", "path to unix socket for IPC")
	_ = cmd.MarkFlagRequired("sock")
	return cmd
}

func runScannerCmd(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, sock string) error {
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

func getScannerConfig(c compconfig.Component) types.ScannerConfig {
	return types.ScannerConfig{
		Env:                 c.GetString("env"),
		DogstatsdPort:       c.GetInt("dogstatsd_port"),
		DefaultRoles:        c.GetStringSlice("agentless_scanner.default_roles"),
		AWSRegion:           c.GetString("agentless_scanner.aws_region"),
		AWSEC2Rate:          c.GetFloat64("agentless_scanner.limits.aws_ec2_rate"),
		AWSEBSListBlockRate: c.GetFloat64("agentless_scanner.limits.aws_ebs_list_block_rate"),
		AWSEBSGetBlockRate:  c.GetFloat64("agentless_scanner.limits.aws_ebs_get_block_rate"),
		AWSDefaultRate:      c.GetFloat64("agentless_scanner.limits.aws_default_rate"),
	}
}
