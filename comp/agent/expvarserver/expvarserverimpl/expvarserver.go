// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package expvarserverimpl contains the implementation of the expVar server component.
package expvarserverimpl

import (
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newExpvarServer),
	)
}

type dependencies struct {
	fx.In
	Config    config.Component
	Telemetry telemetry.Component
}

func newExpvarServer(deps dependencies) expvarserver.Component {
	telemetryHandler := deps.Telemetry.Handler()
	expvarPort := deps.Config.GetString("expvar_port")
	http.Handle("/telemetry", telemetryHandler)
	go func() {
		common.ExpvarServer = &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%s", expvarPort),
			Handler: http.DefaultServeMux,
		}
		if err := common.ExpvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("Error creating expvar server on %v: %v", common.ExpvarServer.Addr, err)
		}
	}()

	return struct{}{}
}
