// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package clusteridresolver provides the Kubernetes cluster identity.
package clusteridresolver

import (
	"context"
	"errors"
)

// team: container-platform

var (
	// ErrNotResolved indicates that resolution has not succeeded yet.
	ErrNotResolved = errors.New("Kubernetes cluster ID is not resolved")
	// ErrDisabled indicates that cluster identity resolution is unavailable in this environment.
	ErrDisabled = errors.New("Kubernetes cluster ID resolution is disabled")
)

// Component owns cluster ID resolution, retries and caching for this process.
// Reads are safe concurrently and never perform network requests.
type Component interface {
	// GetID returns the resolved ID, or the latest resolution error without waiting.
	GetID() (string, error)
	// WaitForID waits for resolution or shutdown. Canceling ctx only cancels this
	// caller's wait; the shared resolution continues until the component stops.
	WaitForID(ctx context.Context) (string, error)
}
