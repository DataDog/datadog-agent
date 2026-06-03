// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthreporter provides a reusable gRPC client for system-probe modules
// to report and resolve health issues via the core agent's AgentSecure endpoint.
// Any module that detects a runtime failure can instantiate a Reporter and call
// ReportWithRetry / ResolveWithRetry without duplicating connection or retry logic.
package healthreporter

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	ipcdef "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultCallTimeout caps a single gRPC call attempt.
	DefaultCallTimeout = 5 * time.Second
	// DefaultMaxWait is how long ReportWithRetry / ResolveWithRetry keep retrying.
	// Long enough to cover the common case where system-probe starts before the
	// core agent gRPC server is ready.
	DefaultMaxWait = 3 * time.Minute
)

// Reporter sends health issues and resolutions to the core agent via the
// AgentSecure ReportHealthIssue / ResolveHealthIssue gRPC endpoints.
type Reporter struct {
	ipc         ipcdef.Component
	callTimeout time.Duration
	maxWait     time.Duration
}

// New returns a Reporter that uses the given IPC component for mTLS and auth.
func New(ipc ipcdef.Component) *Reporter {
	return &Reporter{
		ipc:         ipc,
		callTimeout: DefaultCallTimeout,
		maxWait:     DefaultMaxWait,
	}
}

// ReportWithRetry sends issue to the core agent in a background goroutine,
// retrying with exponential backoff until the call succeeds or DefaultMaxWait elapses.
func (r *Reporter) ReportWithRetry(issue *healthplatformpayload.Issue) {
	go r.retryWithBackoff("report "+issue.GetId(), func() error {
		return r.Report(context.Background(), issue)
	})
}

// ResolveWithRetry clears issueID on the core agent in a background goroutine,
// retrying with exponential backoff until the call succeeds or DefaultMaxWait elapses.
func (r *Reporter) ResolveWithRetry(issueID string) {
	go r.retryWithBackoff("resolve "+issueID, func() error {
		return r.Resolve(context.Background(), issueID)
	})
}

// Report sends a single ReportHealthIssue RPC. The caller is responsible for
// any retry / timeout policy.
func (r *Reporter) Report(ctx context.Context, issue *healthplatformpayload.Issue) error {
	client, err := r.newClient()
	if err != nil {
		return err
	}
	packed, err := anypb.New(issue)
	if err != nil {
		return fmt.Errorf("pack issue: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.callTimeout)
	defer cancel()
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{Issue: packed})
	return err
}

// Resolve sends a single ResolveHealthIssue RPC. The caller is responsible for
// any retry / timeout policy.
func (r *Reporter) Resolve(ctx context.Context, issueID string) error {
	client, err := r.newClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, r.callTimeout)
	defer cancel()
	_, err = client.ResolveHealthIssue(ctx, &pb.ResolveHealthIssueRequest{IssueId: issueID})
	return err
}

func (r *Reporter) newClient() (pb.AgentSecureClient, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("get IPC address: %w", err)
	}
	return ddgrpc.GetDDAgentSecureClient(
		context.Background(),
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		r.ipc.GetTLSClientConfig().Clone(),
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(r.ipc.GetAuthToken())),
	)
}

func (r *Reporter) retryWithBackoff(op string, fn func() error) {
	deadline := time.Now().Add(r.maxWait)
	backoff := 2 * time.Second
	for {
		if err := fn(); err == nil {
			return
		} else if time.Now().After(deadline) {
			log.Warnf("health platform: gave up on %s after %s: %v", op, r.maxWait, err)
			return
		} else {
			log.Debugf("health platform: retrying %s in %s (err: %v)", op, backoff, err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}
