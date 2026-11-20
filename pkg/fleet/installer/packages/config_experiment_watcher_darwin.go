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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// deadlineTickInterval is how often watchExperiment polls the deadline file while waiting on
// the exit observer. A var so tests can shorten it.
var deadlineTickInterval = 30 * time.Second

// watcherDeadlineReadFailures is how many consecutive ticks may fail to read the deadline before
// the watcher stops trusting it can supervise the experiment, and reverts it.
const watcherDeadlineReadFailures = 3

// watcherFailSafeAttempts bounds how many times a watcher that can no longer supervise its
// experiment tries to revert it (failSafeRevert), and watcherFailSafeBackoff is the delay before
// the second attempt, doubling before each further one. A var so tests can shorten it.
const watcherFailSafeAttempts = 3

var watcherFailSafeBackoff = 5 * time.Second

// startupPollInterval and startupPollTimeout bound how long watchExperiment waits for a job that
// Kickstart was already told to start, but that launchd has not yet reported a pid for. This
// process is spawned detached, after the jobs have been started, so a healthy job can legitimately
// still be forking when this runs. A job still without a pid or a recorded exit once the bound is
// up failed to start, and fails the experiment: the exit observer only watches the pids it is armed
// with, so a job that started later would crash unnoticed until the deadline.
const startupPollInterval = 100 * time.Millisecond

var startupPollTimeout = 5 * time.Second

// installerLockPoll bounds each wait for the installer's lock, so a watcher whose experiment was
// stopped, promoted or replaced meanwhile notices and exits instead of waiting out a long operation.
// A var so tests can shorten it.
var installerLockPoll = time.Minute

// watchExperiment runs in a detached process, in a session of its own so that it survives the
// installer daemons it swaps, and supervises a configuration experiment until it ends. Two such
// processes take part in an experiment, one after the other:
//
//   - W1 is launched by the start hook, from the stable daemon (postStartConfigExperimentDatadogAgent).
//     It arms on the agent experiment jobs at once, hands the installer daemon over to its
//     experiment job once the start command has exited, and arms on the experiment daemon too.
//   - W2 is launched as the experiment daemon starts, by its job definition
//     (startSupervisorRelay, com.datadoghq.installer.plist.tmpl). It arms on every experiment job,
//     then stops every other watcher (stopOtherWatchers): W1, and any left from an earlier
//     experiment. A watcher keeps running for its signal grace after SIGTERM, so for a few seconds
//     both supervise.
//
// Which one a process is follows from the host: the experiment daemon running means this process is
// W2. Each failure has a single owner, so the two never race to revert the same experiment
// (experimentSupervisor.owns):
//
//   - W1 owns the experiment daemon's exit for as long as it runs, and every other failure only
//     while no other watcher runs: before W2 starts, when W2 never starts, or once W2 is gone.
//   - W2 owns every failure, and the hand-back, except the experiment daemon's exit while W1 still
//     runs.
//
// A failure a watcher does not own is deferred rather than dropped (deferFailure): the watcher waits
// for the owner to end the experiment, and acts itself if the owner goes away first.
//
// Whichever supervises, it ends the experiment the same way:
//
//   - an experiment job exiting in a way that fails the experiment (exitFailsExperiment), or the
//     deadline expiring, reverts every job, the daemon included (revertFromWatcher);
//   - a stop or promote replaces the deadline with the reverting marker, hands the Agent back from
//     its hook and clears the deadline; the supervisor sees the deadline change, waits for the
//     hook's command to exit and hands the installer daemon back (handBackInstaller);
//   - a stop or promote whose command died partway, leaving the reverting marker, is finished from
//     here, once that command has released the installer lock (finishUnlessEnded).
//
// The deadline file is the only coordination point: the supervisor re-reads it on every change to
// its directory (watchExperimentState) and on a slower tick, and every revert takes the lock every
// installer command holds for its whole run and re-checks the deadline under it. That lock also
// keeps a misjudged ownership harmless: a second revert finds the deadline changed and does nothing.
//
// It fails safe: once the experiment is known to be running, any failure that keeps this process
// from supervising it -- launchctl, the exit observers, the installer lock, the deadline file --
// reverts the experiment rather than leaving it running with nothing watching (failSafeRevert),
// unless another watcher is supervising it.
func watchExperiment(ctx context.Context) error {
	deadline := experimentDeadline()

	token, err := deadline.Read()
	if err != nil {
		// The experiment would otherwise run with nothing watching it.
		return failSafeRevert(ctx, deadline, "", fmt.Errorf("watcher: could not read experiment deadline: %w", err))
	}
	if !experimentPending(token) {
		// Stopped or promoted before this process got to run: nothing left to supervise.
		return nil
	}
	successor, err := experimentDaemonRunning(ctx)
	if err != nil {
		return failSafeRevert(ctx, deadline, token, fmt.Errorf("watcher: could not tell which daemon runs: %w", err))
	}
	s := &experimentSupervisor{deadline: deadline, token: token, pids: map[int]string{}, handOver: make(chan handOverResult, 1)}
	if successor {
		s.role = secondWatcher
	}
	defer s.release()
	if s.role == secondWatcher {
		err = s.takeOver(ctx)
	} else {
		err = s.lead(ctx)
	}
	if err == nil && !s.finished {
		err = s.run(ctx)
	}
	if err != nil {
		if running, lookupErr := watcherRunning(ctx); lookupErr == nil && running {
			log.Warnf("watcher: cannot keep supervising, leaving the experiment to the other watcher: %v", err)
			return nil
		}
		return failSafeRevert(ctx, deadline, token, err)
	}
	return nil
}

