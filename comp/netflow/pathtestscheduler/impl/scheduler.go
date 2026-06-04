// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl contains the implementation of the pathtestscheduler component.
package impl

import (
	"slices"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	pathtestscheduler "github.com/DataDog/datadog-agent/comp/netflow/pathtestscheduler/def"
	npcollector "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/def"
)

const (
	// Metric names follow the convention from the plan §1.6.
	metricFlowsReceived      = "datadog.networkpath.netflow_scheduler.flows_received"
	metricConnectionsEmitted = "datadog.networkpath.netflow_scheduler.connections_emitted"
	metricDropped            = "datadog.networkpath.netflow_scheduler.dropped"
)

// dependencies holds the fx-injected dependencies of the pathtestscheduler component.
type dependencies struct {
	compdef.In

	Logger      log.Component
	AgentConfig config.Component
	Statsd      ddgostatsd.ClientInterface
	NpCollector npcollector.Component
}

// Provides defines the output of the pathtestscheduler component.
type Provides struct {
	compdef.Out

	Comp pathtestscheduler.Component
}

// schedulerImpl is the concrete implementation of pathtestscheduler.Component.
type schedulerImpl struct {
	logger      log.Component
	statsd      ddgostatsd.ClientInterface
	cfg         *schedulerConfig
	npcollector npcollector.Component
}

// NewComponent creates a new pathtestscheduler component.
func NewComponent(deps dependencies) (Provides, error) {
	cfg, err := newSchedulerConfig(deps.AgentConfig)
	if err != nil {
		return Provides{}, err
	}

	s := &schedulerImpl{
		logger:      deps.Logger,
		statsd:      deps.Statsd,
		cfg:         cfg,
		npcollector: deps.NpCollector,
	}

	return Provides{Comp: s}, nil
}

// ScheduleFromFlows implements pathtestscheduler.Component. It converts a batch
// of NetFlow records into NetworkPathConnection values and hands them off to
// npcollector.ScheduleNetworkPathTests.
//
// The method is non-blocking and never error-propagates; failures are reported
// via statsd metrics.
func (s *schedulerImpl) ScheduleFromFlows(flows []*common.Flow) {
	if !s.cfg.enabled {
		return
	}

	// Emit flows-received metric.
	flowCount := len(flows)
	_ = s.statsd.Count(metricFlowsReceived, int64(flowCount), nil, 1)

	if flowCount == 0 {
		return
	}

	// Convert flows to connections (stage 1 + stage 2 aggregation).
	connections, dropCounts := AggregateFlows(flows)

	// Emit per-reason drop metrics from the converter.
	for reason, count := range dropCounts {
		if count > 0 {
			_ = s.statsd.Count(metricDropped, int64(count), []string{"reason:" + reason}, 1)
		}
	}

	// Apply dest_excludes filter. Use conn.DestIP (the canonical preserved IP)
	// rather than re-parsing conn.Dest.Addr(). Accumulate the drop count and
	// emit one aggregated metric to avoid per-connection statsd calls.
	if len(s.cfg.destExcludePrefixes) > 0 {
		var excludedCount int64
		filtered := connections[:0]
		for _, conn := range connections {
			excluded := false
			for _, prefix := range s.cfg.destExcludePrefixes {
				if prefix.Contains(conn.DestIP) {
					excluded = true
					break
				}
			}
			if excluded {
				excludedCount++
			} else {
				filtered = append(filtered, conn)
			}
		}
		if excludedCount > 0 {
			_ = s.statsd.Count(metricDropped, excludedCount, []string{"reason:dest_excluded"}, 1)
		}
		connections = filtered
	}

	// Apply max_destinations_per_flush cap.
	maxDest := s.cfg.maxDestinationsPerFlush
	if maxDest > 0 && len(connections) > maxDest {
		overflow := len(connections) - maxDest
		_ = s.statsd.Count(metricDropped, int64(overflow), []string{"reason:max_destinations_cap"}, 1)
		connections = connections[:maxDest]
	}

	// Emit connections-emitted metric.
	_ = s.statsd.Count(metricConnectionsEmitted, int64(len(connections)), nil, 1)

	if len(connections) == 0 {
		return
	}

	// Hand off to npcollector.
	s.npcollector.ScheduleNetworkPathTests(slices.Values(connections))
}

// Ensure schedulerImpl satisfies the Component interface at compile time.
var _ pathtestscheduler.Component = (*schedulerImpl)(nil)

// noopScheduler is a zero-allocation no-op used when the component is disabled
// at the fx level (e.g., when npcollector is unavailable). It satisfies the
// Component interface without requiring a real npcollector.
type noopScheduler struct{}

func (n *noopScheduler) ScheduleFromFlows(_ []*common.Flow) {}

// NewNoopComponent returns a pathtestscheduler.Component that does nothing.
// Useful for tests or when the feature is fully disabled.
func NewNoopComponent() pathtestscheduler.Component {
	return &noopScheduler{}
}
