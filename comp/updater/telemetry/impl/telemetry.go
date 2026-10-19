// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetryimpl provides the telemetry component implementation.
package telemetryimpl

import (
	"context"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	telemetry "github.com/DataDog/datadog-agent/comp/updater/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	fleettelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Requires defines the dependencies for the telemetry component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Config config.Component
}

// Provides defines the output of the telemetry component.
type Provides struct {
	Comp telemetry.Component
}

// daemonTelemetryStopFlushTimeout bounds the telemetry flush on shutdown so an unreachable
// intake does not delay stopping the installer service.
const daemonTelemetryStopFlushTimeout = 2 * time.Second

// NewComponent creates a new telemetry component.
func NewComponent(reqs Requires) (Provides, error) {
	client := &http.Client{
		Transport: httputils.CreateHTTPTransport(reqs.Config),
	}
	opts := []fleettelemetry.Option{fleettelemetry.WithStopFlushTimeout(daemonTelemetryStopFlushTimeout)}
	if url := reqs.Config.GetString("apm_config.telemetry.dd_url"); url != "" {
		opts = append(opts, fleettelemetry.WithEndpointURL(url))
	}
	if !reqs.Config.GetBool("apm_config.telemetry.enabled") {
		opts = append(opts, fleettelemetry.WithDisabled())
	}
	tel := fleettelemetry.NewTelemetry(client, utils.SanitizeAPIKey(reqs.Config.GetString("api_key")), reqs.Config.GetString("site"), "datadog-installer-daemon", opts...)
	reqs.Lifecycle.Append(compdef.Hook{OnStop: func(context.Context) error { tel.Stop(); return nil }})
	return Provides{Comp: tel}, nil
}
