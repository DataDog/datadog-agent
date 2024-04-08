// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"go.uber.org/fx"
)

// GlobalParams holds the global flags from the root cmd
type GlobalParams struct {
	DiskMode       string
	DefaultActions []string
	NoForkScanners bool
	ConfigFilePath string
}

// ConfigProvider returns the scanner configuration
func ConfigProvider(globalParams *GlobalParams) func (c compconfig.Component) (*types.ScannerConfig, error) {
	return func(c compconfig.Component) (*types.ScannerConfig, error) {
		defaultRolesMapping, err := types.ParseRolesMapping(c.GetStringSlice("agentless_scanner.default_roles"))
		if err != nil {
			return nil, fmt.Errorf("could not parse default roles mapping: %w", err)
		}
		diskMode, err := types.ParseDiskMode(globalParams.DiskMode)
		if err != nil {
			return nil, fmt.Errorf("could not parse disk mode: %w", err)
		}
		defaultActions, err := types.ParseScanActions(globalParams.DefaultActions)
		if err != nil {
			return nil, fmt.Errorf("could not parse default actions: %w", err)
		}
		return &types.ScannerConfig{
			Env:                 c.GetString("env"),
			DogstatsdHost:       pkgconfig.GetBindHost(),
			DogstatsdPort:       c.GetInt("dogstatsd_port"),
			DefaultRolesMapping: defaultRolesMapping,
			DefaultActions:      defaultActions,
			NoForkScanners:      globalParams.NoForkScanners,
			DiskMode:            diskMode,
			AWSRegion:           c.GetString("agentless_scanner.aws_region"),
			AWSEC2Rate:          c.GetFloat64("agentless_scanner.limits.aws_ec2_rate"),
			AWSEBSListBlockRate: c.GetFloat64("agentless_scanner.limits.aws_ebs_list_block_rate"),
			AWSEBSGetBlockRate:  c.GetFloat64("agentless_scanner.limits.aws_ebs_get_block_rate"),
			AWSDefaultRate:      c.GetFloat64("agentless_scanner.limits.aws_default_rate"),
			AzureClientID:       c.GetString("agentless_scanner.azure_client_id"),
		}, nil
	}
}

// Bundle returns the fx.Option for the agentless-scanner
func Bundle(globalParams *GlobalParams) fx.Option {
	return fx.Options(
		fx.Supply(core.BundleParams{
			ConfigParams: compconfig.NewAgentParams(globalParams.ConfigFilePath),
			SecretParams: secrets.NewEnabledParams(),
			LogParams:    logimpl.ForDaemon(runner.LoggerName, "log_file", pkgconfigsetup.DefaultAgentlessScannerLogFile),
		}),
		core.Bundle(),
		eventplatformimpl.Module(),
		fx.Supply(eventplatformimpl.NewDefaultParams()),
		eventplatformreceiverimpl.Module(),
	)
}

// InitStatsd initializes the dogstatsd client
func InitStatsd(sc types.ScannerConfig) ddogstatsd.ClientInterface {
	statsdAddr := fmt.Sprintf("%s:%d", sc.DogstatsdHost, sc.DogstatsdPort)
	statsd, err := ddogstatsd.New(statsdAddr)
	if err != nil {
		log.Warnf("could not init dogstatsd client: %s", err)
		return &ddogstatsd.NoOpClient{}
	}
	return statsd
}

// CtxTerminated cancels the context on termination signal
func CtxTerminated() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}

// TryGetHostname returns the hostname when possible
func TryGetHostname(ctx context.Context) string {
	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		return "unknown"
	}
	return hostname
}
