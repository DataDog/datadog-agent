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
	"google.golang.org/protobuf/types/known/anypb"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
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

// reportNetworkProbeInitFailure sends a health issue to the core agent asynchronously,
// retrying until the core agent's AgentSecure gRPC endpoint is reachable.
func reportNetworkProbeInitFailure(deps module.FactoryDependencies, initErr error, npmEnabled, usmEnabled bool) {
	issue := buildNetworkProbeIssue(initErr, npmEnabled, usmEnabled)
	go func() {
		retryWithBackoff("report", func() error {
			return sendHealthIssue(deps, issue)
		})
	}()
}

// resolveNetworkProbeInitFailure clears a previously reported network probe failure.
// Called on successful initialization so stale issues from prior failed runs are cleaned up.
func resolveNetworkProbeInitFailure(deps module.FactoryDependencies) {
	go func() {
		retryWithBackoff("resolve", func() error {
			return sendHealthResolve(deps, networkProbeIssueID)
		})
	}()
}

func buildNetworkProbeIssue(initErr error, npmEnabled, usmEnabled bool) *healthplatformpayload.Issue {
	errStr := "unknown error"
	if initErr != nil {
		errStr = initErr.Error()
	}

	var which string
	switch {
	case npmEnabled && usmEnabled:
		which = "NPM and USM"
	case npmEnabled:
		which = "NPM"
	case usmEnabled:
		which = "USM"
	default:
		which = "network monitoring"
	}

	return &healthplatformpayload.Issue{
		Id:          networkProbeIssueID,
		IssueName:   networkProbeIssueName,
		Title:       fmt.Sprintf("%s eBPF Probe Failed to Initialize", which),
		Description: fmt.Sprintf("%s is enabled but the eBPF network probe failed to load: %s", which, errStr),
		Category:    "runtime",
		Location:    "system-probe",
		Severity:    healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      "system-probe",
		Tags:        []string{"system-probe", "npm", "usm", "ebpf", "network-monitoring"},
		Remediation: &healthplatformpayload.Remediation{
			Summary: "Check kernel compatibility and system-probe capabilities, then restart system-probe.",
			Steps: []*healthplatformpayload.RemediationStep{
				{Order: 1, Text: "Check system-probe logs: journalctl -u datadog-agent-sysprobe or /var/log/datadog/system-probe.log"},
				{Order: 2, Text: "Verify kernel version (>= 4.4 for NPM, >= 4.14 for USM): uname -r"},
				{Order: 3, Text: "Check BTF availability for CO-RE probes: ls /sys/kernel/btf/vmlinux"},
				{Order: 4, Text: "Verify capabilities: CAP_NET_ADMIN, CAP_SYS_ADMIN (CAP_BPF on kernel >= 5.8)"},
				{Order: 5, Text: "If in a container, ensure privileged mode or a permissive seccomp profile"},
				{Order: 6, Text: "Restart after fixing: systemctl restart datadog-agent-sysprobe"},
			},
		},
	}
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

func sendHealthIssue(deps module.FactoryDependencies, issue *healthplatformpayload.Issue) error {
	client, err := newAgentSecureClient(deps)
	if err != nil {
		return err
	}
	packed, err := anypb.New(issue)
	if err != nil {
		return fmt.Errorf("pack health issue: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthReportTimeout)
	defer cancel()
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{Issue: packed})
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
