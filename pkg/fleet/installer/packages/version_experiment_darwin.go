// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/experiment"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// versionExperiment swaps the Agent between its stable and its experiment launchd job set for an
// update experiment: a different version of the code, rather than a different configuration.
//
// The difference from a configuration experiment is which side of the layout moves. A
// configuration experiment swaps etc for etc-exp and leaves the pool alone; a version experiment
// leaves the configuration alone and moves the pool's experiment link. The job definitions are the
// same two sets in both cases, which is why both go through launchd.JobSet.
type versionExperiment struct {
	jobs launchd.JobSet

	// poolRoot is the package's directory in the versioned pool, holding the version
	// directories and the stable and experiment links.
	poolRoot string

	// deadline bounds how long the experiment may run. It is recorded here, by the hook, rather
	// than by the daemon, because the hook is the only thing that knows the experiment actually
	// started -- and because the daemon is itself part of the payload and is restarted.
	deadline *experiment.Deadline

	// appBundle swaps the /Applications bundle. Nil skips it.
	appBundle *appBundleSwap

	// duration is how long this experiment may run. Zero takes the default.
	duration time.Duration
}

func (e versionExperiment) stableLink() string {
	return filepath.Join(e.poolRoot, "stable")
}

func (e versionExperiment) experimentLink() string {
	return filepath.Join(e.poolRoot, "experiment")
}

// Prepare clears anything a previous experiment left behind, before the payload is installed.
//
// Nothing here is disruptive: the stable job set keeps running throughout. A stale -exp definition
// is not merely untidy -- it names the pool's experiment link, so it would come back up against
// whatever version that link ends up pointing at.
func (e versionExperiment) Prepare(ctx context.Context) error {
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		log.Warnf("could not unload a leftover experiment job set: %v", err)
	}
	if err := e.jobs.Remove(launchd.Experiment); err != nil {
		return fmt.Errorf("could not remove a leftover experiment job set: %w", err)
	}
	return nil
}

// Start hands the Agent over to the experiment job set.
//
// The order is the point. Everything reversible happens before anything disruptive:
//
//  1. the experiment definitions are written -- reversible by deleting them;
//  2. the deadline is recorded -- reversible by clearing it, and recorded *before* the experiment
//     runs so there is no window in which an experiment is running unbounded. A deadline on a host
//     with no experiment costs one revert of a host that is already stable, which is a no-op
//     sequence; an experiment with no deadline costs the host;
//  3. only then are the stable jobs stopped and the experiment jobs started.
//
// If starting the experiment jobs fails the host is put back on stable before the error is
// returned, because the alternative is a host running neither set.
func (e versionExperiment) Start(ctx context.Context, version string) (err error) {
	if err := e.jobs.Write(launchd.Experiment); err != nil {
		return err
	}
	if err := e.deadline.Set(version, time.Now().Add(experiment.ClampDuration(e.duration))); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			log.Errorf("Could not start experiment %s, returning to stable: %v", version, err)
			// Detached: the caller's context may already be cancelled, and the host must
			// not be left running neither job set because a deadline elapsed.
			if revertErr := e.Revert(context.WithoutCancel(ctx)); revertErr != nil {
				err = fmt.Errorf("%w, and the host could not be returned to stable: %w", err, revertErr)
			}
		}
	}()

	if err := e.jobs.Stop(ctx, launchd.Stable); err != nil {
		return err
	}
	return e.jobs.Start(ctx, launchd.Experiment)
}

// Revert puts the host back on the stable version.
//
// The experiment link is collapsed onto the stable link *first*, before the experiment jobs are
// stopped. The -exp definitions reach the binaries through that link, so once it names stable
// there is no path by which an -exp job -- one launchd is slow to kill, one an operator
// kickstarts, one a later bootstrap loads from a definition that failed to be removed -- can run
// the version being abandoned. Stopping the jobs first would leave that window open for as long as
// the link move took.
//
// The deadline is not cleared here. It is cleared by the hook that runs last, after the stable job
// set is confirmed back up, so a revert that fails partway leaves the deadline in place and is
// retried rather than forgotten.
//
// On a host that is already stable every step is a no-op: the link already points at stable, there
// are no -exp jobs to stop, no definitions to remove, and starting the stable set that is already
// running is idempotent. That is deliberate -- it is what makes the supervisor able to revert
// without first working out whether it needs to.
func (e versionExperiment) Revert(ctx context.Context) error {
	if err := e.collapseExperimentLink(); err != nil {
		return err
	}
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		return err
	}
	if err := e.jobs.Remove(launchd.Experiment); err != nil {
		return err
	}
	// The stable definitions are rewritten rather than assumed present: this is the path a host
	// takes back to a working state, and bootstrap needs the file on disk.
	if err := e.jobs.Write(launchd.Stable); err != nil {
		return err
	}
	return e.jobs.Start(ctx, launchd.Stable)
}

// StopExperimentJobs unloads the experiment job set and removes its definitions, without touching
// the pool links.
//
// This is the promote path's half of the swap: the links are the installer's to move, and it moves
// the stable link onto the experiment's version between the pre- and post-promote hooks.
func (e versionExperiment) StopExperimentJobs(ctx context.Context) error {
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		return err
	}
	return e.jobs.Remove(launchd.Experiment)
}

// StartStableJobs writes and starts the stable job set. On the promote path the stable link now
// names the promoted version, so this is what puts it into service.
func (e versionExperiment) StartStableJobs(ctx context.Context) error {
	if err := e.jobs.Write(launchd.Stable); err != nil {
		return err
	}
	return e.jobs.Start(ctx, launchd.Stable)
}

