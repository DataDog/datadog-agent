// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package expvarserverimpl contains the implementation of the expVar server component.
package expvarserverimpl

import (
	"context"
	"errors"
	"net/http"

	expvarserver "github.com/DataDog/datadog-agent/comp/agent/expvarserver/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the expvarserver component.
type Requires struct {
	LC     compdef.Lifecycle
	Config config.Component
	Log    log.Component
}

// Provides defines the output of the expvarserver component.
type Provides struct {
	Comp expvarserver.Component
}

// NewComponent creates a new expvarserver component.
func NewComponent(reqs Requires) Provides {
	expvarPort := reqs.Config.GetString("expvar_port")
	var expvarServer *http.Server
	reqs.LC.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			expvarServer = &http.Server{
				Addr:    "127.0.0.1:" + expvarPort,
				Handler: http.DefaultServeMux,
			}
			go func() {
				if err := expvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					reqs.Log.Errorf("Error creating expvar server on %v: %v", expvarServer.Addr, err)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if err := expvarServer.Shutdown(context.Background()); err != nil {
				reqs.Log.Errorf("Error shutting down expvar server: %v", err)
			}
			return nil
		},
	})
	return Provides{Comp: struct{}{}}
}
