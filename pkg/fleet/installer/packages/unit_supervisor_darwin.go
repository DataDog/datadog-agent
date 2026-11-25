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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The unit supervisor gives the macOS launchd jobs the systemd semantics the Linux configuration
// experiment relies on (datadog-agent.service.tmpl and its children):
//
//   - the experiment Agent wants every other experiment job, the installer daemon last, and
//     conflicts with the stable Agent: starting the experiment stops the stable set;
//   - every other experiment job is bound to the experiment Agent: it stops when that stops;
//   - no experiment job is relaunched. Any exit that fails the experiment (exitFailsExperiment: a
//     failed exit of any job, or any exit of the Agent or the installer daemon), or the experiment
//     timing out (EXPERIMENT_TIMEOUT), starts the stable set again (OnFailure). The experiment
//     configuration stays deployed until Fleet's stop. Unlike Linux, where the other jobs are
//     Restart=on-failure, a crash of any of them fails the experiment: Fleet's health check only
//     sees the installer daemon's configuration, so a configuration under which a job cannot run
//     would otherwise be promoted.
//
// It runs as its own launchd job, outside both installer daemon jobs, so it can replace either
// daemon. The hooks talk to it through a target file and wait for its status file
// (requestUnits), the way the Linux hooks call systemctl.

const (
	// unitSupervisorJob is the supervisor's launchd job. It has no experiment variant, starts only
	// when a hook kickstarts it, and is relaunched by launchd if it crashes (KeepAlive Crashed,
	// which unlike SuccessfulExit does not imply RunAtLoad).
	unitSupervisorJob = "com.datadoghq.unit-supervisor"

	// superviseUnitsCommand is the package command the supervisor job runs.
	superviseUnitsCommand = "superviseUnits"

	// The targets a hook can ask the supervisor for.
	unitsTargetExperiment    = "experiment"
	unitsTargetStable        = "stable"
	unitsTargetRestartStable = "restart-stable"
	// unitsTargetFailed is what the supervisor leaves once the experiment failed on the host, so a
	// relaunched supervisor does not start it again.
	unitsTargetFailed = "failed"

	// The phases the supervisor reports for a request.
	unitsPhaseAgents = "agents" // the Agent jobs are on the target; the installer daemon follows
	unitsPhaseDone   = "done"   // every job, the installer daemon included, is on the target
	unitsPhaseFailed = "failed" // the request could not be applied, or the experiment failed
)

var (
	// unitsTargetPath and unitsStatusPath sit beside the deadline, with its trust rules. Vars so
	// tests can point them at temporary files.
	unitsTargetPath = filepath.Join(paths.ConfigExperimentStatePath, "units-target")
	unitsStatusPath = filepath.Join(paths.ConfigExperimentStatePath, "units-status")

	// unitsRequestTimeout bounds how long a hook waits for the supervisor to apply its request.
	unitsRequestTimeout = 5 * time.Minute
	// unitsPollInterval is how often a hook checks the supervisor's status, and how often the
	// supervisor checks its deadline and target when nothing else wakes it.
	unitsPollInterval = 200 * time.Millisecond
	// unitsTickInterval is the supervisor's slow tick, for the deadline.
	unitsTickInterval = 30 * time.Second
	// unitsRetryDelay is how long the supervisor waits before retrying a step that failed.
	unitsRetryDelay = 10 * time.Second
)

func unitsTargetFile() launchd.Deadline {
	return launchd.Deadline{Path: unitsTargetPath, OwnerUID: configExperimentStateOwner}
}

func unitsStatusFile() launchd.Deadline {
	return launchd.Deadline{Path: unitsStatusPath, OwnerUID: configExperimentStateOwner}
}

// unitsRequest is a target and the nonce a hook asked for it under.
type unitsRequest struct {
	target, nonce string
}

// unitsStatus is the supervisor's progress on the request nonce.
type unitsStatus struct {
	nonce, phase, message string
}

func readUnitsRequest() (unitsRequest, error) {
	raw, err := unitsTargetFile().Read()
	if err != nil {
		return unitsRequest{}, err
	}
	target, nonce, _ := strings.Cut(raw, " ")
	return unitsRequest{target: target, nonce: nonce}, nil
}

