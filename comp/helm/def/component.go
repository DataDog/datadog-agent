// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package helm provides a Helm SDK wrapper for the cluster-agent.
package helm

import "context"

// team: container-platform

// Component exposes Helm release operations against the local cluster.
type Component interface {
	// Rollback rolls a release back to a prior revision. A revision of 0
	// rolls back to the immediately previous successful release.
	Rollback(ctx context.Context, releaseName, namespace string, revision int) error
}
