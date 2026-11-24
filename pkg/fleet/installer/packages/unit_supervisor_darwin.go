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
//   - every other experiment job is bound to the experiment Agent: it stops when that stops, and
//     the supervisor restarts it on a failed exit (Restart=on-failure). launchd cannot: a KeepAlive
//     with SuccessfulExit implies RunAtLoad, which would start the experiment jobs at boot;
//   - the experiment Agent is never relaunched, and any exit of it, or the experiment timing out
//     (EXPERIMENT_TIMEOUT), starts the stable set again (OnFailure). The experiment configuration
//     stays deployed until Fleet's stop.
//
// It runs as its own launchd job, outside both installer daemon jobs, so it can replace either
// daemon. The hooks talk to it through a target file and wait for its status file
// (requestUnits), the way the Linux hooks call systemctl.

const (
	// unitSupervisorJob is the supervisor's launchd job. It has no experiment variant, starts only
	// when a hook kickstarts it, and is relaunched by launchd if it crashes (KeepAlive Crashed,
	// which unlike SuccessfulExit does not imply RunAtLoad).
	unitSupervisorJob = "com.datadoghq.unit-supervisor"

	// unitSupervisorLogPath is where the supervisor logs, beside the other jobs' launchd logs
	// (LogDir in embedded/tmpl/main.go).
	unitSupervisorLogPath = "/opt/datadog-agent/logs/launchd_unit_supervisor.log"

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
	// unitsRestartDelay is how long a failed child job stays down before the supervisor restarts it,
	// and how long the supervisor waits before retrying a step that failed.
	unitsRestartDelay = 10 * time.Second
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

func writeUnitsStatus(nonce, phase, message string) {
	if err := unitsStatusFile().WriteRaw(strings.TrimSpace(nonce + " " + phase + " " + message)); err != nil {
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

// unitSupervisorDefinition renders the supervisor's job definition. It has no template in
// embedded/tmpl: that generator also renders the Linux units, and this job exists only on macOS.
func unitSupervisorDefinition(installerBin string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>` + unitSupervisorJob + `</string>
        <key>RunAtLoad</key>
        <false/>
        <key>KeepAlive</key>
        <dict>
            <key>Crashed</key>
            <true/>
        </dict>
        <key>ProgramArguments</key>
        <array>
            <string>` + installerBin + `</string>
            <string>package-command</string>
            <string>` + agentPackage + `</string>
            <string>` + superviseUnitsCommand + `</string>
        </array>
        <key>StandardOutPath</key>
        <string>` + unitSupervisorLogPath + `</string>
        <key>StandardErrorPath</key>
        <string>` + unitSupervisorLogPath + `</string>
        <key>UserName</key>
        <string>root</string>
        <key>GroupName</key>
        <string>wheel</string>
    </dict>
</plist>
`)
}

// ensureUnitSupervisor writes the supervisor's job definition, loads it if it is not, and starts
// it. Starting a supervisor that runs already does nothing: it sees the new target by itself.
var ensureUnitSupervisor = func(ctx context.Context) error {
	installerBin, err := installerExecutable()
	if err != nil {
		return err
	}
	client := installerLaunchdClient()
	set := launchd.JobSet{Labels: []string{unitSupervisorJob}, Dir: launchdJobDir, Client: client}
	job := set.Job(unitSupervisorJob, launchd.Stable)
	changed, err := job.WriteIfChanged(unitSupervisorDefinition(installerBin))
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
// is retried after unitsRestartDelay; every step resumes from launchd's state, which is also what a
// supervisor relaunched after a crash does.
func superviseUnits(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			// Stopped on purpose: by an install, an uninstall, or launchd's bootout.
			return nil
		}
		request, err := readUnitsRequest()
		if err != nil {
			return fmt.Errorf("unit supervisor: could not read its target: %w", err)
		}
		status, err := readUnitsStatus()
		if err != nil {
			return fmt.Errorf("unit supervisor: could not read its status: %w", err)
		}
		finished := status.nonce == request.nonce && (status.phase == unitsPhaseDone || status.phase == unitsPhaseFailed)
		switch request.target {
		case unitsTargetExperiment:
			if status.nonce == request.nonce && status.phase == unitsPhaseFailed {
				return nil
			}
			err = runExperiment(ctx, request, status)
		case unitsTargetStable, unitsTargetRestartStable:
			if finished {
				return nil
			}
			err = applyStable(ctx, request, request.target == unitsTargetRestartStable)
		default:
			return nil
		}
		if errors.Is(err, errTargetChanged) {
			continue
		}
		if err != nil {
			log.Errorf("unit supervisor: %v; retrying in %s", err, unitsRestartDelay)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(unitsRestartDelay):
			}
			continue
		}
		if latest, readErr := readUnitsRequest(); readErr == nil && latest == request {
			return nil
		}
	}
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

// childExit is the exit of one of the experiment Agent's child jobs.
type childExit struct {
	label  string
	status int
}

// superviseExperiment waits for the experiment Agent to exit, the deadline to pass or the target
// to change. The first two fail the experiment (OnFailure). A child job that exits non-zero is
// restarted (Restart=on-failure); one that exits cleanly, as a job whose feature is off does, stays
// down.
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

	children := make(chan childExit, len(configExperimentJobs))
	var childObservers []*launchd.ExitObserver
	defer func() {
		for _, o := range childObservers {
			_ = o.Disarm()
		}
	}()
	armChild := func(label string) {
		o, err := armChildJob(ctx, label, children)
		if err != nil {
			log.Warnf("unit supervisor: could not watch %s: %v", label+string(launchd.Experiment), err)
			return
		}
		if o != nil {
			childObservers = append(childObservers, o)
		}
	}
	for _, label := range experimentChildJobs() {
		armChild(label)
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
			if exitedCleanly(child.status) {
				continue
			}
			log.Warnf("unit supervisor: %s exited (status %d), restarting it", child.label+string(launchd.Experiment), child.status)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(unitsRestartDelay):
			}
			if err := launchdClient().Kickstart(ctx, child.label+string(launchd.Experiment), false); err != nil {
				log.Errorf("unit supervisor: could not restart %s: %v", child.label+string(launchd.Experiment), err)
				continue
			}
			armChild(child.label)
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
// child that is not running -- it exited cleanly at start, its feature being off -- is not watched.
func armChildJob(ctx context.Context, label string, exits chan<- childExit) (*launchd.ExitObserver, error) {
	set := launchd.JobSet{Labels: []string{label}, Dir: launchdJobDir, Client: launchdClient()}
	pids, _, err := resolveExperimentPids(ctx, set)
	if err != nil {
		return nil, err
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
		return nil, err
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
		// Left as it is, the target still asks for the experiment: launchd relaunches the supervisor,
		// which tries again.
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
		// Loaded without running: what launchd leaves of it after a reboot mid-experiment.
		if err := installerLaunchdClient().Bootout(ctx, installerJob+string(launchd.Experiment)); err != nil {
			log.Warnf("unit supervisor: could not unload the idle experiment daemon: %v", err)
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