// experimentDaemonRunning reports whether the experiment daemon runs. Its job loaded without a pid is
// what launchd leaves of it after a reboot, and does not count.
var experimentDaemonRunning = func(ctx context.Context) (bool, error) {
	status, err := launchdClient().Print(ctx, installerJob+string(launchd.Experiment))
	if err != nil {
		return false, err
	}
	return status.PID != 0, nil
}

// watcherOverlap bounds how long W1 defers a failure it does not own to W2 before acting itself: the
// grace a watcher gets after SIGTERM, after which W1 is gone anyway. A var so tests can shorten it.
var watcherOverlap = 10 * time.Second

// deferPoll is how often a deferring watcher checks on the other one. A var so tests can shorten it.
var deferPoll = 200 * time.Millisecond

// supervisorRole tells the watcher the start hook launched from the one the experiment daemon did.
type supervisorRole int

const (
	firstWatcher supervisorRole = iota
	secondWatcher
)

// experimentSupervisor is the state of a watcher that supervises an experiment.
type experimentSupervisor struct {
	deadline launchd.Deadline
	token    string
	role     supervisorRole

	// mu keeps the hand-over's daemon swap and every revert or hand-back from running at once.
	mu sync.Mutex

	// pids maps every armed pid to its job's label, without the variant suffix.
	pids map[int]string
	// agentExits and daemonExits deliver the exits of the jobs armed in each stage. W2 arms every
	// job at once, on agentExits.
	agentExits, daemonExits         <-chan launchd.ExitEvent
	agentObserver, daemonObserver   *launchd.ExitObserver
	agentArmed, daemonArmed         int
	agentDelivered, daemonDelivered int

	// stateChanges delivers a change to the deadline's directory; stopStateWatch ends the watch.
	stateChanges   <-chan struct{}
	stopStateWatch func()

	// predecessorGone is set once W2 has stopped every other watcher.
	predecessorGone atomic.Bool

	// handOver delivers W1's hand-over outcome.
	handOver chan handOverResult
	// finished is set once the experiment has been ended from here, and nothing is left to supervise.
	finished bool
}

// handOverResult is what W1's hand-over goroutine reports.
type handOverResult struct {
	err error
	// over means the experiment ended during the hand-over, and the hand-over ended it.
	over bool
	// ended means the experiment was stopped or promoted before the swap: the daemon was not
	// handed over, and the next deadline check finishes it.
	ended bool
	// daemonGone means the experiment daemon was swapped in but exited before it could be armed on.
	daemonGone bool
	// observer, exits and pid are the experiment daemon's, once armed.
	observer *launchd.ExitObserver
	exits    <-chan launchd.ExitEvent
	pid      int
}

// release stops watching the deadline's directory and disarms the exit observers.
func (s *experimentSupervisor) release() {
	if s.stopStateWatch != nil {
		s.stopStateWatch()
	}
	for _, observer := range []*launchd.ExitObserver{s.agentObserver, s.daemonObserver} {
		if observer == nil {
			continue
		}
		if err := observer.Disarm(); err != nil {
			log.Warnf("watcher: could not disarm exit observer: %v", err)
		}
	}
}

// watchState starts watching the deadline's directory. Not being able to only costs the shortcut:
// the deadline tick still sees every change.
func (s *experimentSupervisor) watchState() {
	changes, stop, err := watchExperimentState()
	if err != nil {
		log.Warnf("watcher: supervising on the deadline tick only: %v", err)
		return
	}
	s.stateChanges, s.stopStateWatch = changes, stop
}

