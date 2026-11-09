// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package controllerimpl implements the DatadogInstrumentation controller component.
package controllerimpl

import (
	"context"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	controllerdef "github.com/DataDog/datadog-agent/comp/instrumentation/controller/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

// Requires defines dependencies shared across controller starts.
type Requires struct {
	Telemetry telemetry.Component
	Handlers  []instrumentation.Handler
}

// Provides exposes the controller component.
type Provides struct {
	Comp controllerdef.Component
}

type component struct {
	telemetry telemetry.Component
	handlers  []instrumentation.Handler
}

// NewComponent constructs the controller without contacting Kubernetes.
func NewComponent(reqs Requires) Provides {
	return Provides{Comp: &component{telemetry: reqs.Telemetry, handlers: reqs.Handlers}}
}

func (c *component) Start(ctx context.Context, params controllerdef.Params) error {
	controller, err := newController(params.StatusClient, params.InformerFactory, c.handlers, params.IsLeader, c.telemetry)
	if err != nil {
		return err
	}
	go controller.Run(ctx)
	return nil
}
