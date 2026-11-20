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
	"path/filepath"
	"strings"
	"time"

	bberrors "go.etcd.io/bbolt/errors"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// configExperimentDeadlineWindow bounds how long an unsupervised configuration experiment
	// may run before the watcher (config_experiment_watcher_darwin.go) treats it as hung and
	// reverts it. It matches the EXPERIMENT_TIMEOUT=3000s the Linux experiment units run the
	// Agent under (datadog-agent-exp.service, embedded/tmpl), so an experiment is bounded the
	// same way on both platforms.
	configExperimentDeadlineWindow = 50 * time.Minute

	// configExperimentRestoreRetries bounds restoreStableJobs's attempts at handing each label back
	// to its stable job. A transient disk or launchd hiccup is the most plausible failure here;
	// anything that still fails after this many attempts is reported and left, with the reverting
	// marker, for whatever ends the experiment next to retry (see restoreStable).
	configExperimentRestoreRetries = 4
)

// configExperimentRestoreBackoff is the delay before the second restoreStableJobs attempt; it
// doubles before each further one. A package-level var, not a constant, so tests can shrink it
// instead of a retrying test taking several real seconds.
var configExperimentRestoreBackoff = 2 * time.Second

// configExperimentDeadlinePath is where the watcher's deadline is persisted. A package-level var,
// not a constant, so tests can point it at a temporary file instead of the real, root-owned
// directory — the same indirection launchdJobDir uses for job definitions.
//
// The root watcher and the experiment hooks act on this file, so it lives in a directory only root may
// write to (paths.ConfigExperimentStatePath), never in the Agent's run directory.
var configExperimentDeadlinePath = filepath.Join(paths.ConfigExperimentStatePath, "experiment-deadline")

// configExperimentStateOwner is the uid the deadline file and its directory must belong to before
// they are trusted: root. A package-level var so unprivileged tests can use their own uid.
var configExperimentStateOwner = 0

// experimentDeadline is the configuration experiment's deadline file.
func experimentDeadline() launchd.Deadline {
	return launchd.Deadline{Path: configExperimentDeadlinePath, OwnerUID: configExperimentStateOwner}
}

// configExperimentLockPath is the installer's package database. Every installer command holds its
// exclusive file lock for its whole run, hooks included, so the detached watcher takes it too before
// reverting (see revertFromWatcher). A package-level var for the same reason as
// configExperimentDeadlinePath: tests point it at a temporary file.
var configExperimentLockPath = filepath.Join(paths.PackagesPath, "packages.db")

// configExperimentDirs are the Agent's stable and experiment configuration directories, the same
// pair the installer's own config.Directories manages. A package-level var for the same reason as
// configExperimentDeadlinePath: tests point it at a temporary state root.
var configExperimentDirs = config.Directories{StablePath: paths.AgentConfigDir, ExperimentPath: paths.AgentConfigDirExp}

// discardExperimentConfig puts the experiment configuration path back to resting on the stable
// one, so the daemon no longer reports an experiment. A start that fails must call it, since the
// installer returns the error without removing what it published, and the next attempt's expected
// state would no longer match. An experiment that fails after its start was reported keeps its
// configuration instead (configExperiment.Fail), for Fleet to stop.
func discardExperimentConfig(ctx context.Context) error {
	if err := configExperimentDirs.RemoveExperiment(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("could not discard the experiment configuration directory: %w", err)
	}
	return nil
}

// configExperiment swaps the Agent between its stable and its experiment launchd job set.
//
// It owns the jobs and the deadline file that bounds how long an experiment may run
// unsupervised (config_experiment_watcher_darwin.go watches the jobs and the deadline in a
// separate, detached process; this type never talks to that process, it only leaves and clears
// the file the watcher reads). The configuration directories are the installer's own business:
// it publishes the experiment directory before calling the post-start hook, and it discards or
// promotes it around the stop and promote hooks. So by the time any method here runs, the
// directory the incoming job set is about to read is already in place.
//
// Which jobs are in the set depends on who drives it. The hooks run inside the installer daemon's
// job, so they drive the agent jobs only (agentJobSet): a set that contained the daemon would stop
// itself halfway through. The detached watcher runs outside it and drives every job
// (configExperimentJobSet). See installerJob.
type configExperiment struct {
	jobs launchd.JobSet
	// keepRunningStable makes a hand-back leave alone a label that is already on its stable job,
	// instead of restarting it (restoreStableJobs). Only a stop sets it: after a reboot
	// mid-experiment launchd has already started every stable job, and the stable configuration
	// they read is the one the stop goes back to. A promote never does, since the stable jobs
	// have to be restarted to read the promoted configuration.
	keepRunningStable bool
}