// arm resolves and arms an exit observer on every experiment job of jobs. It reports a job that
// already exited in a way that fails the experiment, or a set where none runs, as reason.
func (s *experimentSupervisor) arm(ctx context.Context, jobs launchd.JobSet) (*launchd.ExitObserver, <-chan launchd.ExitEvent, int, string, error) {
	jobPIDs, exited, err := resolveExperimentPids(ctx, jobs)
	if err != nil {
		return nil, nil, 0, "", fmt.Errorf("watcher: could not resolve experiment pids: %w", err)
	}
	if exited != nil {
		// launchd itself recorded this job's exit -- a durable fact, not a timing guess --
		// before the watcher ever got a chance to arm an observer on it.
		return nil, nil, 0, fmt.Sprintf("experiment job %s exited before the watcher could observe it (status %d)", exited.Label, exited.LastExitStatus), nil
	}
	if len(jobPIDs) == 0 {
		return nil, nil, 0, "experiment never started", nil
	}
	pids := make([]int, 0, len(jobPIDs))
	for pid, label := range jobPIDs {
		pids = append(pids, pid)
		s.pids[pid] = label
	}
	observer, exits, err := launchd.ArmExitObserver(pids)
	if err != nil {
		return nil, nil, 0, "", fmt.Errorf("watcher: could not arm exit observer: %w", err)
	}
	return observer, exits, len(pids), "", nil
}

// lead is W1's start: arm on the agent experiment jobs, and hand the installer daemon over in the
// background, so a crash is caught while the hand-over waits for the start command.
func (s *experimentSupervisor) lead(ctx context.Context) error {
	// A watcher an earlier experiment left behind would otherwise act on this experiment's end. W2
	// stops other watchers too, but only once the experiment daemon is up.
	if err := stopSurvivingWatchers(ctx); err != nil {
		log.Warnf("watcher: could not stop a previous watcher, supervising anyway: %v", err)
	}
	observer, exits, armed, reason, err := s.arm(ctx, agentJobSet())
	if err != nil {
		return err
	}
	if reason != "" {
		return s.endWithFailure(ctx, reason)
	}
	s.agentObserver, s.agentExits, s.agentArmed = observer, exits, armed
	s.watchState()
	go func() { s.handOver <- s.handOverInstaller(ctx) }()
	return nil
}

// handOverInstaller hands the installer daemon over from its stable job to its experiment one, and
// arms on the experiment daemon.
//
// It first waits for the installer command that ran the start hook to exit
// (waitForInstallerCommand): its process is in the stable daemon's job, so swapping the daemon
// earlier would kill it halfway. The lock is released again before the swap: the daemon still has to
// finish the request that command was part of once it exits -- read the package states, through an
// installer command that takes the same lock, and record the task as done -- and a swap made under
// the lock would stop it with that request stuck, reported as running forever.
//
// With the lock released, a stop or promote can come in before the swap. The deadline is read again
// once the swap is done: an experiment that ended is handed straight back, and a stop the swap
// killed partway through, which left the reverting marker, is finished here, every job with it --
// that stop is never reported as done. A failed swap may have left the host with no daemon at all,
// so it reverts every job, the daemon included.
func (s *experimentSupervisor) handOverInstaller(ctx context.Context) handOverResult {
	pending, err := waitForInstallerCommand(ctx, s.deadline, s.token)
	if err != nil {
		return handOverResult{err: err}
	}
	if !pending {
		return handOverResult{ended: true}
	}
	s.mu.Lock()
	current, err := s.deadline.Read()
	if err != nil {
		s.mu.Unlock()
		return handOverResult{err: fmt.Errorf("hand-over: could not read experiment deadline: %w", err)}
	}
	if current != s.token {
		s.mu.Unlock()
		return handOverResult{ended: true}
	}
	swapErr := installerJobSet().Swap(context.WithoutCancel(ctx), installerJob, launchd.Stable, launchd.Experiment)
	if swapErr != nil {
		reason := fmt.Sprintf("could not hand the installer daemon over to the experiment: %v", swapErr)
		_, err := revertFromWatcher(ctx, s.deadline, s.token, reason)
		s.mu.Unlock()
		if err != nil {
			return handOverResult{err: fmt.Errorf("%s, and %w", reason, err)}
		}
		return handOverResult{over: true}
	}
	current, err = s.deadline.Read()
	if err != nil {
		s.mu.Unlock()
		return handOverResult{err: fmt.Errorf("hand-over: could not read experiment deadline: %w", err)}
	}
	switch current {
	case s.token:
	case launchd.DeadlineReverting:
		log.Warnf("hand-over: a stop or promote was interrupted while the installer daemon was handed over, finishing it")
		_, err := revertFromWatcher(ctx, s.deadline, current, "a stop or promote was interrupted by the installer daemon hand-over")
		s.mu.Unlock()
		if err != nil {
			return handOverResult{err: fmt.Errorf("hand-over: %w", err)}
		}
		return handOverResult{over: true}
	default:
		log.Infof("hand-over: the experiment ended while the installer daemon was handed over, handing it back")
		err := handBackInstaller(ctx)
		s.mu.Unlock()
		return handOverResult{over: true, err: err}
	}
	s.mu.Unlock()
	jobPIDs, exited, err := resolveExperimentPids(ctx, installerJobSet())
	if err != nil {
		return handOverResult{err: fmt.Errorf("watcher: could not resolve the experiment daemon's pid: %w", err)}
	}
	if exited != nil || len(jobPIDs) == 0 {
		// The experiment daemon exited before it could be armed on: it fails the experiment.
		return handOverResult{daemonGone: true}
	}
	var pid int
	for p := range jobPIDs {
		pid = p
	}
	observer, exits, err := launchd.ArmExitObserver([]int{pid})
	if err != nil {
		return handOverResult{err: fmt.Errorf("watcher: could not arm exit observer: %w", err)}
	}
	return handOverResult{observer: observer, exits: exits, pid: pid}
}

