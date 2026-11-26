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
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// configExperimentDeadlineWindow bounds how long a configuration experiment may run before the
	// unit supervisor (unit_supervisor_darwin.go) treats it as failed. It matches the
	// EXPERIMENT_TIMEOUT=3000s the Linux experiment unit runs the Agent under
	// (datadog-agent.service.tmpl), so an experiment is bounded the same way on both platforms.
	configExperimentDeadlineWindow = 50 * time.Minute

	// configExperimentRestoreRetries bounds restoreStableJobs's attempts at handing each label back
	// to its stable job. A transient disk or launchd hiccup is the most plausible failure here.
	configExperimentRestoreRetries = 4
)

// configExperimentRestoreBackoff is the delay before the second restoreStableJobs attempt; it
// doubles before each further one. A package-level var, not a constant, so tests can shrink it
// instead of a retrying test taking several real seconds.
var configExperimentRestoreBackoff = 2 * time.Second

// configExperimentDeadlinePath is where the experiment's deadline is persisted. A package-level
// var, not a constant, so tests can point it at a temporary file instead of the real, root-owned
// directory — the same indirection launchdJobDir uses for job definitions.
//
// The unit supervisor and the experiment hooks act on this file and on the supervisor's state files
// beside it, so they live in a directory only root may write to (paths.ConfigExperimentStatePath),
// never in the Agent's run directory.
var configExperimentDeadlinePath = filepath.Join(paths.ConfigExperimentStatePath, "experiment-deadline")

// configExperimentStateOwner is the uid the deadline file and its directory must belong to before
// they are trusted: root. A package-level var so unprivileged tests can use their own uid.
var configExperimentStateOwner = 0

// experimentDeadline is the configuration experiment's deadline file.
func experimentDeadline() launchd.Deadline {
	return launchd.Deadline{Path: configExperimentDeadlinePath, OwnerUID: configExperimentStateOwner}
}

// configExperimentLockPath is the installer's package database. Every installer command holds its
// exclusive file lock for its whole run, hooks included, so the unit supervisor waits on it before
// it swaps the installer daemon (waitForInstallerCommand). A package-level var for the same reason
// as configExperimentDeadlinePath: tests point it at a temporary file.
var configExperimentLockPath = filepath.Join(paths.PackagesPath, "packages.db")

// configExperimentDirs are the Agent's stable and experiment configuration directories, the same
// pair the installer's own config.Directories manages. A package-level var for the same reason as
// configExperimentDeadlinePath: tests point it at a temporary state root.
var configExperimentDirs = config.Directories{StablePath: paths.AgentConfigDir, ExperimentPath: paths.AgentConfigDirExp}

// discardExperimentConfig puts the experiment configuration path back to resting on the stable
// one, so the daemon no longer reports an experiment. A start that fails must call it, since the
// installer returns the error without removing what it published, and the next attempt's expected
// state would no longer match.
func discardExperimentConfig(ctx context.Context) error {
	if err := configExperimentDirs.RemoveExperiment(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("could not discard the experiment configuration directory: %w", err)
	}
	return nil
}

// configExperiment swaps a set of launchd jobs between their stable and experiment variants. Only
// the unit supervisor drives it: it runs outside both installer daemon jobs, so the set may include
// the installer daemon (see installerJob).
type configExperiment struct {
	jobs launchd.JobSet
	// keepRunningStable makes a hand-back leave alone a label that is already on its stable job,
	// instead of restarting it (restoreStableJobs). A stop sets it: after a reboot mid-experiment,
	// or after the experiment failed on the host, the stable jobs already run on the configuration
	// the stop goes back to. A promote does not, since the stable jobs have to be restarted to read
	// the promoted configuration.
	keepRunningStable bool
}

