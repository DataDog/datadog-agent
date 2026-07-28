// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

// Package preflightmodeimpl implements the Agent Data Plane preflight mode component
package preflightmodeimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	preflightmode "github.com/DataDog/datadog-agent/comp/dataplane/preflightmode/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// workDirName is the directory, under run_path, holding the generated config, ADP's
	// own log file and the throwaway DogStatsD socket.
	workDirName = "adp-preflight"

	// listenerPollInterval and listenerTimeout bound how long we wait for ADP to bind
	// the throwaway DogStatsD endpoint before giving up on the probe.
	listenerPollInterval = 200 * time.Millisecond
	listenerTimeout      = 20 * time.Second

	// probeConnectTimeout and probeWriteTimeout bound the probe's DogStatsD client.
	probeConnectTimeout = 3 * time.Second
	probeWriteTimeout   = 2 * time.Second

	// defaultStopTimeout is used when data_plane.stop_timeout is unset or nonsensical.
	defaultStopTimeout = 10 * time.Second
	// shutdownSlack is added on top of the bounds stop actually waits for.
	shutdownSlack = 5 * time.Second
	// maxShutdownGrace caps how long an Agent shutdown can be delayed by the pre-flight.
	maxShutdownGrace = 30 * time.Second
)

// preflightModeDuration is how long ADP is left running.
//
// Deliberately not a config setting. There is no operational reason to tune it: the off
// switch operators actually need is data_plane.preflight_mode, and a documented duration would be
// public API we have to support and later deprecate for a mechanism that only exists until ADP
// goes GA. Fixing it also makes the agent telemetry schedule's start_after a provable
// relationship rather than one that silently breaks when an operator raises the window past it.
//
// A var rather than a const only so tests can shorten it; nothing in production reassigns it.
// If this is raised, check start_after on the data-plane-preflight-mode profile in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml still lands after the window.
var preflightModeDuration = 90 * time.Second

// resolveDataPlanePath returns the ADP binary path. A variable so tests can substitute a
// stand-in binary.
var resolveDataPlanePath = defaultpaths.GetDefaultDataPlaneBin

// errDataPlaneNotInstalled means the ADP binary is not on disk. That is an expected state, not
// a failure: the packaging omits ADP on Heroku, and the Bazel packaging flow does not ship it
// at all yet. Reporting it would drown the real signal in hosts that were never going to run
// ADP.
var errDataPlaneNotInstalled = errors.New("the Agent Data Plane binary is not installed")

// findDataPlane returns a runnable ADP binary path, or an error indicating why the binary
// could not be found.
func findDataPlane(path string) (string, error) {
	fi, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return "", fmt.Errorf("%w: %s", errDataPlaneNotInstalled, path)
	case err != nil:
		return "", fmt.Errorf("could not stat %s: %w", path, err)
	case !fi.Mode().IsRegular():
		return "", fmt.Errorf("%s is not a regular file", path)
	// Windows decides executability from the extension, not a mode bit.
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0111 == 0:
		return "", fmt.Errorf("%s is not executable", path)
	}
	return path, nil
}

// Requires defines the dependencies for the ADP preflight mode component
type Requires struct {
	Lc        compdef.Lifecycle
	Config    configcomp.Component
	Log       logcomp.Component
	Telemetry telemetry.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp preflightmode.Component
}

type preflightModeComponent struct {
	log      logcomp.Component
	config   configcomp.Component
	reporter *reporter

	// binPath is the ADP binary to run.
	binPath string
	// out captures ADP's output. Created up front and never reassigned, because preflight
	// mode runs exactly once per Agent start.
	out *capture

	ctxCancel context.CancelFunc
	done      chan struct{}
}