// takeOver is W2's start: arm on every experiment job, then stop every other watcher, W1 included.
// The stop runs in the background: W1 keeps supervising for its signal grace, and owns the
// experiment daemon's exit until it is gone.
func (s *experimentSupervisor) takeOver(ctx context.Context) error {
	observer, exits, armed, reason, err := s.arm(ctx, configExperimentJobSet())
	if err != nil {
		return err
	}
	if reason != "" {
		return s.endWithFailure(ctx, reason)
	}
	s.agentObserver, s.agentExits, s.agentArmed = observer, exits, armed
	s.watchState()
	go func() {
		if err := stopOtherWatchers(ctx); err != nil {
			log.Warnf("watcher: could not stop the other watchers: %v", err)
		}
		s.predecessorGone.Store(true)
	}()
	log.Infof("watcher: took supervision over from the watcher the start hook launched")
	return nil
}

// owns reports whether this watcher acts on a failure of the job label, without its variant suffix;
// "" stands for a failure no job reported, like the deadline expiring. See watchExperiment.
func (s *experimentSupervisor) owns(ctx context.Context, label string) bool {
	if s.role == secondWatcher {
		if label != installerJob || s.predecessorGone.Load() {
			return true
		}
		running, err := watcherRunning(ctx)
		return err != nil || !running
	}
	if label == installerJob {
		return true
	}
	running, err := watcherRunning(ctx)
	return err != nil || !running
}

// deferFailure leaves a failure this watcher does not own to the other watcher: it waits, for as
// long as the other watcher can take to act, for the experiment to end, and acts itself once the
// other watcher is gone and the experiment is still this one. It reports whether the experiment was
// ended from here.
func (s *experimentSupervisor) deferFailure(ctx context.Context, label, reason string) (bool, error) {
	wait := watcherOverlap
	if s.role == secondWatcher {
		wait = watcherStopTimeout
	}
	giveUp := time.Now().Add(wait)
	for time.Now().Before(giveUp) {
		current, err := s.deadline.Read()
		if err != nil {
			return false, fmt.Errorf("watcher: could not read experiment deadline: %w", err)
		}
		if current != s.token {
			// The owner ended it, or a stop or promote is under way: the next check decides.
			return false, nil
		}
		if s.owns(ctx, label) {
			break
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-time.After(deferPoll):
		}
	}
	return s.onFailingExit(ctx, reason)
}

// run is the supervisor's main loop, until the experiment ends.
func (s *experimentSupervisor) run(ctx context.Context) error {
	ticker := time.NewTicker(deadlineTickInterval)
	defer ticker.Stop()

	readFailures := 0
	revertingTicks := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-s.agentExits:
			if !ok {
				if s.agentDelivered < s.agentArmed {
					return errors.New("watcher: the exit observer stopped before every experiment job exited")
				}
				s.agentExits = nil
				continue
			}
			s.agentDelivered++
			if done, err := s.onExit(ctx, ev); done || err != nil {
				return err
			}
		case ev, ok := <-s.daemonExits:
			if !ok {
				if s.daemonDelivered < s.daemonArmed {
					return errors.New("watcher: the exit observer stopped before the experiment daemon exited")
				}
				s.daemonExits = nil
				continue
			}
			s.daemonDelivered++
			if done, err := s.onExit(ctx, ev); done || err != nil {
				return err
			}
		case result := <-s.handOver:
			if result.err != nil {
				return result.err
			}
			if result.over {
				s.finished = true
				return nil
			}
			if result.ended {
				continue
			}
			if result.daemonGone {
				if done, err := s.onFailingExit(ctx, "the experiment daemon exited before the watcher could observe it"); done || err != nil {
					return err
				}
				continue
			}
			s.daemonObserver, s.daemonExits, s.daemonArmed = result.observer, result.exits, 1
			s.pids[result.pid] = installerJob
		case <-s.stateChanges:
			if done, err := s.onDeadline(ctx, false, &readFailures, &revertingTicks); done || err != nil {
				return err
			}
		case <-ticker.C:
			if done, err := s.onDeadline(ctx, true, &readFailures, &revertingTicks); done || err != nil {
				return err
			}
		}
	}
}

