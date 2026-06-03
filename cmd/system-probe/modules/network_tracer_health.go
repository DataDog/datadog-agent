// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm) || darwin

package modules

import (
	"fmt"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/healthreporter"
)

const (
	networkProbeIssueID   = "network-probe-init-failure"
	networkProbeIssueName = "network_probe_init_failure"
)

// reportNetworkProbeInitFailure sends a thin IssueReport to the core agent.
// The core agent resolves it via the template registry (BuildIssue), so
// issue-shape logic lives only in comp/healthplatform/issues/networkprobefailure.
func reportNetworkProbeInitFailure(deps module.FactoryDependencies, initErr error, npmEnabled, usmEnabled bool) {
	errStr := "unknown error"
	if initErr != nil {
		errStr = initErr.Error()
	}
	healthreporter.New(deps.Ipc).ReportWithRetry(&pb.RemoteIssueReport{
		IssueId:   networkProbeIssueID,
		IssueName: networkProbeIssueName,
		Source:    "system-probe",
		Context: map[string]string{
			"error":       errStr,
			"npm_enabled": fmt.Sprintf("%v", npmEnabled),
			"usm_enabled": fmt.Sprintf("%v", usmEnabled),
		},
	})
}

// resolveNetworkProbeInitFailure clears a previously reported network probe failure.
// Called on successful initialization to clean up stale issues from prior failed runs.
func resolveNetworkProbeInitFailure(deps module.FactoryDependencies) {
	healthreporter.New(deps.Ipc).ResolveWithRetry(networkProbeIssueID)
}
