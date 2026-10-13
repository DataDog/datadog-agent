// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotereporterimpl implements the remotereporter component
package remotereporterimpl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	remotereporter "github.com/DataDog/datadog-agent/comp/healthplatform/remotereporter/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const (
	requestTimeout = 10 * time.Second
	minRetryDelay  = time.Second
	maxRetryDelay  = 2 * time.Minute
	resyncInterval = 10 * time.Minute
	// stopFlushTimeout bounds the final delivery attempt on stop, so that
	// issues reported right before the process exits (for example a module
	// failing at startup) are not lost, without holding up shutdown.
	stopFlushTimeout = 3 * time.Second
)

// Requires defines the dependencies for the remotereporter component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	IPC       ipc.Component
	Log       log.Component
}

// Provides defines the output of the remotereporter component
type Provides struct {
	Comp remotereporter.Component
}

// NewComponent creates a reporter that sends issues to the core agent over the
// AgentSecure gRPC API. It is a no-op when the health platform is disabled or
// the core agent client cannot be created, so that health reporting never
// prevents the host process from starting.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("health_platform.enabled") {
		return Provides{Comp: noopReporter{}}, nil
	}

	agentAddress := net.JoinHostPort(reqs.Config.GetString("cmd_host"), reqs.Config.GetString("cmd_port"))
	client, conn, err := helper.NewAgentSecureClient(agentAddress, reqs.IPC.GetAuthToken(), reqs.IPC.GetTLSClientConfig(), reqs.Config.GetString("vsock_addr"), reqs.Log)
	if err != nil {
		reqs.Log.Warnf("health issues will not be reported to the core agent: failed to create core agent client: %v", err)
		return Provides{Comp: noopReporter{}}, nil
	}

	r := newReporter(client, reqs.Log)
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			r.start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			r.stop(ctx)
			if err := conn.Close(); err != nil {
				return fmt.Errorf("failed to close core agent client: %w", err)
			}
			return nil
		},
	})

	return Provides{Comp: r}, nil
}

// pendingOp is the desired state of an issue not yet acknowledged by the core
// agent. A nil issue means the issue must be resolved.
type pendingOp struct {
	issue *healthplatformpayload.Issue
}

type reporter struct {
	client pbcore.AgentSecureClient
	log    log.Component

	mu sync.Mutex
	// pending holds the operations not yet delivered, by issue ID.
	pending map[string]*pendingOp
	// synced holds the last state acknowledged by the core agent, by issue ID.
	// A nil value means the issue was resolved.
	synced map[string]*healthplatformpayload.Issue

	notify chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
}

func newReporter(client pbcore.AgentSecureClient, log log.Component) *reporter {
	return &reporter{
		client:  client,
		log:     log,
		pending: make(map[string]*pendingOp),
		synced:  make(map[string]*healthplatformpayload.Issue),
		notify:  make(chan struct{}, 1),
	}
}

func (r *reporter) ReportIssue(issue *healthplatformpayload.Issue) error {
	if issue == nil {
		return errors.New("issue cannot be nil")
	}
	if issue.GetId() == "" {
		return errors.New("issue id cannot be empty")
	}
	if issue.GetIssueName() == "" {
		return errors.New("issue name cannot be empty")
	}

	issue = proto.Clone(issue).(*healthplatformpayload.Issue)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, isPending := r.pending[issue.GetId()]; !isPending {
		if synced := r.synced[issue.GetId()]; synced != nil && proto.Equal(synced, issue) {
			return nil
		}
	}
	r.pending[issue.GetId()] = &pendingOp{issue: issue}
	r.wake()
	return nil
}

func (r *reporter) ResolveIssue(issueID string) {
	if issueID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if op, isPending := r.pending[issueID]; isPending && op.issue == nil {
		return
	}
	if synced, known := r.synced[issueID]; known && synced == nil {
		if _, isPending := r.pending[issueID]; !isPending {
			return
		}
	}
	r.pending[issueID] = &pendingOp{}
	r.wake()
}

// wake signals the background loop; must be called with mu held.
func (r *reporter) wake() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *reporter) start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx, resyncInterval)
}

// stop stops the background loop and makes a last, bounded attempt to deliver
// the pending operations.
func (r *reporter) stop(ctx context.Context) {
	if r.cancel == nil {
		return
	}
	r.cancel()
	<-r.done

	ctx, cancel := context.WithTimeout(ctx, stopFlushTimeout)
	defer cancel()
	if err := r.flush(ctx); err != nil {
		r.log.Debugf("failed to send pending health issues to the core agent before stopping: %v", err)
	}
}

func (r *reporter) run(ctx context.Context, resync time.Duration) {
	defer close(r.done)

	resyncTicker := time.NewTicker(resync)
	defer resyncTicker.Stop()

	var retry <-chan time.Time
	retryDelay := minRetryDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.notify:
		case <-retry:
		case <-resyncTicker.C:
			r.requeueActive()
		}

		if err := r.flush(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Debugf("failed to send health issues to the core agent, retrying in %s: %v", retryDelay, err)
			retry = time.After(retryDelay)
			retryDelay = min(retryDelay*2, maxRetryDelay)
		} else {
			retry = nil
			retryDelay = minRetryDelay
		}
	}
}

// requeueActive schedules every active issue to be sent again, so that issues
// lost by the core agent (restart without persistence, ...) are restored.
func (r *reporter) requeueActive() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, issue := range r.synced {
		if _, isPending := r.pending[id]; !isPending && issue != nil {
			r.pending[id] = &pendingOp{issue: issue}
		}
	}
}

// flush delivers all pending operations, stopping at the first failure.
func (r *reporter) flush(ctx context.Context) error {
	r.mu.Lock()
	ops := make(map[string]*pendingOp, len(r.pending))
	for id, op := range r.pending {
		ops[id] = op
	}
	r.mu.Unlock()

	for id, op := range ops {
		if err := r.send(ctx, id, op); err != nil {
			return err
		}

		r.mu.Lock()
		// The operation may have been superseded while it was in flight.
		if r.pending[id] == op {
			delete(r.pending, id)
		}
		r.synced[id] = op.issue
		r.mu.Unlock()
	}
	return nil
}

func (r *reporter) send(ctx context.Context, id string, op *pendingOp) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	if op.issue == nil {
		if _, err := r.client.ResolveHealthIssue(ctx, &pbcore.ResolveHealthIssueRequest{IssueId: id}); err != nil {
			return fmt.Errorf("failed to resolve health issue %s: %w", id, err)
		}
		return nil
	}
	if _, err := r.client.ReportHealthIssue(ctx, &pbcore.ReportHealthIssueRequest{Issue: op.issue}); err != nil {
		return fmt.Errorf("failed to report health issue %s: %w", id, err)
	}
	return nil
}

type noopReporter struct{}

func (noopReporter) ReportIssue(*healthplatformpayload.Issue) error { return nil }

func (noopReporter) ResolveIssue(string) {}
