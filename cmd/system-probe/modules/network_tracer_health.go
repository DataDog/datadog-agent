// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm) || darwin

package modules

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	networkProbeIssueID   = "network-probe-init-failure"
	networkProbeIssueName = "network_probe_init_failure"

	// healthReportTimeout caps a single gRPC call attempt.
	healthReportTimeout = 5 * time.Second
	// healthReportMaxWait is how long we retry before giving up.
	// Covers the case where system-probe starts before the core agent gRPC server is up.
	healthReportMaxWait = 3 * time.Minute
)

// reportNetworkProbeInitFailure sends a thin IssueReport to the core agent asynchronously.
// The core agent resolves it via the template registry (BuildIssue), keeping issue-shape
// logic in one place.
func reportNetworkProbeInitFailure(deps module.FactoryDependencies, initErr error, npmEnabled, usmEnabled bool) {
	errStr := "unknown error"
	if initErr != nil {
		errStr = initErr.Error()
	}
	report := &pb.RemoteIssueReport{
		IssueId:   networkProbeIssueID,
		IssueName: networkProbeIssueName,
		Source:    "system-probe",
		Context: map[string]string{
			"error":       errStr,
			"npm_enabled": fmt.Sprintf("%v", npmEnabled),
			"usm_enabled": fmt.Sprintf("%v", usmEnabled),
		},
	}
	go func() {
		retryWithBackoff("report", func() error {
			return sendHealthReport(deps, report)
		})
	}()
}

// resolveNetworkProbeInitFailure clears a previously reported network probe failure.
// Called on successful initialization to clean up stale issues from prior failed runs.
func resolveNetworkProbeInitFailure(deps module.FactoryDependencies) {
	go func() {
		retryWithBackoff("resolve", func() error {
			return sendHealthResolve(deps, networkProbeIssueID)
		})
	}()
}

func newAgentSecureClient(deps module.FactoryDependencies) (pb.AgentSecureClient, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("get IPC address: %w", err)
	}
	return ddgrpc.GetDDAgentSecureClient(
		context.Background(),
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		deps.Ipc.GetTLSClientConfig().Clone(),
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(deps.Ipc.GetAuthToken())),
	)
}

func sendHealthReport(deps module.FactoryDependencies, report *pb.RemoteIssueReport) error {
	client, err := newAgentSecureClient(deps)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthReportTimeout)
	defer cancel()
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{
		Payload: &pb.ReportHealthIssueRequest_Report{Report: report},
	})
	return err
}

func sendHealthResolve(deps module.FactoryDependencies, issueID string) error {
	client, err := newAgentSecureClient(deps)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthReportTimeout)
	defer cancel()
	_, err = client.ResolveHealthIssue(ctx, &pb.ResolveHealthIssueRequest{IssueId: issueID})
	return err
}

// retryWithBackoff calls fn repeatedly with exponential backoff until it succeeds
// or healthReportMaxWait elapses.
func retryWithBackoff(op string, fn func() error) {
	deadline := time.Now().Add(healthReportMaxWait)
	backoff := 2 * time.Second
	for {
		err := fn()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			log.Warnf("health platform: gave up on %s of network probe issue after %s: %v", op, healthReportMaxWait, err)
			return
		}
		log.Debugf("health platform: retrying %s of network probe issue in %s (err: %v)", op, backoff, err)
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
