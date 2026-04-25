// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package forwarder defines the interface for the health platform forwarder.
package forwarder

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

// team: agent-health

// IssueProvider provides the current set of issues to report.
type IssueProvider interface {
	GetAllIssues() (int, map[string]*healthplatformpayload.Issue)
}

// Component is the forwarder component.
type Component interface {
	// SetProvider wires the issue provider after construction, breaking the
	// circular fx dependency between core and forwarder.
	// Must be called before the first send fires (i.e. from the core lifecycle start hook).
	SetProvider(provider IssueProvider)
}
