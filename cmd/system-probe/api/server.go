// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"runtime/pprof"
	"strings"

	gorilla "github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxGRPCServerMessage = 100 * 1024 * 1024

// StartServer starts the HTTP and gRPC servers for the system-probe, which registers endpoints from all enabled modules.
func StartServer(cfg *config.Config, telemetry telemetry.Component) error {
	conn, err := net.NewListener(cfg.SocketAddress)
	if err != nil {
		return fmt.Errorf("error creating IPC socket: %s", err)
	}

	var grpcServer *grpc.Server
	var srv *http.Server

	mux := gorilla.NewRouter()
	if cfg.GRPCServerEnabled {
		grpcServer = grpc.NewServer(
			grpc.MaxRecvMsgSize(maxGRPCServerMessage),
			grpc.MaxSendMsgSize(maxGRPCServerMessage),
			grpc.StatsHandler(&pprofGRPCStatsHandler{}),
		)
	}

	err = module.Register(cfg, mux, grpcServer, modules.All)
	if err != nil {
		return fmt.Errorf("failed to create system probe: %s", err)
	}

	// Register stats endpoint
	mux.HandleFunc("/debug/stats", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		utils.WriteAsJSON(w, module.GetStats())
	}))

	setupConfigHandlers(mux)

	// Module-restart handler
	mux.HandleFunc("/module-restart/{module-name}", restartModuleHandler).Methods("POST")

	mux.Handle("/debug/vars", http.DefaultServeMux)
	mux.Handle("/telemetry", telemetry.Handler())

	if cfg.GRPCServerEnabled {
		srv = grpcutil.NewMuxedGRPCServer(
			cfg.SocketAddress,
			nil,
			grpcServer,
			mux,
		)
	} else {
		srv = &http.Server{
			Handler: mux,
		}
	}

	go func() {
		err = srv.Serve(conn.GetListener())
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("error creating HTTP server: %s", err)
		}
	}()

	return nil
}

func init() {
	expvar.Publish("modules", expvar.Func(func() interface{} {
		return module.GetStats()
	}))
}

type pprofGRPCStatsHandler struct{}

func (p *pprofGRPCStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	parts := strings.Split(info.FullMethodName, "/")
	if len(parts) >= 1 {
		moduleName := module.NameFromGRPCServiceName(parts[0])
		if moduleName != "" {
			return pprof.WithLabels(ctx, pprof.Labels("module", moduleName))
		}
	}
	return ctx
}

func (p *pprofGRPCStatsHandler) HandleRPC(_ context.Context, _ stats.RPCStats) {
	// intentionally empty
}

func (p *pprofGRPCStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (p *pprofGRPCStatsHandler) HandleConn(_ context.Context, _ stats.ConnStats) {
	// intentionally empty
}
