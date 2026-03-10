// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package telemetryimpl provides the telemetry component implementation.
package telemetryimpl

import (
	"context"
	"net/http"

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

// NewComponent creates a new telemetry component.
func NewComponent(reqs Requires) (Provides, error) {
	client := &http.Client{
		Transport: httputils.CreateHTTPTransport(reqs.Config),
	}
	tel := fleettelemetry.NewTelemetry(client, utils.SanitizeAPIKey(reqs.Config.GetString("api_key")), reqs.Config.GetString("site"), "datadog-installer-daemon")
	reqs.Lifecycle.Append(compdef.Hook{OnStop: func(context.Context) error { tel.Stop(); return nil }})
	return Provides{Comp: tel}, nil
}
