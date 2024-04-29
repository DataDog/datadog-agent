// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package expvarsimpl initializes the expvar server of the process agent.
package expvarsimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/process/expvars"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newExpvarServer))
}

var _ expvars.Component = (*expvarServer)(nil)

type expvarServer *http.Server

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	HostInfo       hostinfo.Component

	Log       log.Component
	Telemetry telemetry.Component
}

func newExpvarServer(deps dependencies) (expvars.Component, error) {
	// Initialize status
	err := InitProcessStatus(deps.Config, deps.SysProbeConfig, deps.HostInfo, deps.Log, deps.Telemetry)
	if err != nil {
		_ = deps.Log.Critical("Failed to initialize status server:", err)
		return struct{}{}, err
	}

	expvarPort := getExpvarPort(deps)
	expvarServer := &http.Server{Addr: fmt.Sprintf("localhost:%d", expvarPort), Handler: http.DefaultServeMux}

	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				err := expvarServer.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					_ = deps.Log.Error(err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := expvarServer.Shutdown(ctx)
			if err != nil {
				_ = deps.Log.Error("Failed to properly shutdown expvar server:", err)
			}
			return nil
		},
	})
	return expvarServer, nil
}

func getExpvarPort(deps dependencies) int {
	expVarPort := deps.Config.GetInt("process_config.expvar_port")
	if expVarPort <= 0 {
		_ = deps.Log.Warnf("Invalid process_config.expvar_port -- %d, using default port %d", expVarPort, ddconfig.DefaultProcessExpVarPort)
		expVarPort = ddconfig.DefaultProcessExpVarPort
	}
	return expVarPort
}

// InitProcessStatus initializes the data required for the process status
func InitProcessStatus(
	Config config.Component,
	SysProbeConfig sysprobeconfig.Component,
	HostInfo hostinfo.Component,
	Log log.Component,
	Telemetry telemetry.Component,
) error {
	// update docker socket path in info
	dockerSock, err := util.GetDockerSocketPath()
	if err != nil {
		Log.Debugf("Docker is not available on this host")
	}
	status.UpdateDockerSocket(dockerSock)

	// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
	_, processModuleEnabled := SysProbeConfig.SysProbeObject().EnabledModules[sysconfig.ProcessModule]
	eps, err := endpoint.GetAPIEndpoints(Config)
	if err != nil {
		_ = Log.Criticalf("Failed to initialize Api Endpoints: %s", err.Error())
	}
	languageDetectionEnabled := Config.GetBool("language_detection.enabled")
	status.InitExpvars(Config, Telemetry, HostInfo.Object().HostName, processModuleEnabled, languageDetectionEnabled, eps)
	return nil
}