// NewComponent creates the ADP preflight mode component.
//
// When preflight mode should not run, this returns an inert component rather than an error:
// not running is the common case (ADP already configured either way, an unsupported
// flavor, or a build that does not ship ADP), and none of those are failures.
func NewComponent(reqs Requires) Provides {
	inert := Provides{Comp: &preflightModeComponent{}}

	// Heroku and IoT share the core Agent's run command with a reduced set of build
	// tags, so the flavor is the only runtime way to tell them apart. Neither ships ADP.
	if f := flavor.GetFlavor(); f != flavor.DefaultAgent {
		reqs.Log.Debugf("Agent Data Plane preflight mode does not run on the %s flavor", f)
		return inert
	}

	if eligible, reason := isEligible(reqs.Config); !eligible {
		reqs.Log.Debugf("Agent Data Plane preflight mode is not eligible to run: %s", reason)
		return inert
	}

	reporter := newReporter(reqs.Log, reqs.Telemetry)

	binPath, err := findDataPlane(resolveDataPlanePath())
	if err != nil {
		if errors.Is(err, errDataPlaneNotInstalled) {
			// Expected on builds that do not ship ADP. Not a finding: reporting it would
			// drown the real signal in hosts that were never going to run ADP.
			reqs.Log.Debugf("Agent Data Plane preflight mode has nothing to run: %v", err)
			return inert
		}
		// The binary is there but unusable, which is a genuine packaging or permissions
		// problem worth knowing about.
		reqs.Log.Warnf("Agent Data Plane preflight mode cannot run the installed binary: %v", err)
		reporter.report(&outcome{findings: []finding{findingSpawnFailed}})
		return inert
	}

	comp := &preflightModeComponent{
		log:      reqs.Log,
		config:   reqs.Config,
		reporter: reporter,
		binPath:  binPath,
		out:      newCapture(reqs.Log),
		done:     make(chan struct{}),
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	return Provides{Comp: comp}
}

// start kicks off the pre-flight in the background. It must not block: a preflight mode run
// lasts a minute by default, and OnStart hooks run in sequence with every other
// component's.
func (d *preflightModeComponent) start(context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.ctxCancel = cancel

	go func() {
		defer close(d.done)
		d.run(ctx)
	}()
	return nil
}

// stop cancels an in-flight run and waits, bounded, for it to unwind so that a fast Agent
// shutdown cannot leave an orphaned ADP process behind.
func (d *preflightModeComponent) stop(context.Context) error {
	if d.ctxCancel == nil {
		return nil
	}
	d.ctxCancel()

	grace := d.shutdownGrace()
	select {
	case <-d.done:
	case <-time.After(grace):
		// The run is wedged. Its deferred cleanup will not have happened, and the generated
		// config holds the api_key and every other credential in the Agent's configuration, so remove it
		// here rather than letting it outlive the Agent.
		d.log.Warnf("Agent Data Plane preflight mode did not unwind within %s; cleaning up its working directory", grace)
		if dir := d.workDir(); dir != "" {
			if err := os.RemoveAll(dir); err != nil {
				d.log.Errorf("Could not remove %s, which holds the Agent's credentials: %v", dir, err)
			}
		}
	}
	return nil
}

// shutdownGrace bounds how long stop waits for an in-flight run to unwind.
//
// It is derived rather than fixed because the work it waits on is itself configurable:
// terminate waits up to data_plane.stop_timeout, which ComputeDataPlaneStopTimeout derives
// from aggregator_stop_timeout + forwarder_stop_timeout, and the run then still waits for
// the probe. A fixed 10s was shorter than that for any operator who raised either timeout,
// which meant the deferred cleanup and the report were skipped on every Agent shutdown.
//
// Capped so a pathological configuration cannot stall Agent shutdown indefinitely.
func (d *preflightModeComponent) shutdownGrace() time.Duration {
	grace := d.stopTimeout() + probeConnectTimeout + probeWriteTimeout + shutdownSlack
	return min(grace, maxShutdownGrace)
}

// stopTimeout is how long ADP is given to exit after being asked to stop.
func (d *preflightModeComponent) stopTimeout() time.Duration {
	timeout := time.Duration(d.config.GetInt(DataPlaneStopTimeout)) * time.Second
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}
	return timeout
}

// workDir is the directory holding the generated config and the throwaway socket.
func (d *preflightModeComponent) workDir() string {
	if d.config == nil {
		return ""
	}
	runPath := d.config.GetString("run_path")
	if runPath == "" {
		return ""
	}
	return filepath.Join(runPath, workDirName)
}