func writeUnitsRequest(r unitsRequest) error {
	return unitsTargetFile().WriteRaw(r.target + " " + r.nonce)
}

func readUnitsStatus() (unitsStatus, error) {
	raw, err := unitsStatusFile().Read()
	if err != nil {
		return unitsStatus{}, err
	}
	fields := strings.SplitN(raw, " ", 3)
	for len(fields) < 3 {
		fields = append(fields, "")
	}
	return unitsStatus{nonce: fields[0], phase: fields[1], message: fields[2]}, nil
}

// unitsStatusEllipsis marks a status message cut to fit the status file.
const unitsStatusEllipsis = "..."

func writeUnitsStatus(nonce, phase, message string) {
	line := strings.TrimSpace(nonce + " " + phase + " " + message)
	if len(line) > launchd.MaxDeadlineSize {
		// The status is read with the deadline's size limit: a longer one would be refused, by the
		// supervisor and by every hook, until a reinstall. The message is only informative.
		log.Warnf("unit supervisor: status message cut to fit its file: %s", message)
		cut := launchd.MaxDeadlineSize - len(unitsStatusEllipsis)
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		line = line[:cut] + unitsStatusEllipsis
	}
	if err := unitsStatusFile().WriteRaw(line); err != nil {
		log.Errorf("unit supervisor: could not write its status: %v", err)
	}
}

// requestUnits asks the supervisor for target and waits until the Agent jobs are on it. The
// installer daemon is replaced afterwards, once the installer command that runs the hook has
// exited, so the hook never stops the daemon it runs in.
var requestUnits = func(ctx context.Context, target string) error {
	request := unitsRequest{target: target, nonce: strconv.FormatInt(time.Now().UnixNano(), 10)}
	if err := writeUnitsRequest(request); err != nil {
		return fmt.Errorf("could not write the unit supervisor's target: %w", err)
	}
	if err := ensureUnitSupervisor(ctx); err != nil {
		return err
	}
	giveUp := time.Now().Add(unitsRequestTimeout)
	for {
		status, err := readUnitsStatus()
		if err != nil {
			return fmt.Errorf("could not read the unit supervisor's status: %w", err)
		}
		if status.nonce == request.nonce {
			switch status.phase {
			case unitsPhaseAgents, unitsPhaseDone:
				return nil
			case unitsPhaseFailed:
				return fmt.Errorf("could not apply %s: %s", target, status.message)
			}
		}
		if time.Now().After(giveUp) {
			return fmt.Errorf("the unit supervisor did not apply %s within %s", target, unitsRequestTimeout)
		}
		// A supervisor that exited between the target being written and being kickstarted, after
		// finishing an earlier request, is started again.
		if running, err := unitSupervisorRunning(ctx); err == nil && !running {
			if err := ensureUnitSupervisor(ctx); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(unitsPollInterval):
		}
	}
}

// withdrawUnitsRequest replaces a request the hook gives up on with the stable target, and starts
// the supervisor without waiting for it. A supervisor still applying the experiment sees the target
// change and brings the stable jobs back; one with nothing left to do finds them running and exits.
// Failures are logged only: the hook already fails.
func withdrawUnitsRequest(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)
	request := unitsRequest{target: unitsTargetStable, nonce: strconv.FormatInt(time.Now().UnixNano(), 10)}
	if err := writeUnitsRequest(request); err != nil {
		log.Errorf("could not ask the unit supervisor for the stable jobs: %v", err)
		return
	}
	if err := ensureUnitSupervisor(ctx); err != nil {
		log.Errorf("could not start the unit supervisor to bring the stable jobs back: %v", err)
	}
}

