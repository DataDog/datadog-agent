// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetryimpl provides the telemetry component implementation.
package telemetryimpl

import (
	"context"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	fleettelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Config config.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newTelemetry),
	)
}

func newTelemetry(deps dependencies) (telemetry.Component, error) {
	client := &http.Client{
		Transport: httputils.CreateHTTPTransport(deps.Config),
	}
	telemetry := fleettelemetry.NewTelemetry(client, utils.SanitizeAPIKey(deps.Config.GetString("api_key")), deps.Config.GetString("site"), "datadog-installer-daemon")
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error { telemetry.Stop(); return nil }})
	return telemetry, nil
}
