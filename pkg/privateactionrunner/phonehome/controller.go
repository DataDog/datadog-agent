// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome implements a startup gate, not an enrollment controller.
package phonehome

import (
	"context"
	"expvar"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/deployment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	controlProcess  = "datadog-agent-par-control"
	executorProcess = "datadog-agent-action-executor"
	pollInterval    = time.Minute
	slowRetry       = 15 * time.Minute
)

type State = phonehomedef.State

var status = struct {
	sync.RWMutex
	value *State
}{}

func Status() *State {
	status.RLock()
	defer status.RUnlock()
	if status.value == nil {
		return nil
	}
	value := *status.value
	return &value
}

func publish(s State) { status.Lock(); defer status.Unlock(); status.value = &s }

// The controller emits serially; status must stay fresh even without a transition.
func newStateEmitter(report, logState func(State)) func(State) {
	var previous State
	observed := false
	return func(s State) {
		publish(s)
		if observed && s.State == previous.State && s.Reason == previous.Reason {
			return
		}
		previous, observed = s, true
		report(s)
		logState(s)
	}
}

func Enabled(cfg model.Reader) bool {
	return runtime.GOOS != "windows" && cfg.GetBool(par.Enabled)
}

// Run watches supervisor registrations and authorizes each new lifecycle's initial startup.
// It never retries an ambiguous Start or stops a child.
// Status updates on every observation; report receives state/reason transitions.
func Run(ctx context.Context, cfg model.Reader, source Source, report func(State)) {
	if !Enabled(cfg) {
		return
	}
	mode, err := deployment.Resolve(cfg)
	// Intentional ungated modes take precedence over gate-specific configuration warnings.
	bypass := runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent ||
		(err == nil && mode != deployment.Split)
	emit := newStateEmitter(report, newStateLogger(bypass))
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	if bypass || err != nil || !filepath.IsAbs(enrollment.GetIdentityFilePath(cfg)) || os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		emit(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonUnsupportedConfiguration})
		return
	}
	socket := supervisorSocketPath()
	if !filepath.IsAbs(socket) {
		emit(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonInvalidSupervisorSocket})
		return
	}
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		emit(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonSupervisorConnectionFailed})
		return
	}
	defer conn.Close()
	c := controller{cfg: cfg, source: source, pm: pb.NewProcessManagerClient(conn), emit: emit, now: time.Now}
	c.run(ctx)
}

func supervisorSocketPath() string {
	if socket := os.Getenv("DD_PM_SOCKET_PATH"); socket != "" {
		return socket
	}
	// Match dd-procmgrd's Unix default in pkg/procmgr/rust/client/src/lib.rs.
	return "/var/run/datadog-procmgrd/dd-procmgrd.sock"
}

type controller struct {
	cfg           model.Reader
	source        Source
	pm            pb.ProcessManagerClient
	emit          func(State)
	now           func() time.Time
	attempted     bool
	generation    [2]string
	handoffReason phonehomedef.Reason
}

func (c *controller) run(ctx context.Context) {
	defer func() {
		if ctx.Err() != nil {
			log.Debug("PAR startup gate: canceled")
		}
	}()
	delay := time.Duration(rand.Int64N(int64(pollInterval)))
	c.emit(State{State: phonehomedef.StateWaiting, Reason: phonehomedef.ReasonStartupJitter})
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = c.step(ctx)
		if delay == 0 {
			return
		}
	}
}

