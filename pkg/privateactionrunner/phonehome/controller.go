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
	"google.golang.org/grpc/credentials/insecure"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/deployment"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
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

func Enabled(cfg model.Reader) bool {
	return runtime.GOOS != "windows" && cfg.GetBool(par.Enabled)
}

// Run hands off at most once to the existing PAR startup path. It never retries enrollment or stops a child.
func Run(ctx context.Context, cfg model.Reader, source Source) {
	if !Enabled(cfg) {
		return
	}
	mode, err := deployment.Resolve(cfg)
	if err == nil && mode != deployment.Split {
		return
	}
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	if runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent || err != nil || mode != deployment.Split ||
		!filepath.IsAbs(cfg.GetString(par.IdentityFilePath)) || os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		publish(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonUnsupportedConfiguration})
		return
	}
	socket := os.Getenv("DD_PM_SOCKET_PATH")
	if !filepath.IsAbs(socket) {
		publish(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonExplicitSupervisorSocketRequired})
		return
	}
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		publish(State{State: phonehomedef.StateBlocked, Reason: phonehomedef.ReasonSupervisorConnectionFailed})
		return
	}
	defer conn.Close()
	c := controller{cfg: cfg, source: source, pm: pb.NewProcessManagerClient(conn), emit: publish, now: time.Now}
	c.run(ctx)
}

type controller struct {
	cfg       model.Reader
	source    Source
	pm        pb.ProcessManagerClient
	emit      func(State)
	now       func() time.Time
	attempted bool
}

func (c *controller) run(ctx context.Context) {
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
	if c.attempted {
		return 0
	}
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var control *pb.ProcessDetail
	for _, name := range []string{controlProcess, executorProcess} {
		p, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: name})
		if err != nil || p.GetDetail() == nil {
			return set(phonehomedef.StateWaiting, phonehomedef.ReasonSupervisorUnavailable, pollInterval)
		}
		if p.Detail.AutoStart || p.Detail.RestartPolicy != "never" {
			return set(phonehomedef.StateBlocked, phonehomedef.ReasonSupervisorMustBeOnDemandNoRestart, 0)
		}
		switch p.Detail.State {
		case pb.ProcessState_STARTING, pb.ProcessState_RUNNING, pb.ProcessState_STOPPING:
			return set(phonehomedef.StateHandedOff, phonehomedef.ReasonExistingProcessLeftUntouched, 0)
		case pb.ProcessState_SKIPPED, pb.ProcessState_STOPPED, pb.ProcessState_CREATED:
		default:
			return set(phonehomedef.StateBlocked, phonehomedef.ReasonPreviousProcessExitRequiresReview, 0)
		}
		if name == controlProcess {
			control = p.Detail
		}
	}
	// Old recovery experiments may hold credentials: require deliberate reconciliation before migration.
	if _, err := os.Stat(c.cfg.GetString(par.IdentityFilePath) + ".phone-home"); !os.IsNotExist(err) {
		return set(phonehomedef.StateBlocked, phonehomedef.ReasonLegacyAttemptRequiresReconciliation, 0)
	}
	gate := c.source.Read(c.now())
	if !gate.Ready {
		return set(phonehomedef.StateWaiting, gate.Reason, pollInterval)
	}
	if ctx.Err() != nil || !Enabled(c.cfg) {
		return 0
	}
	// Recheck after supervisor/config inspection, immediately before the only Start RPC.
	if gate = c.source.Read(c.now()); !gate.Ready {
		return set(phonehomedef.StateWaiting, gate.Reason, pollInterval)
	}
	startCtx, startCancel := context.WithTimeout(ctx, 5*time.Second)
	defer startCancel()
	c.attempted = true
	if _, err := c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: control.Uuid}); err != nil {
		return set(phonehomedef.StateBlocked, phonehomedef.ReasonLaunchOutcomeUnknownRequiresReview, 0)
	}
	return set(phonehomedef.StateHandedOff, phonehomedef.ReasonSupervisorStartAcknowledged, 0)
}
