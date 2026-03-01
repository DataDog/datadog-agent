// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiserverimpl implements the apiserver component.
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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var _ apiserver.Component = (*apiserverImpl)(nil)

type apiserverImpl struct {
	server *http.Server
}

// Requires defines the dependencies for the apiserver component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log logComp.Component

	IPC ipc.Component

	APIServerDeps api.APIServerDeps
}

// Provides defines the output of the apiserver component.
type Provides struct {
	Comp apiserver.Component
}

//nolint:revive // TODO(PROC) Fix revive linter
func NewComponent(reqs Requires) (Provides, error) {
	r := mux.NewRouter()
	r.Use(reqs.IPC.HTTPMiddleware)
	api.SetupAPIServerHandlers(reqs.APIServerDeps, r)

	addr, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return Provides{}, err
	}
	reqs.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(pkgconfigsetup.Datadog().GetInt("server_timeout")) * time.Second

	as := &apiserverImpl{
		server: &http.Server{
			Handler:      r,
			Addr:         addr,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
			IdleTimeout:  timeout,
		},
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			go func() {
				tlsListener := tls.NewListener(ln, reqs.IPC.GetTLSServerConfig())
				err = as.server.Serve(tlsListener)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					_ = reqs.Log.Error(err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := as.server.Shutdown(ctx)
			if err != nil {
				_ = reqs.Log.Error("Failed to properly shutdown api server:", err)
			}

			return nil
		},
	})

	return Provides{Comp: as}, nil
}