// ensureUnitSupervisor writes the supervisor's job definition (embedded/tmpl/
// com.datadoghq.unit-supervisor.plist.tmpl), loads it if it is not, and starts it. Starting a
// supervisor that runs already does nothing: it sees the new target by itself.
var ensureUnitSupervisor = func(ctx context.Context) error {
	client := installerLaunchdClient()
	set := launchd.JobSet{Labels: []string{unitSupervisorJob}, Dir: launchdJobDir, Client: client}
	job := set.Job(unitSupervisorJob, launchd.Stable)
	changed, err := job.WriteIfChanged(embedded.GetLaunchdUnitSupervisorJob())
	if err != nil {
		return fmt.Errorf("could not write the unit supervisor's job definition: %w", err)
	}
	loaded, err := client.Loaded(ctx, unitSupervisorJob)
	if err != nil {
		return fmt.Errorf("could not tell whether the unit supervisor is loaded: %w", err)
	}
	if changed && loaded {
		// launchd caches the definition it loaded; a stale one is replaced, unless it runs.
		if running, err := unitSupervisorRunning(ctx); err == nil && !running {
			if err := client.Bootout(ctx, unitSupervisorJob); err != nil {
				return fmt.Errorf("could not reload the unit supervisor: %w", err)
			}
			loaded = false
		}
	}
	if !loaded {
		if err := client.Bootstrap(ctx, job); err != nil {
			return fmt.Errorf("could not load the unit supervisor: %w", err)
		}
	}
	if err := client.Kickstart(ctx, unitSupervisorJob, false); err != nil {
		return fmt.Errorf("could not start the unit supervisor: %w", err)
	}
	return nil
}

func unitSupervisorRunning(ctx context.Context) (bool, error) {
	status, err := launchdClient().Print(ctx, unitSupervisorJob)
	return status.PID != 0, err
}

// unloadUnitSupervisor stops the supervisor and removes its job definition, for an install or an
// uninstall that discards whatever experiment is left. It runs before any other job is unloaded: a
// running supervisor would read the experiment jobs going away as a failure and start jobs on top
// of the install.
func unloadUnitSupervisor(ctx context.Context) {
	set := launchd.JobSet{Labels: []string{unitSupervisorJob}, Dir: launchdJobDir, Client: launchdClient()}
	if err := set.Client.Bootout(ctx, unitSupervisorJob); err != nil {
		log.Warnf("could not unload the unit supervisor: %v", err)
	}
	if err := set.Remove(launchd.Stable); err != nil {
		log.Warnf("could not remove the unit supervisor's job definition: %v", err)
	}
}

// clearUnitsState removes the supervisor's target and status files.
func clearUnitsState() {
	for _, f := range []launchd.Deadline{unitsTargetFile(), unitsStatusFile()} {
		if err := f.Clear(); err != nil {
			log.Warnf("could not remove %s: %v", f.Path, err)
		}
	}
}

// errTargetChanged ends the work on a request a newer one replaced.
var errTargetChanged = errors.New("the target changed")

// superviseUnits is the supervisor's main loop: it applies the target the hooks asked for, and
// supervises a running experiment, until there is nothing left to do, then exits. A step that fails
// is retried after unitsRetryDelay; every step resumes from launchd's state, which is also what a
// supervisor relaunched after a crash does.
//
// It never returns an error, and recovers a panic in a step: launchd relaunches the supervisor only
// after a crash signal (KeepAlive Crashed), so an error exit would leave the experiment unsupervised.
// A Go fatal error, or a panic in another goroutine, would exit with status 2 the same way: the job
// sets GOTRACEBACK=crash so the runtime aborts with SIGABRT instead, which launchd relaunches. A
// SIGKILL is still not relaunched; the next hook that asks for the supervisor, or a reboot, recovers
// from it.
func superviseUnits(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			// Stopped on purpose: by an install, an uninstall, or launchd's bootout.
			return nil
		}
		done, err := superviseUnitsStep(ctx)
		if done {
			return nil
		}
		if errors.Is(err, errTargetChanged) {
			continue
		}
		if err != nil {
			log.Errorf("unit supervisor: %v; retrying in %s", err, unitsRetryDelay)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(unitsRetryDelay):
			}
		}
	}
}

// superviseUnitsStep applies the current target once. It reports done when there is nothing left
// to do, and turns a panic into an error so the loop retries it.
func superviseUnitsStep(ctx context.Context) (done bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			done, err = false, fmt.Errorf("unit supervisor: panic: %v", r)
		}
	}()
	request, err := readUnitsRequest()
	if err != nil {
		return false, fmt.Errorf("unit supervisor: could not read its target: %w", err)
	}
	status, err := readUnitsStatus()
	if err != nil {
		return false, fmt.Errorf("unit supervisor: could not read its status: %w", err)
	}
	finished := status.nonce == request.nonce && (status.phase == unitsPhaseDone || status.phase == unitsPhaseFailed)
	switch request.target {
	case unitsTargetExperiment:
		if status.nonce == request.nonce && status.phase == unitsPhaseFailed {
			return true, nil
		}
		err = runExperiment(ctx, request, status)
	case unitsTargetStable, unitsTargetRestartStable:
		if finished {
			return true, nil
		}
		err = applyStable(ctx, request, request.target == unitsTargetRestartStable)
	default:
		return true, nil
	}
	if err != nil {
		return false, err
	}
	latest, err := readUnitsRequest()
	return err == nil && latest == request, nil
}