// Start hands the set over to its experiment jobs: the stable jobs are stopped, the experiment ones
// written and started. If that fails partway, after some of the stable jobs were already stopped,
// the stable jobs are put back rather than left down with nothing in their place.
func (e configExperiment) Start(ctx context.Context) error {
	err := e.jobs.Stop(ctx, launchd.Stable)
	if err == nil {
		err = e.jobs.Write(launchd.Experiment)
	}
	if err == nil {
		err = e.jobs.Start(ctx, launchd.Experiment)
	}
	if err == nil {
		return nil
	}
	if restoreErr := e.restoreStable(context.WithoutCancel(ctx)); restoreErr != nil {
		return fmt.Errorf("experiment failed to start (%w) and could not be rolled back: %w", err, restoreErr)
	}
	return err
}

// restoreStable unloads the experiment jobs, removes their definitions and starts the stable jobs.
//
// The definitions are removed, not just unloaded, so a host that has finished with an experiment
// is indistinguishable from one that never ran one — including to an operator reading
// /Library/LaunchDaemons.
func (e configExperiment) restoreStable(ctx context.Context) error {
	// Unloading the whole experiment set first keeps the reverse teardown order. It is best
	// effort: restoreStableJobs unloads each experiment job again before starting its stable twin,
	// so a job that failed to unload here is retried there rather than ending the restore.
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		log.Warnf("could not unload every experiment job, retrying label by label: %v", err)
	}
	if err := e.restoreStableJobs(ctx); err != nil {
		return err
	}
	// A leftover experiment definition is not loaded at boot, so failing to remove one does not
	// stop the stable set from being restored; it is still reported, but the hand-back is done.
	return e.jobs.Remove(launchd.Experiment)
}

// ensureProcmgr makes sure the process manager is running on its stable job once an experiment
// ends, loading it if anything left it unloaded, and restarting it when restart is set so it picks
// up a promoted configuration.
//
// A configuration experiment never swaps the process manager (see procmgrJob), so nothing in the
// experiment should have stopped it. This is the backstop that keeps it from staying stopped if
// something did. Failures are logged rather than returned: the process manager is not part of the
// experiment, and failing a stop or a promote over it would misreport the experiment's outcome.
func ensureProcmgr(ctx context.Context, restart bool) {
	ctx = context.WithoutCancel(ctx)
	client := launchdClient()
	loaded, err := client.Loaded(ctx, procmgrJob)
	if err != nil {
		log.Warnf("could not tell whether %s is loaded: %v", procmgrJob, err)
		return
	}
	if !loaded {
		log.Warnf("%s is not loaded after the configuration experiment, loading it", procmgrJob)
		definition, err := embedded.GetLaunchdJob(procmgrJob, launchd.Stable)
		if err != nil {
			log.Warnf("could not read the job definition for %s: %v", procmgrJob, err)
			return
		}
		set := launchd.JobSet{Labels: []string{procmgrJob}, Dir: launchdJobDir, Client: client}
		if _, err := set.Job(procmgrJob, launchd.Stable).WriteIfChanged(definition); err != nil {
			log.Warnf("could not write the job definition for %s: %v", procmgrJob, err)
			return
		}
		if err := set.Start(ctx, launchd.Stable); err != nil {
			log.Warnf("could not load %s: %v", procmgrJob, err)
		}
		return
	}
	if restart {
		if err := client.Kickstart(ctx, procmgrJob, true); err != nil {
			log.Warnf("could not restart %s on the promoted configuration: %v", procmgrJob, err)
		}
	}
}

// waitForInstallerCommand waits for any installer command holding the installer's lock to exit,
// for as long as stillWanted reports true. The lock is released before it returns: it is only taken
// to know that no command is running, and the installer daemon must be free to finish the request
// that command was part of -- it reads the package states through another command that takes the
// same lock.
func waitForInstallerCommand(ctx context.Context, stillWanted func() (bool, error)) (bool, error) {
	lock, err := waitForInstallerLock(ctx, stillWanted)
	if err != nil || lock == nil {
		return false, err
	}
	if err := lock.Close(); err != nil {
		log.Warnf("could not release the installer lock: %v", err)
	}
	return true, nil
}