// collapseExperimentLink points the pool's experiment link at its stable link.
//
// It is the same end state as repository.DeleteExperiment's, done here because ordering is the
// whole content of a revert and the installer runs DeleteExperiment after this hook, not before.
// Doing it twice is harmless: setting a symlink to what it already names is idempotent.
func (e versionExperiment) collapseExperimentLink() error {
	stable := e.stableLink()
	if _, err := os.Lstat(stable); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No stable link at all: this host has never had a successful install, so
			// there is nothing to fall back to and nothing this function can fix.
			return fmt.Errorf("cannot return to stable: %s does not exist", stable)
		}
		return fmt.Errorf("could not inspect %s: %w", stable, err)
	}
	if err := symlink.Set(e.experimentLink(), stable); err != nil {
		return fmt.Errorf("could not collapse the experiment link onto stable: %w", err)
	}
	return nil
}

// appBundleSwap replaces the /Applications bundle with the one from a version directory.
//
// The bundle is swapped at promote and at no other time. An experiment is a bet on a version of
// the daemons, which are invisible; the bundle is what the person in front of the machine sees, and
// swapping it for a version that may be reverted within the hour would show them an application
// that then disappears -- and would have to be swapped back on a path whose whole job is to be as
// short and as reliable as possible. The stable jobs and the bundle are therefore promoted
// together, and a running experiment leaves /Applications exactly as it was.
type appBundleSwap struct {
	// appsDir is where the bundle lives, normally /Applications.
	appsDir string
	// name is the bundle's name, e.g. "Datadog Agent.app".
	name string
}

var defaultAppBundle = &appBundleSwap{appsDir: "/Applications", name: "Datadog Agent.app"}

// Swap moves the bundle from the version directory into place.
//
// A version that ships no bundle is not an error: the .pkg payload is the same tree for every
// flavour, and a build without the GUI simply has nothing to swap. The bundle already in
// /Applications is then left alone rather than removed, because removing it would take the GUI
// away from a host on the strength of a build flag.
func (s *appBundleSwap) Swap(_ context.Context, versionPath string) error {
	if s == nil {
		return nil
	}
	source := filepath.Join(versionPath, s.name)
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Debugf("Version %s ships no %s, leaving the installed bundle alone", versionPath, s.name)
			return nil
		}
		return fmt.Errorf("could not inspect %s: %w", source, err)
	}
	target := filepath.Join(s.appsDir, s.name)
	// The bundle is replaced rather than merged: a bundle is a unit, and a tree with files
	// from two versions in it is a bundle from neither.
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("could not remove %s: %w", target, err)
	}
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("could not move %s into %s: %w", source, s.appsDir, err)
	}
	return nil
}

// agentVersionExperiment is the version experiment over the Agent's swappable job set.
//
// The installer daemon is not in the set: it is the process performing the swap, and a set that
// contained it would stop itself halfway through. That is also why the daemon keeps running the
// stable installer binary for the whole experiment -- and why the deadline has to be on disk,
// because a promote replaces that binary underneath it.
func agentVersionExperiment() versionExperiment {
	return versionExperiment{
		jobs:      agentJobSet(),
		poolRoot:  filepath.Join(paths.PackagesPath, "datadog-agent"),
		deadline:  experiment.NewDeadline(),
		appBundle: defaultAppBundle,
	}
}

// preStartExperimentDatadogAgent clears a previous experiment's job definitions. It runs before the
// payload is installed and does not disturb the stable job set.
func preStartExperimentDatadogAgent(ctx HookContext) error {
	return agentVersionExperiment().Prepare(ctx)
}

// postStartExperimentDatadogAgent hands the Agent over to the experiment job set. The installer has
// already installed the payload and moved the pool's experiment link by the time it runs.
func postStartExperimentDatadogAgent(ctx HookContext) error {
	version := filepath.Base(ctx.PackagePath)
	return agentVersionExperiment().Start(ctx, version)
}

// preStopExperimentDatadogAgent returns the host to the stable version.
//
// The context is detached from cancellation: the experiment is being torn down, and stopping
// halfway would leave the host running neither job set.
func preStopExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	return agentVersionExperiment().Revert(ctx)
}

// postStopExperimentDatadogAgent clears the deadline, last, once the host is back on stable.
//
// Last, because the deadline is the record that an experiment needs reverting. Clearing it before
// the revert had succeeded would leave a host still running an experiment with nothing left that
// knows to end it.
func postStopExperimentDatadogAgent(_ HookContext) error {
	return agentVersionExperiment().deadline.Clear()
}

// prePromoteExperimentDatadogAgent unloads the experiment job set.
//
// The installer moves the stable link onto the experiment's version immediately after this returns,
// so the jobs have to be down first: they are the processes running out of the directory the link
// is about to stop naming.
func prePromoteExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	return agentVersionExperiment().StopExperimentJobs(ctx)
}

// postPromoteExperimentDatadogAgent puts the promoted version into service.
//
// The stable link now names it, so the stable job definitions -- which name the façade, which
// names the stable link -- reach the new version without being rewritten for it. The bundle is
// swapped here and only here.
func postPromoteExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	e := agentVersionExperiment()
	if err := e.StartStableJobs(ctx); err != nil {
		return err
	}
	if err := e.appBundle.Swap(ctx, ctx.PackagePath); err != nil {
		// The bundle is a convenience for the person at the machine; the Agent is running
		// either way, and failing the promote over it would revert a version that works.
		log.Warnf("Could not swap the /Applications bundle: %v", err)
	}
	// The deadline last, as on the stop path: until it is cleared the supervisor would still
	// revert, and a promote that failed halfway is better reverted than forgotten.
	return e.deadline.Clear()
}
