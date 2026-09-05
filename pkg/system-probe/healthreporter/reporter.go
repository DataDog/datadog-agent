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
	"google.golang.org/protobuf/types/known/structpb"

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
	// DefaultMaxWait is how long ResolveWithRetry keeps retrying.
	// Long enough to cover the common case where system-probe starts before the
	// core agent gRPC server is ready.
	DefaultMaxWait = 3 * time.Minute
	// ReportMaxWait caps how long ReportWithRetry will block when retrying.
	// Callers are typically on the module-init failure path and system-probe exits
	// right after, so we cannot rely on a background goroutine surviving long enough.
	ReportMaxWait = 30 * time.Second
)

// Reporter sends health issues and resolutions to the core agent via the
// AgentSecure ReportHealthIssue / ResolveHealthIssue gRPC endpoints.
type Reporter struct {
	ipc         ipcdef.Component
	callTimeout time.Duration
	maxWait     time.Duration
	// newClientFn overrides newClient when set; used in tests to inject a mock.
	newClientFn func() (pb.AgentSecureClient, error)
}

// New returns a Reporter that uses the given IPC component for mTLS and auth.
func New(ipc ipcdef.Component) *Reporter {
	return &Reporter{
		ipc:         ipc,
		callTimeout: DefaultCallTimeout,
		maxWait:     DefaultMaxWait,
	}
}

// ReportWithRetry sends issue to the core agent. It first blocks for ReportMaxWait (30s)
// so the report survives if system-probe exits immediately after a module-init failure.
// If undelivered after that window but system-probe is still running (other modules are
// loaded), it continues retrying in a background goroutine for the remaining DefaultMaxWait.
func (r *Reporter) ReportWithRetry(issue *healthplatformpayload.Issue) {
	op := "report " + issue.GetId()
	if r.retryWithBackoff(op, ReportMaxWait, func() error {
		return r.Report(context.Background(), issue)
	}) {
		return
	}
	// Synchronous window exhausted without success — keep retrying in the background
	// for the rest of DefaultMaxWait in case the process stays alive.
	remaining := r.maxWait - ReportMaxWait
	if remaining > 0 {
		go r.retryWithBackoff(op, remaining, func() error {
			return r.Report(context.Background(), issue)
		})
	}
}

// ResolveWithRetry clears issueID on the core agent in a background goroutine,
// retrying with exponential backoff until the call succeeds or DefaultMaxWait elapses.
func (r *Reporter) ResolveWithRetry(issueID string) {
	go r.retryWithBackoff("resolve "+issueID, r.maxWait, func() error {
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
	ctx, cancel := context.WithTimeout(ctx, r.callTimeout)
	defer cancel()
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{Issue: issue})
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
	if r.newClientFn != nil {
		return r.newClientFn()
	}
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

// StringStruct converts a map[string]string to a *structpb.Struct suitable for
// the Extra field of a health issue. Any system-probe module building an issue
// can use this instead of reimplementing the conversion.
func StringStruct(fields map[string]string) *structpb.Struct {
	pb := make(map[string]*structpb.Value, len(fields))
	for k, v := range fields {
		pb[k] = structpb.NewStringValue(v)
	}
	return &structpb.Struct{Fields: pb}
}

// retryWithBackoff retries fn with exponential backoff until it succeeds or maxWait elapses.
// Returns true if fn succeeded, false if the deadline was reached.
func (r *Reporter) retryWithBackoff(op string, maxWait time.Duration, fn func() error) bool {
	deadline := time.Now().Add(maxWait)
	backoff := 2 * time.Second
	for {
		err := fn()
		if err == nil {
			return true
		}
		log.Warnf("health platform: %s failed (will retry in %s): %v", op, backoff, err)
		if time.Now().After(deadline) {
			log.Warnf("health platform: gave up on %s after %s", op, maxWait)
			return false
		}
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
