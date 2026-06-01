// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issuereporter defines the interface that Go integrations use to report
// health issues into the health platform store.
package issuereporter

// team: agent-health

import healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

// Component is the interface exposed to Go integrations for reporting health issues.
// It is a subset of the health platform store interface; the store satisfies it directly.
type Component interface {
	// AcceptIssue stores a fully-built issue keyed by issue.Id, bypassing template
	// lookup. Call ResolveIssue with the same Id to clear it.
	AcceptIssue(issue *healthplatformpayload.Issue) error

	// ResolveIssue marks the issue with the given Id as resolved.
	// No-op if the issue is not currently active.
	ResolveIssue(issueID string)
}
