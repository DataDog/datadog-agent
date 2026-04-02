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
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
}

// NewComponent creates a new apiserver component.
//
//nolint:revive // TODO(PROC) Fix revive linter
func NewComponent(deps dependencies) apiserver.Component {
	r := mux.NewRouter()
	r.Use(deps.IPC.HTTPMiddleware)
	api.SetupAPIServerHandlers(api.APIServerDeps{
		Config:       deps.Config,
		Log:          deps.Log,
		WorkloadMeta: deps.WorkloadMeta,
		Status:       deps.Status,
		Settings:     deps.Settings,
		Tagger:       deps.Tagger,
		Secrets:      deps.Secrets,
	}, r) // Set up routes

	addr, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	deps.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(pkgconfigsetup.Datadog().GetInt("server_timeout")) * time.Second

	s := &apiserverImpl{
		server: &http.Server{
			Handler:      r,
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

	return s
}
