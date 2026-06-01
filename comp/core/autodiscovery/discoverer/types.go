// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discoverer implements probe-based "advanced auto-config" by
// dispatching the probe decision to a Python discover() classmethod on the
// integration's check class. The Python side returns the resolved instance
// configs directly; this package handles caching, time budgeting, and
// marshalling.
package discoverer

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

// Result is the output of a successful Discover call.
type Result struct {
	// Configs are the integration.Config values to schedule, one per dict
	// returned by the Python discover() classmethod. Each carries Name set to
	// the integration name and Instances populated from the Python result.
	Configs []integration.Config
}

// Discoverer dispatches discovery probes to the Python side via a Bridge.
// Returns ok=false when the probe did not match (no configs to schedule);
// any error is logged internally.
type Discoverer interface {
	Discover(ctx context.Context, integrationName string, svc listeners.Service) (Result, bool)

	// IsPending reports whether the cache holds a "still retrying" entry for
	// this (svcID, integration) pair (i.e. a failure entry whose retry
	// schedule isn't exhausted).
	IsPending(svcID, integrationName string) bool

	// Forget drops all cache entries for one service. Called by configmgr on
	// service removal so a restarted container starts fresh.
	Forget(svcID string)
}

// Bridge is the boundary between the discoverer and the Python runtime.
// Production uses pkg/collector/python; tests use an in-memory fake.
type Bridge interface {
	// DiscoverConfig invokes the integration's Python discovery bridge for the
	// service and returns discovered instance configs.
	DiscoverConfig(integrationName string, service python.DiscoveryService) ([]integration.Data, error)
}
