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
	"github.com/DataDog/datadog-agent/pkg/fips"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
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

// Run watches supervisor registrations and authorizes each new lifecycle's initial startup.
// It never retries an ambiguous Start or stops a child.
func Run(ctx context.Context, cfg model.Reader, source Source) {
	if !Enabled(cfg) {
		return
	}
	if expvar.Get("par_phone_home") == nil {
		expvar.Publish("par_phone_home", expvar.Func(func() interface{} { return Status() }))
	}
	buildFIPS, err := fips.Enabled()
	if runtime.GOOS != "linux" || configenv.IsContainerized() || flavor.GetFlavor() != flavor.DefaultAgent || err != nil || buildFIPS || cfg.GetBool("fips.enabled") ||
		!cfg.GetBool(par.SplitEnabled) || !filepath.IsAbs(cfg.GetString(par.IdentityFilePath)) || os.Getenv("DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH") != "" {
		publish(State{State: "blocked", Reason: "unsupported_configuration"})
		return
	}
	socket := os.Getenv("DD_PM_SOCKET_PATH")
	if !filepath.IsAbs(socket) {
		publish(State{State: "blocked", Reason: "explicit_supervisor_socket_required"})
		return
	}
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		publish(State{State: "blocked", Reason: "supervisor_connection_failed"})
		return
	}
	defer conn.Close()
	c := controller{cfg: cfg, source: source, pm: pb.NewProcessManagerClient(conn), emit: publish, now: time.Now}
	c.run(ctx)
}

type controller struct {
	cfg           model.Reader
	source        Source
	pm            pb.ProcessManagerClient
	emit          func(State)
	now           func() time.Time
	attempted     bool
	generation    [2]string
	handoffReason string
}

func (c *controller) run(ctx context.Context) {
	delay := time.Duration(rand.Int64N(int64(pollInterval)))
	c.emit(State{State: "waiting", Reason: "startup_jitter"})
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
	set := func(state, reason string, delay time.Duration) time.Duration {
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
			return set("waiting", "supervisor_unavailable", pollInterval)
		}
		children[i] = p.Detail
	}
	generation := [2]string{children[0].Uuid, children[1].Uuid}
	if generation[0] == "" || generation[1] == "" {
		return set("waiting", "supervisor_registration_unavailable", pollInterval)
	}
	// A restart between the two Describe calls must not become a mixed baseline.
	confirmed, err := c.pm.Describe(rpcCtx, &pb.DescribeRequest{NameOrUuid: generation[0]})
	if err != nil || confirmed.GetDetail().GetUuid() != generation[0] {
		return set("waiting", "supervisor_registration_change_pending", pollInterval)
	}
	if c.generation != [2]string{} {
		if generation == c.generation {
			c.emit(State{State: "handed_off", Reason: c.handoffReason})
			return pollInterval // Child crashes/restarts belong to the existing supervisor lifecycle.
		}
		if generation[0] == c.generation[0] || generation[1] == c.generation[1] {
			return set("waiting", "supervisor_registration_change_pending", pollInterval)
		}
		c.attempted = false
		c.generation = [2]string{}
	}
	for _, child := range children {
		if child.AutoStart || child.RestartPolicy != "never" {
			return set("blocked", "supervisor_must_be_on_demand_no_restart", 0)
		}
	}
	for _, child := range children {
		switch child.State {
		case pb.ProcessState_STARTING, pb.ProcessState_RUNNING, pb.ProcessState_STOPPING:
			return c.handoff(generation, "existing_process_left_untouched")
		}
	}
	for _, child := range children {
		switch child.State {
		case pb.ProcessState_SKIPPED, pb.ProcessState_STOPPED, pb.ProcessState_CREATED:
		default:
			return set("blocked", "previous_process_exit_requires_review", 0)
		}
	}
	// Old recovery experiments may hold credentials: require deliberate reconciliation before migration.
	if _, err := os.Stat(c.cfg.GetString(par.IdentityFilePath) + ".phone-home"); !os.IsNotExist(err) {
		return set("blocked", "legacy_attempt_requires_reconciliation", 0)
	}
	gate := c.source.Read(c.now())
	if !gate.Ready {
		return set("waiting", gate.Reason, pollInterval)
	}
	if ctx.Err() != nil || !Enabled(c.cfg) {
		return 0
	}
	// Recheck after supervisor/config inspection, immediately before Start.
	if gate = c.source.Read(c.now()); !gate.Ready {
		return set("waiting", gate.Reason, pollInterval)
	}
	startCtx, startCancel := context.WithTimeout(ctx, 5*time.Second)
	defer startCancel()
	c.attempted = true
	if _, err := c.pm.Start(startCtx, &pb.StartRequest{NameOrUuid: children[0].Uuid}); err != nil {
		if grpcstatus.Code(err) == codes.NotFound {
			// The supervisor resolves the UUID before spawning: this response proves no start.
			c.attempted = false
			return set("waiting", "supervisor_registration_change_pending", pollInterval)
		}
		return set("blocked", "launch_outcome_unknown_requires_review", 0)
	}
	return c.handoff(generation, "supervisor_start_acknowledged")
}

func (c *controller) handoff(generation [2]string, reason string) time.Duration {
	c.generation, c.handoffReason = generation, reason
	c.source.Release()
	c.emit(State{State: "handed_off", Reason: reason})
	return pollInterval
}