// onDeadline acts on the deadline file, on every tick and on every change to its directory. It
// reports whether supervision is over.
func (s *experimentSupervisor) onDeadline(ctx context.Context, tick bool, readFailures, revertingTicks *int) (bool, error) {
	current, err := s.deadline.Read()
	if err != nil {
		*readFailures++
		if *readFailures >= watcherDeadlineReadFailures {
			return false, fmt.Errorf("watcher: could not read experiment deadline %d times in a row: %w", *readFailures, err)
		}
		log.Errorf("watcher: could not read experiment deadline: %v", err)
		return false, nil
	}
	*readFailures = 0
	switch current {
	case s.token:
		*revertingTicks = 0
		return s.checkDeadline(ctx, readFailures)
	case launchd.DeadlineReverting:
		// A stop or promote under way, or one whose command died partway. Seen on two ticks in a
		// row, it is finished from here once no command holds the lock. A change to the directory
		// alone is the hook at work, and is left to it.
		if !tick {
			return false, nil
		}
		*revertingTicks++
		if *revertingTicks < 2 || !s.owns(ctx, "") {
			return false, nil
		}
		return s.finishUnlessEnded(ctx, "a stop or promote left the jobs half handed back")
	case "":
		// A stop or promote finished handing the Agent back: hand the daemon back once its command
		// has exited, unless the other watcher owns that.
		if !s.owns(ctx, "") {
			return false, nil
		}
		return true, s.handBack(ctx)
	default:
		// Failed from elsewhere, or replaced by a newer experiment with its own watcher.
		return true, nil
	}
}

// checkDeadline acts on a deadline still this experiment's: an expired deadline reverts the
// experiment, and so does a configuration promoted by a promote whose hook never handed the jobs
// back.
func (s *experimentSupervisor) checkDeadline(ctx context.Context, readFailures *int) (bool, error) {
	expired, err := s.deadline.Expired(configExperimentDeadlineWindow)
	if err != nil {
		*readFailures++
		if *readFailures >= watcherDeadlineReadFailures {
			return false, fmt.Errorf("watcher: could not check experiment deadline %d times in a row: %w", *readFailures, err)
		}
		log.Errorf("watcher: could not check experiment deadline: %v", err)
		return false, nil
	}
	var reason string
	switch {
	case expired:
		reason = "experiment deadline expired"
	case !experimentConfigDeployed():
		// A promote that published the configuration but whose hook never handed the jobs back.
		// The revert waits for a promote still under way.
		reason = "experiment configuration no longer deployed"
	default:
		return false, nil
	}
	if !s.owns(ctx, "") {
		return s.deferFailure(ctx, "", reason)
	}
	return s.onFailingExit(ctx, reason)
}

// onExit acts on an armed job's exit: one that fails the experiment ends it, from here if this
// watcher owns it (onFailingExit), from the other watcher otherwise (deferFailure); any other exit is
// part of the job's own lifecycle.
func (s *experimentSupervisor) onExit(ctx context.Context, ev launchd.ExitEvent) (bool, error) {
	label := s.pids[ev.Pid]
	if !exitFailsExperiment(label, exitedCleanly(ev.Status)) {
		return false, nil
	}
	reason := fmt.Sprintf("experiment job %s (pid %d) exited (status %d)", label, ev.Pid, ev.Status)
	if !s.owns(ctx, label) {
		return s.deferFailure(ctx, label, reason)
	}
	return s.onFailingExit(ctx, reason)
}

