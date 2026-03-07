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
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	taggerComp "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const defaultProcessCmdPort = 6162

var _ apiserver.Component = (*apiserverImpl)(nil)

type apiserverImpl struct {
	server *http.Server
}

// Requires defines the dependencies for the apiserver component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Config       coreconfig.Component
	Log          logComp.Component
	WorkloadMeta workloadmeta.Component
	Status       status.Component
	Settings     settings.Component
	Tagger       taggerComp.Component
	Secrets      secrets.Component
	IPC          ipc.Component
}

// Provides defines the output of the apiserver component.
type Provides struct {
	Comp apiserver.Component
}

// getProcessAPIAddressPort returns the API endpoint of the process agent.
// This replicates pkgconfigsetup.GetProcessAPIAddressPort to avoid importing
// pkg/config/setup inside the comp/ folder.
func getProcessAPIAddressPort(cfg coreconfig.Component) (string, error) {
	var key string
	if cfg.IsSet("ipc_address") {
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

//nolint:revive // TODO(PROC) Fix revive linter
func NewComponent(reqs Requires) (Provides, error) {
	r := mux.NewRouter()
	r.Use(reqs.IPC.HTTPMiddleware)

	deps := api.APIServerDeps{
		Config:       reqs.Config,
		Log:          reqs.Log,
		WorkloadMeta: reqs.WorkloadMeta,
		Status:       reqs.Status,
		Settings:     reqs.Settings,
		Tagger:       reqs.Tagger,
		Secrets:      reqs.Secrets,
	}
	api.SetupAPIServerHandlers(deps, r)

	addr, err := getProcessAPIAddressPort(reqs.Config)
	if err != nil {
		return Provides{}, err
	}
	reqs.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(reqs.Config.GetInt("server_timeout")) * time.Second

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