// run performs one complete pre-flight: prepare, spawn, probe, stop, scan, report.
// prepare sets up the working directory and generated config, returning a command ready to
// start along with the endpoint ADP will bind.
func (d *preflightModeComponent) prepare() (*exec.Cmd, listener, error) {
	workDir := d.workDir()
	// A run that was killed mid-flight may have left a stale socket and config behind.
	if err := os.RemoveAll(workDir); err != nil {
		d.log.Warnf("Could not clear a stale %s: %v", workDir, err)
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, listener{}, fmt.Errorf("could not create %s: %w", workDir, err)
	}
	// Lock the directory down before anything sensitive goes into it, so the generated config
	// is never on disk with weaker permissions than it ends up with. See secureWorkDir.
	if err := secureWorkDir(workDir); err != nil {
		return nil, listener{}, err
	}

	l := newListener(workDir)
	if err := l.validate(); err != nil {
		return nil, l, fmt.Errorf("unusable DogStatsD endpoint under %s: %w", workDir, err)
	}

	cfgPath, err := writePreflightConfig(d.config, l, workDir)
	if err != nil {
		return nil, l, err
	}

	cmd := exec.Command(d.binPath, "--config", cfgPath, "run")
	// The same writer for both streams on purpose: os/exec then uses a single pipe, so lines
	// cannot interleave mid-line, and Wait only returns once all output has been copied.
	cmd.Stdout = d.out
	cmd.Stderr = d.out
	cmd.Env = childEnv(os.Environ())
	// On Linux, have the kernel kill ADP if the Agent dies without running its own cleanup.
	cmd.SysProcAttr = preflightProcAttr()
	return cmd, l, nil
}

// secureWorkDir restricts the working directory to the Agent's own account.
//
// This is a platform portable method, specifically handling both Unix (through normal permission bits / chmod) and
// Windows (security IDs and what have you).
//
// Defined as a variable in order to allow for tests to manipulate it such that we can test its behavior.
var secureWorkDir = func(dir string) error {
	perms, err := filesystem.NewPermission()
	if err != nil {
		return fmt.Errorf("could not resolve the permissions to apply to %s: %w", dir, err)
	}
	// Fail rather than continue with a directory we could not restrict: writing the Agent's
	// credentials somewhere world-readable is worse than skipping the pre-flight.
	if err := perms.RemoveAccessToOtherUsers(dir); err != nil {
		return fmt.Errorf("could not restrict access to %s: %w", dir, err)
	}
	return nil
}

