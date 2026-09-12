// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// configExperiment swaps the Agent between its stable and its experiment launchd job set.
//
// It owns only the jobs. The configuration directories are the installer's own business: it
// publishes the experiment directory before calling the post-start hook, and it discards or
// promotes it around the stop and promote hooks. So by the time any method here runs, the
// directory the incoming job set is about to read is already in place — which is why the swap can
// be a plain unload-then-load and needs no rollback of its own.
//
// The installer daemon is deliberately not part of the set. It is the process performing the
// swap, and a set that contained it would stop itself halfway through.
type configExperiment struct {
	jobs launchd.JobSet
}

// Start hands the Agent over to the experiment job set.
func (e configExperiment) Start(ctx context.Context) error {
	if err := e.jobs.Stop(ctx, launchd.Stable); err != nil {
		return err
	}
	if err := e.jobs.Write(launchd.Experiment); err != nil {
		return err
	}
	return e.jobs.Start(ctx, launchd.Experiment)
}

// Stop hands the Agent back to the stable job set, abandoning the experiment.
func (e configExperiment) Stop(ctx context.Context) error {
	return e.restoreStable(ctx)
}

// Promote hands the Agent back to the stable job set, which now reads the promoted configuration.
//
// The work is the same as Stop's: what makes this a promotion rather than a rollback happened in
// the configuration directories before the hook ran. The two are kept apart because they are
// distinct outcomes to a reader of the code and of the traces, and because anything that later
// times an experiment out has to distinguish them.
func (e configExperiment) Promote(ctx context.Context) error {
	return e.restoreStable(ctx)
}

// restoreStable unloads the experiment set, removes its definitions and loads the stable set.
//
// The definitions are removed, not just unloaded, so a host that has finished with an experiment
// is indistinguishable from one that never ran one — including to an operator reading
// /Library/LaunchDaemons.
func (e configExperiment) restoreStable(ctx context.Context) error {
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		return err
	}
	if err := e.jobs.Remove(launchd.Experiment); err != nil {
		return err
	}
	// The stable definitions are rewritten rather than assumed present: bootstrap needs the file
	// on disk, and this is the path a host takes back to a working state.
	if err := e.jobs.Write(launchd.Stable); err != nil {
		return err
	}
	return e.jobs.Start(ctx, launchd.Stable)
}
