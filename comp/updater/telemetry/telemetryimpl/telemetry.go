// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetryimpl provides the telemetry component implementation.
package telemetryimpl

import (
	"net/http"

	"go.uber.org/fx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	fleettelemetry "github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
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
	telemetry, err := fleettelemetry.NewTelemetry(client, utils.SanitizeAPIKey(deps.Config.GetString("api_key")), deps.Config.GetString("site"), "datadog-installer-daemon",
		fleettelemetry.WithSamplingRules(
			tracer.NameServiceRule("cdn.*", "datadog-installer-daemon", 0.1),
			tracer.NameServiceRule("*garbage_collect*", "datadog-installer-daemon", 0.05),
			tracer.NameServiceRule("HTTPClient.*", "datadog-installer-daemon", 0.05),
		),
	)
	if err != nil {
		return nil, err
	}
	deps.Lc.Append(fx.Hook{OnStart: telemetry.Start, OnStop: telemetry.Stop})
	return telemetry, nil
}
