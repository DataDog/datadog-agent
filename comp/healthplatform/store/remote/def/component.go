// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remote defines the remote health-platform issue loader component.
package remote

import "context"

// team: fleet-remediation

// Issue is the lifecycle metadata returned by the Agent Health API.
type Issue struct {
	IssueID   string
	IssueName string
	IssueType string
	Category  string
}

// Component loads issue lifecycle metadata from the Agent Health API.
type Component interface {
	Load(context.Context) ([]Issue, error)
}
