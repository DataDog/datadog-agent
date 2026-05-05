// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discovery implements probe-based "advanced auto-config" — running
// a verifying probe against a discovered Service to derive instance config
// values that cannot be expressed by template substitution alone.
package discovery

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
)

// ProbeResult is the outcome of a successful probe.
type ProbeResult struct {
	// Port is the discovered TCP port that responded successfully to the
	// probe.
	Port uint16
}

// Prober probes a Service against a DiscoveryConfig and returns a result
// when one of the candidate (host, port, path) tuples verifies. If no
// candidate verifies within the budget, ok is false.
type Prober interface {
	Probe(ctx context.Context, cfg *integration.DiscoveryConfig, svc listeners.Service) (result ProbeResult, ok bool)
}