// errInstallerLock wraps every failure to take the installer's lock.
var errInstallerLock = errors.New("could not take the installer lock")

// installerLockPoll is how often a waiter for the installer's lock checks that it still has a
// reason to wait. A var so tests can shorten it.
var installerLockPoll = time.Minute

// installerLockRetry is how often waitForInstallerLock tries the lock again, as bbolt does.
const installerLockRetry = 50 * time.Millisecond

// waitForInstallerLock takes the installer's lock, waiting for any installer command that holds
// it, and checks stillWanted every installerLockPoll while it waits. It returns nil, with no error,
// once stillWanted reports false.
//
// The lock is the exclusive flock bbolt holds on packages.db for as long as an installer command has
// the database open. It is taken directly, without opening the database, so a corrupt database or
// a full disk does not keep the stable jobs from being restored. Closing the returned file releases
// it.
func waitForInstallerLock(ctx context.Context, stillWanted func() (bool, error)) (*os.File, error) {
	f, err := os.OpenFile(configExperimentLockPath, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInstallerLock, err)
	}
	nextCheck := time.Now().Add(installerLockPoll)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("%w: %w", errInstallerLock, err)
		}
		if time.Now().After(nextCheck) {
			wanted, err := stillWanted()
			if err != nil || !wanted {
				f.Close()
				return nil, err
			}
			log.Infof("still waiting for a running installer command")
			nextCheck = time.Now().Add(installerLockPoll)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(installerLockRetry):
		}
	}
}

// restoreStableJobs hands every label back from the experiment job to the stable one, retrying a
// bounded number of times on failure before giving up.
//
// The labels are swapped one at a time: a stable job is started only once its experiment twin has
// left the domain, so the two never run at once, and a label that fails does not keep the others
// from coming back. Only the labels that failed are retried, after a doubling backoff. Once the
// retries are exhausted, or ctx is cancelled between attempts, the error is reported. Callers that
// must see the restore through pass a context without cancellation.
//
// The stable definitions are only written when the one on disk differs: they never change during
// an experiment, so a restore normally writes nothing and still works on a full disk.
//
// With keepRunningStable, a label whose stable job is loaded and whose experiment job is not is
// already handed back, and is left alone (onStableJob).
func (e configExperiment) restoreStableJobs(ctx context.Context) error {
	pending := e.jobs.Labels
	var errs []error
	backoff := configExperimentRestoreBackoff
	for attempt := 0; attempt < configExperimentRestoreRetries && len(pending) > 0; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				errs = append(errs, ctx.Err())
				return fmt.Errorf("could not restore %s: %w", strings.Join(pending, ", "), errors.Join(errs...))
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		var failed []string
		errs = nil
		for _, label := range pending {
			if e.keepRunningStable {
				// A label launchctl cannot tell about is handed back as usual.
				if onStable, err := e.onStableJob(ctx, label); err == nil && onStable {
					continue
				}
			}
			if err := e.jobs.Swap(ctx, label, launchd.Experiment, launchd.Stable); err != nil {
				failed = append(failed, label)
				errs = append(errs, err)
			}
		}
		pending = failed
	}
	if len(pending) == 0 {
		return nil
	}
	return fmt.Errorf("could not restore %s after %d attempts: %w", strings.Join(pending, ", "), configExperimentRestoreRetries, errors.Join(errs...))
}

// onStableJob reports whether label is on its stable job only: the stable job loaded and the
// experiment one not. With neither loaded -- a swap that failed halfway -- the label is not on its
// stable job, and a hand-back still has to start it.
func (e configExperiment) onStableJob(ctx context.Context, label string) (bool, error) {
	experiment, err := e.jobs.Client.Loaded(ctx, label+string(launchd.Experiment))
	if err != nil || experiment {
		return false, err
	}
	return e.jobs.Client.Loaded(ctx, label+string(launchd.Stable))
}
