// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ Component = (*apiserver)(nil)

type apiserver struct {
	server *http.Server
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log logComp.Component

	At authtoken.Component

	APIServerDeps api.APIServerDeps
}

//nolint:revive // TODO(PROC) Fix revive linter
func newApiServer(deps dependencies) Component {
	r := mux.NewRouter()
	r.Use(validateToken)
	api.SetupAPIServerHandlers(deps.APIServerDeps, r) // Set up routes

	addr, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	deps.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(pkgconfigsetup.Datadog().GetInt("server_timeout")) * time.Second

	apiserver := &apiserver{
		server: &http.Server{
			Handler:      r,
			Addr:         addr,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
			IdleTimeout:  timeout,
		},
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			go func() {
				tlsListener := tls.NewListener(ln, deps.At.GetTLSServerConfig())
				err = apiserver.server.Serve(tlsListener)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					_ = deps.Log.Error(err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := apiserver.server.Shutdown(ctx)
			if err != nil {
				_ = deps.Log.Error("Failed to properly shutdown api server:", err)
			}

			return nil
		},
	})

	return apiserver
}

func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			log.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