// onFailingExit ends the experiment after something failed it, unless a stop or promote owns the
// outcome. A deadline that still holds this experiment's token is reverted. The reverting marker is
// a stop or promote under way, whose hook is what made the jobs exit, or one that died partway: it is
// finished once no command holds the lock and the marker is still there (finishUnlessEnded). Any
// other deadline means the experiment is over, and the next deadline check hands the daemon back.
// It reports whether the experiment was ended from here.
func (s *experimentSupervisor) onFailingExit(ctx context.Context, reason string) (bool, error) {
	for {
		current, err := s.deadline.Read()
		if err != nil {
			return false, fmt.Errorf("watcher: could not read experiment deadline: %w", err)
		}
		switch current {
		case s.token:
			reverted, err := s.revert(ctx, s.token, reason)
			if err != nil || reverted {
				s.finished = reverted
				return reverted, err
			}
			// The deadline changed while waiting for the lock: look again.
		case launchd.DeadlineReverting:
			return s.finishUnlessEnded(ctx, reason+", while a stop or promote was handing the jobs back")
		default:
			return false, nil
		}
	}
}

// finishUnlessEnded finishes a stop or promote that left the reverting marker, once no installer
// command holds the lock. A hook that is only under way clears the marker before its command exits,
// and is left to end the experiment.
func (s *experimentSupervisor) finishUnlessEnded(ctx context.Context, reason string) (bool, error) {
	reverted, err := s.revert(ctx, launchd.DeadlineReverting, reason)
	if err != nil {
		return false, err
	}
	s.finished = reverted
	return reverted, nil
}

// revert reverts every job, the daemon included, if the deadline still holds token once the
// installer lock is free (revertFromWatcher).
func (s *experimentSupervisor) revert(ctx context.Context, token, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return revertFromWatcher(ctx, s.deadline, token, reason)
}

// handBack hands the installer daemon back once a stop or promote has handed the Agent back, and
// ends supervision.
func (s *experimentSupervisor) handBack(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = true
	if err := handBackInstaller(ctx); err != nil {
		log.Errorf("watcher: could not hand the installer daemon back: %v", err)
	}
	return nil
}

// endWithFailure ends an experiment that failed before it could be armed on.
func (s *experimentSupervisor) endWithFailure(ctx context.Context, reason string) error {
	_, err := s.onFailingExit(ctx, reason)
	s.finished = true
	return err
}

// experimentConfigDeployed reports whether the experiment configuration directory is still
// deployed. A state that cannot be read counts as deployed: this check only ever ends an experiment
// early, and the deadline still bounds it.
func experimentConfigDeployed() bool {
	state, err := configExperimentDirs.GetState()
	if err != nil {
		log.Warnf("watcher: could not read the configuration state: %v", err)
		return true
	}
	return state.ExperimentDeploymentID != ""
}

// failSafeRevert reverts the experiment the watcher can no longer supervise, cause saying why, and
// retries a few times, with a doubling backoff, before giving up. token is the deadline the
// watcher read on start, or "" when it could not read one.
//
// It reverts under the installer lock, like revertFromWatcher, and skips the revert when the
// experiment is over by then -- the deadline cleared or holding another experiment's token. The
// reverting marker does not count as over: it is what a failed restore leaves behind, including
// this function's own previous attempt, so the next attempt restores again rather than giving up. A
// deadline it cannot read does not stop it: an experiment nobody can tell the state of is reverted.
// When the lock itself cannot be taken, for any reason other than a running command, no installer
// command can run either, so the last attempt reverts without it.
//
// A watcher stopped on purpose (ctx cancelled: another watcher or the post-install stopping it)
// reverts nothing.
// Whatever this cannot revert is left with the reverting marker, for Fleet's stop to finish.
func failSafeRevert(ctx context.Context, deadline launchd.Deadline, token string, cause error) error {
	log.Errorf("watcher: cannot keep supervising the configuration experiment, reverting it: %v", cause)
	stillOurs := func() (bool, error) {
		current, err := deadline.Read()
		if err != nil {
			log.Warnf("watcher: could not read experiment deadline, reverting regardless: %v", err)
			return true, nil
		}
		if current == launchd.DeadlineReverting {
			// A restore that did not finish: this fail-safe's own earlier attempt, or a stop
			// that failed partway. Either way the stable set is not back, and restoring it again
			// is idempotent, so it is still this watcher's to finish.
			return true, nil
		}
		if token == "" {
			return experimentPending(current), nil
		}
		return current == token, nil
	}
	revert := func() (bool, error) {
		if ours, _ := stillOurs(); !ours {
			return false, nil
		}
		log.Warnf("watcher: reverting configuration experiment: %v", cause)
		return true, revertExperiment(ctx, configExperiment{jobs: configExperimentJobSet()})
	}

	var err error
	backoff := watcherFailSafeBackoff
	for attempt := 0; attempt < watcherFailSafeAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return cause
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		if ctx.Err() != nil {
			return cause
		}
		var lock *db.PackagesDB
		lock, err = waitForInstallerLock(ctx, stillOurs)
		if err != nil {
			log.Warnf("watcher: fail-safe revert attempt %d: %v", attempt+1, err)
			continue
		}
		if lock == nil {
			// The experiment ended while waiting for a running installer command.
			return nil
		}
		var reverted bool
		reverted, err = revert()
		if closeErr := lock.Close(); closeErr != nil {
			log.Warnf("watcher: could not release the installer lock: %v", closeErr)
		}
		if err == nil {
			if !reverted {
				return nil
			}
			return fmt.Errorf("watcher stopped supervising, experiment reverted: %w", cause)
		}
		log.Warnf("watcher: fail-safe revert attempt %d: %v", attempt+1, err)
	}
	if errors.Is(err, errInstallerLock) {
		log.Warnf("watcher: the installer lock is unusable, reverting without it")
		if _, err = revert(); err == nil {
			return fmt.Errorf("watcher stopped supervising, experiment reverted: %w", cause)
		}
	}
	return fmt.Errorf("watcher stopped supervising (%w) and could not revert the experiment: %w", cause, err)
}

