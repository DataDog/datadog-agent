// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatform implements the "healthplatform" bundle, providing the
// health platform component for detecting and reporting agent health issues.
//
// The health platform collects health signals from various agent components,
// persists detected issues, and forwards reports to the Datadog intake.
//
// This bundle does not depend on any other bundles.
package healthplatform

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
	egressfx "github.com/DataDog/datadog-agent/comp/healthplatform/egress/fx"
	forwarderfx "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/fx"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"

	// Import issue modules to trigger their init() registration.
	// The bundle is the correct place for side-effect imports; impl packages
	// must not import other impl packages.
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/admisconfig"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/admissionprobe"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/checkfailure"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dockerpermissions"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/rofspermissions"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	runnerfx "github.com/DataDog/datadog-agent/comp/healthplatform/runner/fx"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	schedulerfx "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/fx"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	corefx "github.com/DataDog/datadog-agent/comp/healthplatform/store/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-health

// Bundle defines the fx options for the health platform bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		runnerfx.Module(),
		schedulerfx.Module(),
		forwarderfx.Module(),
		egressfx.Module(),
		corefx.Module(),
		fx.Invoke(bootstrapBuiltInHealthChecks),
	)
}

// bootstrapBuiltInHealthChecks registers all built-in health checks at startup
// and forces the egress component to be instantiated (its lifecycle hooks drive the
// periodic store→intake flush).
// Once checks run in background goroutines so they do not block OnStart;
// periodic checks are registered with the scheduler.
func bootstrapBuiltInHealthChecks(
	cfg config.Component,
	logger log.Component,
	runner runnerdef.Component,
	scheduler schedulerdef.Component,
	store storedef.Component,
	_ egressdef.Component,
	lc fx.Lifecycle,
) {
	if !cfg.GetBool("health_platform.enabled") {
		return
	}
	registry := buildRegistry(cfg)
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			for _, once := range registry.GetBuiltInStartupHealthChecks() {
				go func() {
					newIDs, err := runner.Run(once.Source, once.Fn)
					if err != nil {
						logger.Warnf("built-in once-check %q failed at startup: %v", once.Source, err)
						return
					}
					// Resolve any issues from this source that were active
					// (e.g. persisted from a prior run) but are no longer reported.
					newSet := make(map[string]struct{}, len(newIDs))
					for _, id := range newIDs {
						newSet[id] = struct{}{}
					}
					for _, t := range once.IssueTypes {
						for _, id := range store.GetActiveIssueIDsByIssueType(t) {
							if _, still := newSet[id]; !still {
								store.ResolveIssue(id)
							}
						}
					}
				}()
			}
			for _, check := range registry.GetBuiltInPeriodicHealthChecks() {
				var initialIDs []string
				for _, t := range check.IssueTypes {
					initialIDs = append(initialIDs, store.GetActiveIssueIDsByIssueType(t)...)
				}
				if err := scheduler.Schedule(check.Source, check.Fn, check.Interval, initialIDs); err != nil {
					logger.Warnf("failed to schedule built-in health check %q: %v", check.Source, err)
				}
			}
			return nil
		},
	})
}

// buildRegistry instantiates all registered modules into a Registry.
// TODO: this duplicates the registry built inside store.NewComponent (for template
// lookups). Both will be unified when issues/registry is promoted to an fx component.
func buildRegistry(cfg config.Component) *issuesmod.Registry {
	registry := issuesmod.NewRegistry()
	for _, module := range issuesmod.GetAllModules(cfg) {
		registry.RegisterModule(module)
	}
	return registry
}
