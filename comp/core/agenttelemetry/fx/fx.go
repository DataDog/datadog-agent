// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the agenttelemetry component
package fx

import (
	"context"
	"time"

	"go.uber.org/fx"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	agenttelemetryimpl "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	errortrackingpkg "github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			agenttelemetryimpl.NewComponent,
		),
		fxutil.ProvideOptional[agenttelemetry.Component](),
		fx.Invoke(installErrortrackingHandler),
	)
}

// installErrortrackingHandler is a no-op when the feature is disabled
// (agent_telemetry.errortracking.enabled or the parent agent_telemetry
// gate). The OnStart hook installs the submitter into pkg/util/log/setup;
func installErrortrackingHandler(lc fx.Lifecycle, cfg config.Component, at agenttelemetry.Component) {
	if !configUtils.IsErrorTrackingEnabled(cfg) {
		return
	}

	submitter := func(elog errortrackingpkg.ErrorLog) {
		at.SubmitErrorLog(elog)
	}

	bouncerWindow := time.Duration(cfg.GetInt("agent_telemetry.errortracking.bouncer_window_seconds")) * time.Second
	bouncer := errortrackingpkg.NewBouncer(bouncerWindow, 0)

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			pkglogsetup.RegisterErrortrackingSubmitter(submitter)
			pkglogsetup.RegisterErrortrackingBouncer(bouncer)
			return nil
		},
	})
}