// targetUnchanged reports whether request is still the one the hooks asked for.
func targetUnchanged(request unitsRequest) func() (bool, error) {
	return func() (bool, error) {
		latest, err := readUnitsRequest()
		return latest == request, err
	}
}

// runExperiment starts the experiment request asks for, picking up where a relaunched supervisor
// left it, then supervises it until it ends.
func runExperiment(ctx context.Context, request unitsRequest, status unitsStatus) error {
	agents := configExperiment{jobs: agentJobSet()}
	if status.nonce != request.nonce {
		if err := experimentDeadline().Write(configExperimentDeadlineWindow); err != nil {
			writeUnitsStatus(request.nonce, unitsPhaseFailed, err.Error())
			return nil
		}
		// Wants and Conflicts: the experiment Agent and its jobs start, the stable ones stop.
		if err := agents.Start(ctx); err != nil {
			_ = experimentDeadline().Clear()
			writeUnitsStatus(request.nonce, unitsPhaseFailed, err.Error())
			return nil
		}
		writeUnitsStatus(request.nonce, unitsPhaseAgents, "")
		status.phase = unitsPhaseAgents
	}
	if status.phase == unitsPhaseAgents {
		// The installer daemon is an experiment job too, but the start command runs inside the
		// stable daemon: it is replaced once that command has exited.
		ok, err := waitForInstallerCommand(ctx, targetUnchanged(request))
		if err != nil {
			return err
		}
		if !ok {
			return errTargetChanged
		}
		if err := swapInstallerDaemon(ctx, launchd.Stable, launchd.Experiment); err != nil {
			log.Errorf("unit supervisor: %v", err)
			return failExperiment(ctx, request, err.Error())
		}
		writeUnitsStatus(request.nonce, unitsPhaseDone, "")
	}
	return superviseExperiment(ctx, request)
}

// childExitHandled is called once the supervisor has handled a child job's exit that did not fail
// the experiment. Tests replace it to wait on that.
var childExitHandled = func(childExit) {}

// childExit is the exit of one of the experiment Agent's child jobs.
type childExit struct {
	label  string
	status int
}

// superviseExperiment waits for an experiment job to exit, the deadline to pass or the target to
// change. An exit that fails the experiment (exitFailsExperiment) or the deadline passing fails it
// (OnFailure): any exit of the Agent or the installer daemon, and a failed exit of any other job.
// A job that exits cleanly, as one whose feature is off does, stays down. No job is restarted.
func superviseExperiment(ctx context.Context, request unitsRequest) error {
	main := launchd.JobSet{Labels: []string{agentJob}, Dir: launchdJobDir, Client: launchdClient()}
	pids, exited, err := resolveExperimentPids(ctx, main)
	if err != nil {
		return fmt.Errorf("unit supervisor: could not find the experiment Agent: %w", err)
	}
	if exited != nil || len(pids) == 0 {
		return failExperiment(ctx, request, "the experiment Agent is not running")
	}
	var pid int
	for p := range pids {
		pid = p
	}
	observer, exits, err := launchd.ArmExitObserver([]int{pid})
	if err != nil {
		return fmt.Errorf("unit supervisor: could not watch the experiment Agent: %w", err)
	}
	defer func() { _ = observer.Disarm() }()

	// A child that already exited, or that cannot be watched, fails the experiment like one that
	// exits while watched: its exit would otherwise go unseen.
	children := make(chan childExit, len(configExperimentJobs))
	var childObservers []*launchd.ExitObserver
	defer func() {
		for _, o := range childObservers {
			_ = o.Disarm()
		}
	}()
	for _, label := range experimentChildJobs() {
		o, err := armChildJob(ctx, label, children)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return failExperiment(ctx, request, fmt.Sprintf("%s: %v", label+string(launchd.Experiment), err))
		}
		if o != nil {
			childObservers = append(childObservers, o)
		}
	}

	changes, stopWatch, err := watchExperimentState()
	if err != nil {
		log.Warnf("unit supervisor: watching its target on a tick only: %v", err)
	} else {
		defer stopWatch()
	}
	ticker := time.NewTicker(unitsTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-exits:
			if !ok {
				return errors.New("unit supervisor: the exit observer stopped")
			}
			if unchanged, _ := targetUnchanged(request)(); !unchanged {
				// A stop or promote stopped the Agent: it is applied on the next turn.
				return errTargetChanged
			}
			return failExperiment(ctx, request, fmt.Sprintf("the experiment Agent exited (status %d)", ev.Status))
		case child := <-children:
			if unchanged, _ := targetUnchanged(request)(); !unchanged {
				return errTargetChanged
			}
			if exitFailsExperiment(child.label, exitedCleanly(child.status)) {
				return failExperiment(ctx, request, fmt.Sprintf("%s exited (status %d)", child.label+string(launchd.Experiment), child.status))
			}
			childExitHandled(child)
		case <-changes:
		case <-ticker.C:
		}
		if unchanged, err := targetUnchanged(request)(); err == nil && !unchanged {
			return errTargetChanged
		}
		if expired, err := experimentDeadline().Expired(configExperimentDeadlineWindow); err == nil && expired {
			return failExperiment(ctx, request, "the experiment timed out")
		}
	}
}

