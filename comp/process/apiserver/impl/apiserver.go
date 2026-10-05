// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiserverimpl initializes the api server that powers many subcommands.
package apiserverimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

var _ apiserver.Component = (*apiserverImpl)(nil)

type apiserverImpl struct {
	server *http.Server
}

type dependencies struct {
	compdef.In

	Lc           compdef.Lifecycle
	Config       config.Component
	Log          logComp.Component
	IPC          ipc.Component
	WorkloadMeta workloadmeta.Component
	Status       status.Component
	Settings     settings.Component
	Tagger       tagger.Component
	Secrets      secrets.Component
	Telemetry    telemetry.Component
}

// NewComponent creates a new apiserver component.
func NewComponent(deps dependencies) (apiserver.Component, error) {
	r := http.NewServeMux()
	api.SetupAPIServerHandlers(api.APIServerDeps{
		Config:       deps.Config,
		Log:          deps.Log,
		WorkloadMeta: deps.WorkloadMeta,
		Status:       deps.Status,
		Settings:     deps.Settings,
		Tagger:       deps.Tagger,
		Secrets:      deps.Secrets,
	}, r) // Set up routes

	addr, err := getProcessAPIAddressPort(deps.Config, deps.Log)
	if err != nil {
		return nil, err
	}
	deps.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(deps.Config.GetInt("server_timeout")) * time.Second

	// Instrument the API server with the same request telemetry as the core agent,
	// tagging requests with the authentication mode (mTLS vs token) used by clients.
	handler := deps.IPC.HTTPMiddleware(r)
	if tmf := newTelemetryMiddlewareFactory(deps); tmf != nil {
		handler = tmf.Middleware("process_api")(handler)
	}

	s := &apiserverImpl{
		server: &http.Server{
			Handler:      handler,
			Addr:         addr,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
			IdleTimeout:  timeout,
		},
	}

	deps.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			go func() {
				tlsListener := tls.NewListener(ln, deps.IPC.GetTLSServerConfig())
				err = s.server.Serve(tlsListener)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					_ = deps.Log.Error(err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := s.server.Shutdown(ctx)
			if err != nil {
				_ = deps.Log.Error("Failed to properly shutdown api server:", err)
			}

			return nil
		},
	})

	return s, nil
}

const defaultProcessCmdPort = 6162

// newTelemetryMiddlewareFactory builds the observability telemetry middleware for the
// process agent API server. It returns nil (and keeps the server uninstrumented) if the
// IPC certificate cannot be loaded, as telemetry must never prevent the API from serving.
func newTelemetryMiddlewareFactory(deps dependencies) observability.TelemetryMiddlewareFactory {
	authTagGetter, err := observability.AuthTagGetter(deps.IPC.GetTLSServerConfig())
	if err != nil {
		deps.Log.Warnf("Unable to build the API telemetry auth tag getter: %v", err)
		return nil
	}
	return observability.NewTelemetryMiddlewareFactory(deps.Telemetry, authTagGetter)
}

// getProcessAPIAddressPort returns the API address:port for the process agent.
func getProcessAPIAddressPort(cfg config.Component, log logComp.Component) (string, error) {
	var key string
	if cfg.IsConfigured("ipc_address") {
		log.Warn("ipc_address is deprecated, use cmd_host instead")
		key = "ipc_address"
	} else {
		key = "cmd_host"
	}
	address, err := system.IsLocalAddress(cfg.GetString(key))
	if err != nil {
		return "", fmt.Errorf("%s: %s", key, err)
	}
	port := cfg.GetInt("process_config.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid process_config.cmd_port -- %d, using default port %d", port, defaultProcessCmdPort)
		port = defaultProcessCmdPort
	}
	return net.JoinHostPort(address, strconv.Itoa(port)), nil
}