// resolveExperimentPids waits for every job in the set to either report a running pid or a
// recorded exit, bounded by startupPollTimeout. Kickstart was already issued for every job in the
// set before this runs -- the start hook starts the agent jobs before it launches W1, and the
// hand-over or the experiment daemon's own start precedes the arming on the daemon -- so a healthy
// job should acquire a pid almost immediately; the bound only covers the gap between that request
// and this process actually getting to check.
//
// It returns the running jobs' pids, each mapped to its label without the variant suffix, which is
// what exitFailsExperiment is keyed on. A job still pending once startupPollTimeout is up is an
// error: it failed to start, and it could not be watched if it started later.
//
// A job that launchd itself recorded as having exited in a way that fails the experiment
// (exitFailsExperiment) stops the wait immediately and is returned in exited, regardless of its
// siblings' state: that is a durable fact recorded by launchd, not something more waiting would
// resolve. Any other job recorded as having exited cleanly (LastExitStatus zero) is not: a job
// whose feature is off exits that way the instant it is kickstarted, same as the stable set, and
// this wait must tolerate it, or every experiment with one disabled feature reverts itself on
// startup before the caller ever gets a chance to observe it running. Unlike exitedCleanly's
// wait(2) status word, LastExitStatus is launchd's own simple signed code, so a direct zero check
// is what it takes to tell a clean exit apart here.
func resolveExperimentPids(ctx context.Context, jobs launchd.JobSet) (jobPIDs map[int]string, exited *launchd.JobStatus, err error) {
	giveUp := time.Now().Add(startupPollTimeout)
	for {
		statuses, err := jobs.Statuses(ctx, launchd.Experiment)
		if err != nil {
			return nil, nil, err
		}
		jobPIDs = make(map[int]string, len(statuses))
		var pending []string
		for i, status := range statuses {
			label := strings.TrimSuffix(status.Label, string(launchd.Experiment))
			if status.PID != 0 {
				jobPIDs[status.PID] = label
				continue
			}
			if status.HasExited && exitFailsExperiment(label, status.LastExitStatus == 0) {
				return nil, &statuses[i], nil
			}
			if status.HasExited {
				continue
			}
			pending = append(pending, status.Label)
		}
		if len(pending) == 0 {
			return jobPIDs, nil, nil
		}
		if time.Now().After(giveUp) {
			return nil, nil, fmt.Errorf("experiment jobs %s did not start within %s", strings.Join(pending, ", "), startupPollTimeout)
		}
		select {
		case <-ctx.Done():
			return jobPIDs, nil, nil
		case <-time.After(startupPollInterval):
		}
	}
}

// essentialJobs are the jobs that must keep running for as long as an experiment does: the Agent,
// and the installer daemon, the only process that can stop or promote the experiment. Either one
// exiting fails the experiment, even cleanly: an experiment left with no Agent would otherwise run
// on, and could be promoted, until its deadline, and one left with no daemon could not be ended
// before it. The other jobs exit cleanly by design when their feature is off.
var essentialJobs = []string{agentJob, installerJob}

// exitFailsExperiment reports whether an experiment job exiting, cleanly or not, fails the
// experiment. label is the job's label without its variant suffix.
func exitFailsExperiment(label string, clean bool) bool {
	return !clean || slices.Contains(essentialJobs, label)
}