// experimentChildJobs are the experiment Agent's child jobs: every other experiment job, the
// installer daemon included.
func experimentChildJobs() []string {
	return withoutJob(configExperimentJobs, agentJob)
}

// armChildJob watches the child job label's running process, and delivers its exit on exits. A
// child that is not running because it exited cleanly at start, its feature being off, is not
// watched. One whose recorded exit fails the experiment, or that cannot be found or watched, is
// reported as an error.
func armChildJob(ctx context.Context, label string, exits chan<- childExit) (*launchd.ExitObserver, error) {
	set := launchd.JobSet{Labels: []string{label}, Dir: launchdJobDir, Client: launchdClient()}
	pids, exited, err := resolveExperimentPids(ctx, set)
	if err != nil {
		return nil, fmt.Errorf("could not find it: %w", err)
	}
	if exited != nil {
		return nil, fmt.Errorf("it exited before it could be watched (status %d)", exited.LastExitStatus)
	}
	var pid int
	for p := range pids {
		pid = p
	}
	if pid == 0 {
		return nil, nil
	}
	observer, events, err := launchd.ArmExitObserver([]int{pid})
	if err != nil {
		return nil, fmt.Errorf("could not watch it: %w", err)
	}
	go func() {
		for ev := range events {
			select {
			case exits <- childExit{label: label, status: ev.Status}:
			default:
				// Nobody supervises any more: the experiment ended.
			}
		}
	}()
	return observer, nil
}

// failExperiment is OnFailure: every job goes back to its stable variant, the installer daemon last,
// and the experiment configuration stays deployed until Fleet's stop. It waits for any running
// installer command first, so it never stops the daemon in the middle of one.
func failExperiment(ctx context.Context, request unitsRequest, reason string) error {
	log.Warnf("unit supervisor: the configuration experiment failed, starting the stable jobs: %s", reason)
	lock, err := waitForInstallerLock(ctx, targetUnchanged(request))
	if err != nil {
		return err
	}
	if lock == nil {
		return errTargetChanged
	}
	defer func() {
		if err := lock.Close(); err != nil {
			log.Warnf("unit supervisor: could not release the installer lock: %v", err)
		}
	}()
	restoreErr := configExperiment{jobs: configExperimentJobSet()}.restoreStable(context.WithoutCancel(ctx))
	ensureProcmgr(ctx, false)
	if restoreErr != nil {
		// Left as it is, the target still asks for the experiment: superviseUnits retries, finds the
		// experiment Agent gone and fails the experiment again.
		return fmt.Errorf("unit supervisor: could not start the stable jobs: %w", restoreErr)
	}
	if err := experimentDeadline().Clear(); err != nil {
		log.Warnf("unit supervisor: could not clear the deadline: %v", err)
	}
	if err := writeUnitsRequest(unitsRequest{target: unitsTargetFailed, nonce: request.nonce}); err != nil {
		log.Warnf("unit supervisor: could not record the failed experiment: %v", err)
	}
	writeUnitsStatus(request.nonce, unitsPhaseFailed, reason)
	return nil
}

