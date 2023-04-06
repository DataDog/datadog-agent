// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

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
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var _ Component = (*expvarServer)(nil)

type expvarServer *http.Server

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	HostInfo       hostinfo.Component

	Log log.Component
}

func newExpvarServer(deps dependencies) (Component, error) {
	// Initialize status
	err := initStatus(deps)
	if err != nil {
		_ = deps.Log.Critical("Failed to initialize status server:", err)
		return nil, err
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
		OnStop: expvarServer.Shutdown,
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

func initStatus(deps dependencies) error {
	// update docker socket path in info
	dockerSock, err := util.GetDockerSocketPath()
	if err != nil {
		deps.Log.Debugf("Docker is not available on this host")
	}
	status.UpdateDockerSocket(dockerSock)

	// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
	_, processModuleEnabled := deps.SysProbeConfig.Object().EnabledModules[sysconfig.ProcessModule]
	eps, err := runner.GetAPIEndpoints()
	if err != nil {
		_ = deps.Log.Criticalf("Failed to initialize Api Endpoints: %s", err.Error())
	}
	status.InitExpvars(deps.Config, deps.HostInfo.Object().HostName, processModuleEnabled, eps)
	return nil
}