func (c *controller) step(ctx context.Context) time.Duration {
	set := func(state phonehomedef.StateName, reason phonehomedef.Reason, delay time.Duration) time.Duration {
		s := State{State: state, Reason: reason}
		if delay > 0 {
			s.NextCheck = c.now().Add(delay).UTC().Format(time.RFC3339)
		}
		c.emit(s)
		return delay
	}
	if ctx.Err() != nil || !Enabled(c.cfg) {
		return 0
	}
	if c.attempted && c.generation == [2]string{} {
		return 0 // An ambiguous Start is not evidence that re-arming is safe.
	}
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var children [2]*pb.ProcessDetail
	for i, name := range []string{controlProcess, executorProcess} {
		p, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: name})
		if err != nil || p.GetDetail() == nil {
			return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorUnavailable, pollInterval)
		}
		children[i] = p.Detail
	}
	generation := [2]string{children[0].Uuid, children[1].Uuid}
	if generation[0] == "" || generation[1] == "" {
		return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorRegistrationUnavailable, pollInterval)
	}
	// A restart between the two Describe calls must not become a mixed baseline.
	confirmed, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: generation[0]})
	if err != nil || confirmed.GetDetail().GetUuid() != generation[0] {
		return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorRegistrationChangePending, pollInterval)
	}
	if c.generation != [2]string{} {
		if generation == c.generation {
			c.emit(State{State: phonehomedef.StateHandedOff, Reason: c.handoffReason})
			return pollInterval // Child crashes/restarts belong to the existing supervisor lifecycle.
		}
		if generation[0] == c.generation[0] || generation[1] == c.generation[1] {
			return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorRegistrationChangePending, pollInterval)
		}
		log.Info("PAR startup gate: fresh supervisor lifecycle detected; new startup requires RC and fresh AAA authorization")
		c.attempted = false
		c.generation = [2]string{}
	}
	for i, child := range children {
		// The control plane may recover after a crash, but only core owns its initial launch.
		restartAllowed := child.RestartPolicy == "never" || (i == 0 && child.RestartPolicy == "on-failure")
		if child.AutoStart || !restartAllowed {
			return set(phonehomedef.StateBlocked, phonehomedef.ReasonUnsupportedSupervisorPolicy, 0)
		}
	}
	for _, child := range children {
		switch child.State {
		case pb.ProcessState_STARTING, pb.ProcessState_RUNNING, pb.ProcessState_STOPPING:
			return c.handoff(generation, phonehomedef.ReasonExistingProcessLeftUntouched)
		}
	}
	for _, child := range children {
		switch child.State {
		case pb.ProcessState_SKIPPED, pb.ProcessState_STOPPED, pb.ProcessState_CREATED:
		default:
			return set(phonehomedef.StateBlocked, phonehomedef.ReasonPreviousProcessExitRequiresReview, 0)
		}
	}
	// Old recovery experiments may hold credentials: require deliberate reconciliation before migration.
	if _, err := os.Stat(enrollment.GetIdentityFilePath(c.cfg) + ".phone-home"); !os.IsNotExist(err) {
		return set(phonehomedef.StateBlocked, phonehomedef.ReasonLegacyAttemptRequiresReconciliation, 0)
	}
	gate := c.source.Read(c.now())
	if !gate.Ready {
		return set(phonehomedef.StateWaiting, gate.Reason, pollInterval)
	}
	if ctx.Err() != nil || !Enabled(c.cfg) {
		return 0
	}
	// Recheck after supervisor/config inspection, immediately before Start.
	if gate = c.source.Read(c.now()); !gate.Ready {
		return set(phonehomedef.StateWaiting, gate.Reason, pollInterval)
	}
	startCtx, startCancel := context.WithTimeout(ctx, 5*time.Second)
	defer startCancel()
	c.attempted = true
	log.Infof("PAR startup gate: requesting supervisor Start: process=%s", controlProcess)
	if _, err := c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: children[0].Uuid}); err != nil {
		if grpcstatus.Code(err) == codes.NotFound {
			// The supervisor resolves the UUID before spawning: this response proves no start.
			c.attempted = false
			return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorRegistrationChangePending, pollInterval)
		}
		return set(phonehomedef.StateBlocked, phonehomedef.ReasonLaunchOutcomeUnknownRequiresReview, 0)
	}
	return c.handoff(generation, phonehomedef.ReasonSupervisorStartAcknowledged)
}

func (c *controller) handoff(generation [2]string, reason phonehomedef.Reason) time.Duration {
	c.generation, c.handoffReason = generation, reason
	c.source.Release()
	if reason == phonehomedef.ReasonSupervisorStartAcknowledged {
		log.Info("PAR startup gate: supervisor Start acknowledged; handoff is not enrollment or readiness")
	} else {
		log.Info("PAR startup gate: existing children left untouched; supervisor retains ownership")
	}
	c.emit(State{State: phonehomedef.StateHandedOff, Reason: reason})
	return pollInterval
}