// applyStable brings every job back to its stable variant: the Agent jobs first, then, once the
// installer command that asked for it has exited, the installer daemon. A stop leaves alone a stable
// job that already runs (after a reboot, or after the experiment failed on the host); a promote
// restarts every stable job so it reads the promoted configuration.
func applyStable(ctx context.Context, request unitsRequest, restart bool) error {
	ctx = context.WithoutCancel(ctx)
	agents := configExperiment{jobs: agentJobSet(), keepRunningStable: !restart}
	if err := agents.restoreStable(ctx); err != nil {
		writeUnitsStatus(request.nonce, unitsPhaseFailed, err.Error())
		return nil
	}
	ensureProcmgr(ctx, restart)
	writeUnitsStatus(request.nonce, unitsPhaseAgents, "")

	ok, err := waitForInstallerCommand(ctx, targetUnchanged(request))
	if err != nil {
		return err
	}
	if !ok {
		return errTargetChanged
	}
	status, err := launchdClient().Print(ctx, installerJob+string(launchd.Experiment))
	if err != nil {
		return fmt.Errorf("unit supervisor: could not tell whether the experiment daemon runs: %w", err)
	}
	switch {
	case status.PID != 0:
		if err := swapInstallerDaemon(ctx, launchd.Experiment, launchd.Stable); err != nil {
			return fmt.Errorf("unit supervisor: %w", err)
		}
	case status.Loaded:
		// Loaded without running: what launchd leaves of it after a reboot mid-experiment, or after it
		// died during the stop.
		if err := installerLaunchdClient().Bootout(ctx, installerJob+string(launchd.Experiment)); err != nil {
			log.Warnf("unit supervisor: could not unload the idle experiment daemon: %v", err)
		}
	}
	// Whatever happened to the experiment daemon, the host keeps a daemon: the stable one is started
	// when it is not loaded.
	if loaded, err := launchdClient().Loaded(ctx, installerJob); err == nil && !loaded {
		if err := startInstallerDaemon(ctx, installerJobSet(), launchd.Stable); err != nil {
			return fmt.Errorf("unit supervisor: could not start the stable installer daemon: %w", err)
		}
	}
	if err := installerJobSet().Remove(launchd.Experiment); err != nil {
		log.Warnf("unit supervisor: could not remove the experiment daemon's job definition: %v", err)
	}
	if err := experimentDeadline().Clear(); err != nil {
		log.Warnf("unit supervisor: could not clear the deadline: %v", err)
	}
	writeUnitsStatus(request.nonce, unitsPhaseDone, "")
	return nil
}

// swapInstallerDaemon replaces the installer daemon on its from job with the one on its to job. A
// launchd error starting the incoming daemon loads the outgoing one back, so the host is never left
// without a daemon.
func swapInstallerDaemon(ctx context.Context, from, to launchd.Variant) error {
	jobs := installerJobSet()
	if err := jobs.Client.Bootout(ctx, installerJob+string(from)); err != nil {
		return fmt.Errorf("could not stop the installer daemon: %w", err)
	}
	if err := startInstallerDaemon(ctx, jobs, to); err != nil {
		if restoreErr := startInstallerDaemon(ctx, jobs, from); restoreErr != nil {
			return fmt.Errorf("could not start the installer daemon (%w), nor restore the previous one: %w", err, restoreErr)
		}
		return fmt.Errorf("could not start the installer daemon: %w", err)
	}
	return nil
}

// startInstallerDaemon writes the installer daemon's definition for variant and loads it.
func startInstallerDaemon(ctx context.Context, jobs launchd.JobSet, variant launchd.Variant) error {
	definition, err := embedded.GetLaunchdJob(installerJob, variant)
	if err != nil {
		return fmt.Errorf("could not read the job definition for %s: %w", installerJob+string(variant), err)
	}
	if _, err := jobs.Job(installerJob, variant).WriteIfChanged(definition); err != nil {
		return err
	}
	return jobs.Start(ctx, variant)
}
