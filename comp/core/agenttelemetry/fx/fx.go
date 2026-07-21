// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the agenttelemetry component
package fx

import (
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
// gate). Registration runs synchronously here, at fx.Invoke time, rather
// than from an OnStart hook: some components (e.g. npcollector) log
// construction-time errors from their own constructor, which Fx may
// resolve — and thus run — before app.Start() fires any OnStart hook.
// Registering synchronously (ahead of any later fx.Invoke that forces
// such a component's construction) ensures those early errors aren't
// dropped for lack of a registered submitter. This is safe before atel's
// own OnStart: SubmitErrorLog only enqueues into errLogsCh, which
// createAtel already allocates. The matching clear runs synchronously
// inside atel.stop() (deliberately not as a separate OnStop hook here) so
// it precedes the final flush-goroutine drain.
func installErrortrackingHandler(cfg config.Component, at agenttelemetry.Component) {
	if !configUtils.IsErrorTrackingEnabled(cfg) {
		return
	}

	submitter := func(elog errortrackingpkg.ErrorLog) {
		at.SubmitErrorLog(elog)
	}

	bouncerWindow := time.Duration(cfg.GetInt("agent_telemetry.errortracking.bouncer_window_seconds")) * time.Second
	bouncer := errortrackingpkg.NewBouncer(bouncerWindow, 0)

	pkglogsetup.RegisterErrortrackingSubmitter(submitter)
	pkglogsetup.RegisterErrortrackingBouncer(bouncer)
}
