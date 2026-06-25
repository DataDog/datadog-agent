// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package executor implements the hidden 'private-action-runner executor run'
// subcommand. It is launched by the orchestrator process when running in
// binary executor mode: the orchestrator re-execs the current binary into
// this subcommand to host the action surface in a child process.
package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformfx "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	rcclientfx "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/fx"
	rcservicefx "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	parexecutor "github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	executorserver "github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor/server"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor/task-verifier"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/adapters/config"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
	socketPath string
}

// Commands returns the hidden 'executor run' subcommand for the
// 'private-action-runner' command. It is spawned by the orchestrator when
// running in binary executor mode; operators do not invoke it directly.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{GlobalParams: globalParams}

	executorCmd := &cobra.Command{
		Use:    "executor",
		Short:  "Internal: PAR action-executor subcommand group",
		Long:   "The 'executor' subcommand group is spawned by the PAR orchestrator when running in binary executor mode and is not intended for direct use.",
		Hidden: true,
	}

	runCmd := &cobra.Command{
		Use:    "run",
		Short:  "Internal: run as a PAR action executor child process",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if params.socketPath == "" {
				return errors.New("executor run requires --socket-path")
			}
			return runChild(context.Background(), params)
		},
	}
	runCmd.Flags().StringVar(&params.socketPath, "socket-path", "", "Local socket the executor listens on")

	executorCmd.AddCommand(runCmd)
	return []*cobra.Command{executorCmd}
}

type executorDeps struct {
	fx.In
	Config        config.Component
	Log           log.Component
	IPC           ipc.Component
	RcClient      rcclient.Component
	Traceroute    traceroute.Component
	EventPlatform eventplatform.Component
}

func runChild(ctx context.Context, params *cliParams) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fxOptions := []fx.Option{
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(params.ConfFilePath, config.WithExtraConfFiles(params.ExtraConfFilePath)),
			LogParams:    log.ForDaemon(command.LoggerName, pkgconfigsetup.PARLogFile, defaultpaths.GetDefaultPrivateActionRunnerLogFile()),
		}),
		core.Bundle(core.WithSecrets()),
		remotehostnameimpl.Module(),
		ipcfx.ModuleReadWrite(),
		rcservicefx.Module(),
		rcclientfx.Module(),
		fx.Supply(rcclient.Params{AgentName: "private-action-runner-executor", AgentVersion: version.AgentVersion}),
		remotetraceroute.Module(),
		logscompressionfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformfx.Module(eventplatform.NewDefaultParams()),
		fx.Invoke(func(deps executorDeps) error {
			return serve(ctx, params, deps)
		}),
	}

	if err := fxutil.Run(fxOptions...); err != nil {
		return fmt.Errorf("executor child terminated: %w", err)
	}
	return nil
}

func serve(ctx context.Context, params *cliParams, deps executorDeps) error {
	cfg, err := parconfig.FromDDConfig(deps.Config)
	if err != nil {
		return fmt.Errorf("read PAR config: %w", err)
	}
	keysManager := taskverifier.NewKeyManager(pkgrcclient.NewAdapter(deps.RcClient))
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	handler := parexecutor.NewTaskHandler(cfg, keysManager, taskVerifier, deps.Traceroute, deps.EventPlatform, deps.IPC.GetClient())

	server := executorserver.NewServer(handler, deps.IPC.GetAuthToken())
	listener, err := parexecutor.Listen(params.socketPath)
	if err != nil {
		return fmt.Errorf("listen on executor socket: %w", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx, listener)
	}()

	select {
	case <-ctx.Done():
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = server.Stop(stopCtx)
		<-serveErr
		return nil
	case err := <-serveErr:
		return err
	}
}
