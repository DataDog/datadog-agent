// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotereporter forwards health issues detected in a sub-agent process
// (system-probe, ...) to the health platform store of the core agent.
package remotereporter

// team: fleet-remediation

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// Component reports and resolves health issues in the core agent.
//
// Calls never block on the core agent: the desired state of each issue is
// recorded and delivered in the background, with retries while the core agent
// is unreachable. Active issues are periodically re-sent so they survive core
// agent restarts.
type Component interface {
	// ReportIssue marks the issue as active. Id and IssueName must be set.
	ReportIssue(issue *healthplatformpayload.Issue) error
	// ResolveIssue marks the issue with the given ID as resolved. Resolving an
	// issue that is not active in the core agent is a no-op there.
	ResolveIssue(issueID string)
}
