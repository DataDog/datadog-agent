// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package expvarserverimpl contains the implementation of the expVar server component.
package expvarserverimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newExpvarServer),
	)
}

type dependencies struct {
	fx.In
	Lc     fx.Lifecycle
	Config config.Component
	Log    log.Component
}

func newExpvarServer(deps dependencies) expvarserver.Component {
	expvarPort := deps.Config.GetString("expvar_port")
	var expvarServer *http.Server
	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			expvarServer = &http.Server{
				Addr:    fmt.Sprintf("127.0.0.1:%s", expvarPort),
				Handler: http.DefaultServeMux,
			}
			go func() {
				if err := expvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					deps.Log.Errorf("Error creating expvar server on %v: %v", expvarServer.Addr, err)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if err := expvarServer.Shutdown(context.Background()); err != nil {
				deps.Log.Errorf("Error shutting down expvar server: %v", err)
			}
			return nil
		}})

	return struct{}{}
}