// Start hands the Agent over to the experiment job set.
//
// The deadline is written before anything else is touched: an orphaned deadline with no
// experiment is harmless, but an experiment with no deadline is unbounded and silent. If the
// handover fails partway, after some of the stable set has already been stopped, the stable set
// is put back rather than left down with nothing in its place.
func (e configExperiment) Start(ctx context.Context) error {
	if err := experimentDeadline().Write(configExperimentDeadlineWindow); err != nil {
		return err
	}
	if err := e.jobs.Stop(ctx, launchd.Stable); err != nil {
		// Some of the stable jobs may already be down.
		return e.abortStart(ctx, err)
	}
	if err := e.jobs.Write(launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	if err := e.jobs.Start(ctx, launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	return nil
}

// abortStart puts the stable jobs back after a failed handover to the experiment set. It
// reports the original failure, not whatever the rollback itself returns, unless the rollback's
// own retries are exhausted — in which case that error better describes the host's actual
// state.
func (e configExperiment) abortStart(ctx context.Context, startErr error) error {
	if err := e.restoreStable(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("experiment failed to start (%w) and could not be rolled back: %w", startErr, err)
	}
	return startErr
}

// Stop hands the Agent back to the stable job set, abandoning the experiment.
func (e configExperiment) Stop(ctx context.Context) error {
	err := e.restoreStable(ctx)
	ensureProcmgr(ctx, false)
	return err
}

// Fail hands every job back to the stable set after the experiment failed on the host, and leaves
// its configuration deployed. The deadline is replaced by DeadlineFailed rather than cleared, so
// the stop hook knows the jobs are already back.
//
// The host keeps reporting the experiment, with the stable daemon's config_id as the running
// configuration, as Linux does after its experiment unit fails. Fleet's health check sees the
// running configuration fall back to the stable one and rolls the experiment back with a stop.
//
// When the marker cannot be written, the experiment is discarded instead, which leaves the host on
// the stable configuration with nothing reported, rather than failing a revert whose jobs are
// already back.
func (e configExperiment) Fail(ctx context.Context) error {
	deadline := experimentDeadline()
	err := e.restoreStableThen(ctx, func() error {
		markErr := deadline.MarkFailed()
		if markErr == nil {
			return nil
		}
		log.Warnf("could not mark the configuration experiment as failed, discarding it instead: %v", markErr)
		return errors.Join(deadline.Clear(), discardExperimentConfig(ctx))
	})
	ensureProcmgr(ctx, false)
	return err
}

// Promote hands the Agent back to the stable job set, which now reads the promoted configuration.
//
// The work is the same as Stop's: what makes this a promotion rather than a rollback happened in
// the configuration directories before the hook ran. The two are kept apart because they are
// distinct outcomes to a reader of the code and of the traces, and because anything that later
// times an experiment out has to distinguish them.
//
// The process manager is restarted as well: it has no experiment variant, so it kept running on
// the stable configuration throughout the experiment, and it only reads that configuration when
// it starts.
func (e configExperiment) Promote(ctx context.Context) error {
	err := e.restoreStable(ctx)
	ensureProcmgr(ctx, true)
	return err
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

// restoreStable unloads the experiment set, removes its definitions and loads the stable set.
//
// The definitions are removed, not just unloaded, so a host that has finished with an experiment
// is indistinguishable from one that never ran one — including to an operator reading
// /Library/LaunchDaemons.
//
// The deadline is replaced by the reverting marker first: nothing signals the detached watcher
// (config_experiment_watcher_darwin.go) that a deliberate stop is starting, so the changed
// deadline file is itself that signal. Changing it after jobs.Stop below would race the watcher's
// own exit observer, which fires the instant that bootout kills the experiment jobs — both this
// method and the watcher would then try to swap the job sets at once.
//
// The file is only cleared once every stable job is back. A restore that fails, or that is
// interrupted, leaves the marker behind, and whatever ends the experiment next -- a stop, the
// supervising watcher, or the fallback hand-back (handBackInstaller) -- hands the jobs back
// again instead of leaving the host half switched.
func (e configExperiment) restoreStable(ctx context.Context) error {
	return e.restoreStableThen(ctx, experimentDeadline().Clear)
}

// restoreStableThen is restoreStable with finish, instead of clearing the deadline, run once every
// stable job is back.
func (e configExperiment) restoreStableThen(ctx context.Context, finish func() error) error {
	deadline := experimentDeadline()
	if err := deadline.MarkReverting(); err != nil {
		// The marker only backs up a restore that fails. Not being able to write it must not keep
		// the stable set from coming back: that is the one thing a rollback has to do.
		log.Warnf("could not mark the configuration experiment as reverting, restoring the stable set anyway: %v", err)
	}
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
	removeErr := e.jobs.Remove(launchd.Experiment)
	return errors.Join(removeErr, finish())
}

// waitForInstallerCommand waits for any installer command holding the installer's lock to exit,
// and reports whether the deadline still holds token once it has. The lock is released before it
// returns: it is only taken to know that no command is running.
func waitForInstallerCommand(ctx context.Context, deadline launchd.Deadline, token string) (bool, error) {
	lock, ok, err := waitForInstaller(ctx, deadline, token)
	if err != nil || !ok {
		return false, err
	}
	current, readErr := deadline.Read()
	if err := lock.Close(); err != nil {
		log.Warnf("hand-over: could not release the installer lock: %v", err)
	}
	if readErr != nil {
		return false, fmt.Errorf("hand-over: could not read experiment deadline: %w", readErr)
	}
	return current == token, nil
}

// handBackInstaller hands the installer daemon back from its experiment job to its stable one once
// a stop or promote has handed the Agent back. The supervising watcher runs it once it sees the hook
// clear the deadline (experimentSupervisor.handBack); a detached process launched by the hook runs
// it when no watcher runs (handBackInstallerAfterHook). Both run outside the experiment daemon's
// job, which the swap stops.
//
// It first waits for the installer command that ran the hook to exit: it holds the installer's
// lock for its whole run, and only once it exits has the experiment configuration directory been
// discarded or promoted. Swapping the daemon earlier would kill that command halfway, its process
// being in the daemon's job. The swap is made with the lock released, for the reason
// experimentSupervisor.handOverInstaller gives: the daemon must be able to finish the request the
// command was part of, or that request is reported as running forever.
//
// A deadline present by the time the command has exited means a new experiment started after the
// one this hand-back is for. Its own watcher moves the daemon to that experiment, so there is
// nothing to hand back.
//
// The reverting marker means the hook could not hand every agent job back. This finishes it, every
// job and the daemon with it (revertFromWatcher).
func handBackInstaller(ctx context.Context) (err error) {
	lock, err := waitForInstallerLock(ctx, func() (bool, error) { return true, nil })
	if err != nil {
		return fmt.Errorf("hand-back: %w", err)
	}
	token, readErr := experimentDeadline().Read()
	if err := lock.Close(); err != nil {
		log.Warnf("hand-back: could not release the installer lock: %v", err)
	}
	if readErr != nil {
		return fmt.Errorf("hand-back: could not read experiment deadline: %w", readErr)
	}
	if experimentPending(token) {
		log.Infof("hand-back: a new configuration experiment started, leaving the installer daemon to it")
		return nil
	}
	if token == launchd.DeadlineReverting {
		// The stop or promote that launched this process could not hand the Agent back. Finish
		// that restore from here, outside the daemon's job, every job and the daemon with it.
		log.Warnf("hand-back: the stop or promote left the jobs half handed back, finishing the restore")
		if _, err := revertFromWatcher(ctx, experimentDeadline(), token, "a stop or promote could not hand every job back"); err != nil {
			return fmt.Errorf("hand-back: %w", err)
		}
		return nil
	}
	e := configExperiment{jobs: installerJobSet()}
	if handedBack, err := installerOnStableJob(ctx); err != nil {
		log.Warnf("hand-back: could not tell which job the installer daemon is on, handing it back: %v", err)
	} else if handedBack {
		// Another hand-back got there first. Swapping again would stop the stable daemon it just
		// started, along with whatever command that daemon is running.
		log.Infof("hand-back: the installer daemon is already on its stable job")
		if err := e.jobs.Remove(launchd.Experiment); err != nil {
			return fmt.Errorf("hand-back: %w", err)
		}
		return nil
	}
	restoreErr := e.restoreStableJobs(context.WithoutCancel(ctx))
	removeErr := e.jobs.Remove(launchd.Experiment)
	if err := errors.Join(restoreErr, removeErr); err != nil {
		return fmt.Errorf("hand-back: %w", err)
	}
	return nil
}

// installerOnStableJob reports whether the installer daemon is on its stable job only: the stable
// job loaded and the experiment one not. With neither loaded -- a swap that failed halfway -- the
// daemon is not on its stable job, and a hand-back still has to start it.
func installerOnStableJob(ctx context.Context) (bool, error) {
	client := launchdClient()
	experiment, err := client.Loaded(ctx, installerJob+string(launchd.Experiment))
	if err != nil || experiment {
		return false, err
	}
	return client.Loaded(ctx, installerJob)
}

// errInstallerLock wraps every failure to take the installer's lock other than a timeout.
var errInstallerLock = errors.New("could not take the installer lock")

// waitForInstallerLock takes the installer's lock, waiting for any installer command that holds
// it. Between waits it asks stillWanted, and gives up without the lock, returning a nil lock and
// no error, once that reports false.
func waitForInstallerLock(ctx context.Context, stillWanted func() (bool, error)) (*db.PackagesDB, error) {
	for {
		lock, err := db.New(ctx, configExperimentLockPath, db.WithTimeout(installerLockPoll))
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, bberrors.ErrTimeout) {
			return nil, fmt.Errorf("%w: %w", errInstallerLock, err)
		}
		wanted, err := stillWanted()
		if err != nil || !wanted {
			return nil, err
		}
		log.Infof("still waiting for a running installer command")
	}
}

// experimentPending reports whether a token read from the deadline file stands for a running
// experiment, as opposed to none (empty), one being handed back to the stable set, or one that
// failed and is back on it already.
func experimentPending(token string) bool {
	return token != "" && token != launchd.DeadlineReverting && token != launchd.DeadlineFailed
}

// restoreStableJobs hands every label back from the experiment job to the stable one, retrying a
// bounded number of times on failure before giving up.
//
// The labels are swapped one at a time: a stable job is started only once its experiment twin has
// left the domain, so the two never run at once, and a label that fails does not keep the others
// from coming back. Only the labels that failed are retried, after a doubling backoff. Once the
// retries are exhausted the error is reported; restoreStable leaves the reverting marker behind,
// so whatever ends the experiment next retries.
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
			time.Sleep(backoff)
			backoff *= 2
		}
		var failed []string
		errs = nil
		for _, label := range pending {
			if e.keepRunningStable && e.onStableJob(ctx, label) {
				continue
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

// onStableJob reports whether label is already on its stable job: the stable job loaded and the
// experiment one not. A label launchctl cannot tell about is not, so it is handed back as usual.
func (e configExperiment) onStableJob(ctx context.Context, label string) bool {
	stable, err := e.jobs.Client.Print(ctx, label+string(launchd.Stable))
	if err != nil || !stable.Loaded {
		return false
	}
	experiment, err := e.jobs.Client.Print(ctx, label+string(launchd.Experiment))
	return err == nil && !experiment.Loaded
}