func (d *preflightModeComponent) run(ctx context.Context) {
	started := time.Now()
	o := &outcome{}
	defer func() {
		o.durationSeconds = time.Since(started).Seconds()
		d.reporter.report(o)
	}()
	// The generated config holds the Agent's credentials, so it must not outlive the run.
	defer func() {
		if err := os.RemoveAll(d.workDir()); err != nil {
			d.log.Warnf("Could not clean up %s: %v", d.workDir(), err)
		}
	}()

	cmd, l, err := d.prepare()
	if err != nil {
		d.log.Warnf("Agent Data Plane preflight mode could not prepare a run: %v", err)
		o.add(findingSpawnFailed)
		return
	}

	d.log.Infof("Starting Agent Data Plane in preflight mode for %s (binary %s, DogStatsD endpoint %s)",
		preflightModeDuration, d.binPath, l.describe())
	if err := cmd.Start(); err != nil {
		d.log.Warnf("Agent Data Plane preflight mode could not start %s: %v", d.binPath, err)
		o.add(findingSpawnFailed)
		return
	}

	// procCtx is cancelled as soon as ADP is gone, so the probe stops waiting for an
	// endpoint that will never be bound instead of burning its full timeout.
	procCtx, procCancel := context.WithCancel(ctx)
	defer procCancel()

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
		procCancel()
	}()

	// The probe runs concurrently with the preflight window so a slow bind does not eat into
	// it. Its findings come back through a channel to keep the bookkeeping on one
	// goroutine.
	probeResult := make(chan bool, 1)
	go func() { probeResult <- d.probe(procCtx, l) }()

	timer := time.NewTimer(preflightModeDuration)
	defer timer.Stop()

	var exitedEarly, interrupted bool
	select {
	case <-waitErr:
		exitedEarly = true
	case <-timer.C:
		d.terminate(cmd, waitErr, o)
	case <-ctx.Done():
		d.log.Infof("Agent Data Plane preflight mode interrupted by Agent shutdown after %s",
			time.Since(started).Round(time.Second))
		interrupted = true
		d.terminate(cmd, waitErr, o)
	}

	// Always wait for the probe so run does not return with a goroutine in flight.
	probeFailed := <-probeResult

	switch {
	case interrupted:
		// Cut short by an Agent restart, not by anything ADP did. Reporting the consequences
		// would be misleading — the probe legitimately never reached a listener — and at fleet
		// scale restarts inside the window are common enough to put a permanent floor of false
		// positives under the primary signal. Recorded first so it becomes the reported result;
		// findings from what ADP actually logged are still added below.
		o.add(findingInterrupted)
	case exitedEarly:
		d.log.Debugf("Agent Data Plane exited before the %s preflight mode window elapsed", preflightModeDuration)
		o.add(findingExitedEarly)
	case probeFailed:
		o.add(findingProbeFailed)
	}

	records, dropped := d.out.finish()
	if dropped > 0 {
		d.log.Warnf("Agent Data Plane preflight mode dropped %d log record(s) from its capture buffer", dropped)
		o.add(findingOutputDropped)
	}
	o.records = records
	if hasErrors(records) {
		o.add(findingErrorsInLog)
	}
	if hasUnexpectedWarnings(records) {
		o.add(findingWarningsInLog)
	}
}

// terminate stops ADP, escalating to a kill if it does not exit within the configured stop
// timeout.
//
// The exit status is deliberately not interpreted: we asked the process to stop, so whether it
// exited 0 or died from the signal says nothing about its health. Anything worth knowing is in
// the log scan.
func (d *preflightModeComponent) terminate(cmd *exec.Cmd, waitErr <-chan error, o *outcome) {
	if err := requestStop(cmd.Process); err != nil {
		// Already gone, most likely; Wait below confirms.
		d.log.Debugf("Could not signal the Agent Data Plane process: %v", err)
	}

	select {
	case <-waitErr:
		return
	case <-time.After(d.stopTimeout()):
		d.log.Warnf("Agent Data Plane did not stop within %s, killing it", d.stopTimeout())
		o.add(findingStopTimeout)
		_ = cmd.Process.Kill()
		<-waitErr
	}
}

// probe waits for ADP to bind its DogStatsD endpoint and pushes one throwaway metric through
// it, exercising the DogStatsD, aggregation, serialization and forwarding path. It reports
// whether the probe failed.
//
// The client is built directly rather than through comp/dogstatsd/statsd, whose createClient
// lets STATSD_URL override the address unconditionally — that would silently send the probe
// somewhere other than the preflight endpoint. Client-side aggregation is off and mutex mode is on
// so exactly one sample goes on the wire and Flush is synchronous.
func (d *preflightModeComponent) probe(ctx context.Context, l listener) bool {
	if !waitFor(ctx, listenerTimeout, l.ready) {
		d.log.Warnf("Agent Data Plane did not bind %s within %s", l.describe(), listenerTimeout)
		return true
	}

	client, err := ddgostatsd.New(l.probeAddr(),
		ddgostatsd.WithoutTelemetry(),
		ddgostatsd.WithoutClientSideAggregation(),
		ddgostatsd.WithMutexMode(),
		ddgostatsd.WithWriteTimeout(probeWriteTimeout),
		ddgostatsd.WithConnectTimeout(probeConnectTimeout),
	)
	if err != nil {
		d.log.Warnf("Could not create the Agent Data Plane preflight mode probe client: %v", err)
		return true
	}
	defer client.Close()

	tags := []string{"preflight_mode:true", "agent_version:" + version.AgentVersion, "os:" + runtime.GOOS}
	if err := client.Gauge(probeMetricName, 1, tags, 1); err != nil {
		d.log.Warnf("Could not submit the Agent Data Plane preflight mode probe metric: %v", err)
		return true
	}
	if err := client.Flush(); err != nil {
		d.log.Warnf("Could not flush the Agent Data Plane preflight mode probe metric: %v", err)
		return true
	}

	d.log.Debugf("Delivered %s to the Agent Data Plane preflight mode endpoint", probeMetricName)
	return false
}