// exitedCleanly reports whether a wait(2) status word (as delivered by NOTE_EXITSTATUS) describes
// a normal, voluntary exit(0). For a job outside essentialJobs, whose feature may be off, that is
// expected lifecycle, not a crash, and must not cost the rest of the set an otherwise-healthy
// experiment.
//
// A pid that had already exited before ArmExitObserver could arm it also reports status 0 (its
// real exit status is unrecoverable by then, see ArmExitObserver's doc comment on that residual
// race): this treats that case the same as a clean exit rather than a crash, narrowing an already
// zero-width, documented race rather than introducing a new one. For an essential job either way
// is a failure.
func exitedCleanly(status int) bool {
	ws := unix.WaitStatus(status)
	return ws.Exited() && ws.ExitStatus() == 0
}

// revertFromWatcher is revertExperimentIfStillPending for the detached watcher. It first takes the
// lock every installer command holds for its whole run, so the deadline it re-reads can't be
// cleared, and the directories it discards can't be swapped, by a stop or promote running at the
// same time.
func revertFromWatcher(ctx context.Context, deadline launchd.Deadline, token string, reason string) (bool, error) {
	lock, ok, err := waitForInstaller(ctx, deadline, token)
	if err != nil || !ok {
		return false, err
	}
	defer func() {
		if err := lock.Close(); err != nil {
			log.Warnf("watcher: could not release the installer lock: %v", err)
		}
	}()
	return revertExperimentIfStillPending(ctx, configExperimentJobSet(), deadline, token, reason)
}

// waitForInstaller takes the installer's lock, waiting for any installer command that holds it. It
// gives up without the lock, and reports false, once the deadline no longer holds token: whatever
// held the lock stopped, promoted or replaced the experiment, so there is nothing left to revert.
func waitForInstaller(ctx context.Context, deadline launchd.Deadline, token string) (*db.PackagesDB, bool, error) {
	lock, err := waitForInstallerLock(ctx, func() (bool, error) {
		current, err := deadline.Read()
		if err != nil {
			return false, fmt.Errorf("could not read experiment deadline: %w", err)
		}
		return current == token, nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("watcher: %w", err)
	}
	return lock, lock != nil, nil
}

// revertExperimentIfStillPending reverts jobs to the stable job set unless the deadline file no longer
// holds token: cleared by a deliberate stop or promote, in which case that path owns the outcome,
// or rewritten by a newer experiment that is not this caller's to revert. Either way this call is
// a no-op. Reports whether it reverted.
//
// The experiment configuration is kept (revertExperiment): Fleet stops the failed experiment, and
// that stop discards it through the installer, as for any stop.
func revertExperimentIfStillPending(ctx context.Context, jobs launchd.JobSet, deadline launchd.Deadline, token string, reason string) (bool, error) {
	current, err := deadline.Read()
	if err != nil {
		return false, fmt.Errorf("watcher: could not read experiment deadline: %w", err)
	}
	if current != token {
		return false, nil
	}
	log.Warnf("watcher: reverting configuration experiment: %s", reason)
	if err := revertExperiment(ctx, configExperiment{jobs: jobs}); err != nil {
		return false, fmt.Errorf("watcher: %w", err)
	}
	return true, nil
}

// revertExperiment hands jobs back to the stable job set after the experiment failed on the host,
// and keeps its configuration deployed for Fleet to stop (configExperiment.Fail). The host goes on
// reporting the experiment, with the stable daemon's config_id as the running configuration, so
// Fleet's health check rolls it back.
//
// The supervising watcher and the fallback hand-back revert every job (configExperimentJobSet): they
// run outside the installer daemon's job, so they can swap the daemon too (see installerJob).
//
// An experiment whose configuration is no longer deployed while its deadline is still pending is a
// promote whose hook never ran: the configuration was promoted, the jobs were not handed back. It is
// finished as a promote, so the process manager is restarted onto the promoted configuration.
func revertExperiment(ctx context.Context, e configExperiment) error {
	if !experimentConfigDeployed() {
		if err := e.Promote(context.WithoutCancel(ctx)); err != nil {
			return fmt.Errorf("could not finish the promoted experiment: %w", err)
		}
		return discardExperimentConfig(ctx)
	}
	if err := e.Fail(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("could not revert experiment: %w", err)
	}
	return nil
}

// revertAndDiscardExperiment hands jobs back to the stable job set and discards the experiment
// configuration, leaving the host as if no experiment had ever been started. It is for an
// experiment whose start fails, or that nothing will ever report or stop: the installer does not
// discard what it published when a start fails.
func revertAndDiscardExperiment(ctx context.Context, e configExperiment) error {
	if err := e.Stop(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("could not revert experiment: %w", err)
	}
	return discardExperimentConfig(ctx)
}