// waitFor polls until ready returns true, the context is cancelled, or the timeout elapses.
func waitFor(ctx context.Context, timeout time.Duration, ready func() bool) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(listenerPollInterval)
	defer ticker.Stop()

	for {
		if ready() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}

// sanitizedEnv strips every DD_ variable from the environment passed to ADP.
//
// This is load-bearing. ADP layers environment variables over its config file, so an
// inherited DD_DOGSTATSD_PORT or DD_DOGSTATSD_SOCKET would override the throwaway
// endpoint in the generated config and make the preflight process bind the real DogStatsD
// endpoint — taking traffic away from the Core Agent. Everything ADP needs is in the
// generated config file, so dropping the whole DD_ namespace costs nothing.
//
// The prefix match is case-insensitive because Windows environment lookups are: a
// dd_dogstatsd_port left in place there would still reach ADP as DD_DOGSTATSD_PORT. Applied
// on every platform rather than only Windows — a lowercase dd_ variable is not something ADP
// needs either way, so there is no reason to carry a platform split for it.
func sanitizedEnv(env []string) []string {
	const prefix = "DD_"
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if len(kv) >= len(prefix) && strings.EqualFold(kv[:len(prefix)], prefix) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// preflightEnvOverrides are the variables the pre-flight sets on ADP to hold its idle footprint
// down. A slice rather than a map so the emitted environment is deterministic.
//
// These live here rather than in the generated config because they are not settings ADP reads from
// its configuration at all.
var preflightEnvOverrides = []struct{ key, value string }{
	// Cap ADP's async runtime at two worker threads.
	//
	// Tokio otherwise sizes its pool from the available parallelism, so on a large host ADP spawns
	// a worker per core and its idle footprint scales with the core count: a stack plus a
	// per-thread allocator cache each. Pinning the count makes the pre-flight's footprint
	// independent of the host it runs on, and two workers is ample for pushing one metric through.
	//
	// This is knowingly not the runtime shape ADP runs with in production, so a problem that only
	// appears with a worker per core will not show up here. The pre-flight gives up that coverage
	// for a predictable footprint on every host in the fleet.
	{key: "TOKIO_WORKER_THREADS", value: "2"},
}

// childEnv builds the environment for the preflight ADP process: the Agent's own environment with
// the DD_ namespace stripped, plus preflightEnvOverrides.
//
// Inherited copies of the overridden variables are dropped rather than left to be shadowed. os/exec
// resolves duplicate keys in favor of the last entry, so appending alone would work, but relying on
// that would make the child's runtime depend on a detail of os/exec rather than on this function.
// The match is case-insensitive for the same reason sanitizedEnv's is: Windows environment lookups
// are, so an inherited lowercase spelling would still reach ADP as the real variable.
func childEnv(env []string) []string {
	out := make([]string, 0, len(env)+len(preflightEnvOverrides))
	for _, kv := range sanitizedEnv(env) {
		if !isOverriddenEnv(kv) {
			out = append(out, kv)
		}
	}
	for _, o := range preflightEnvOverrides {
		out = append(out, o.key+"="+o.value)
	}
	return out
}

// isOverriddenEnv reports whether an environment entry names a variable preflightEnvOverrides sets.
func isOverriddenEnv(kv string) bool {
	// An entry with no "=" is not a variable assignment; treat the whole thing as the name so a
	// malformed entry can never be mistaken for one of the overrides.
	key, _, _ := strings.Cut(kv, "=")
	for _, o := range preflightEnvOverrides {
		if strings.EqualFold(key, o.key) {
			return true
		}
	}
	return false
}
