// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/metricsstore"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// workloadKindMetricsSource provides per-kind workload counts for periodic metric emission.
type workloadKindMetricsSource interface {
	countByKind() map[string]int
	countDisabledByKind(time.Time) map[string]int
}

const (
	tagCapacityTypeSpot     = "capacity_type:spot"
	tagCapacityTypeOnDemand = "capacity_type:on_demand"
	tagJoinLeader           = metrics.JoinLeaderLabel + ":" + metrics.JoinLeaderValue
)

const metricsFlushInterval = 30 * time.Second

// workloadSnapshot holds per-workload pod counts for periodic metric emission.
type workloadSnapshot struct {
	ref                                        objectRef
	spot, onDemand, excessSpot, excessOnDemand int
}

// telemetry emits the spot scheduler's metrics.
type telemetry struct {
	sender          metricsstore.MetricsSender
	workloadMetrics *metricsstore.MetricsStore[workloadSnapshot]
	globalTagsFunc  func() []string
	isLeader        func() bool
}

func newTelemetry(s metricsstore.MetricsSender, isLeader func() bool, globalTagsFunc func() []string) *telemetry {
	t := &telemetry{
		sender:         s,
		globalTagsFunc: globalTagsFunc,
		isLeader:       isLeader,
	}
	t.workloadMetrics = metricsstore.NewMetricsStore(generateWorkloadMetrics, s, isLeader, t.globalTags)
	return t
}

func (t *telemetry) globalTags() []string {
	if t.globalTagsFunc == nil {
		return []string{tagJoinLeader}
	}
	return append(t.globalTagsFunc(), tagJoinLeader)
}

// start launches periodic metric emission and blocks until ctx is cancelled.
func (t *telemetry) start(ctx context.Context, cs workloadKindMetricsSource) {
	ticker := time.NewTicker(metricsFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			t.refreshWorkloadKindMetrics(cs, now)

			err := t.workloadMetrics.WriteAll()
			if err != nil {
				log.Errorf("Failed to write metrics: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// refreshWorkloadKindMetrics emits fresh workload and active-fallback counts per kind.
func (t *telemetry) refreshWorkloadKindMetrics(cs workloadKindMetricsSource, now time.Time) {
	if !t.isLeader() {
		return
	}
	counts := cs.countByKind()
	fallbacks := cs.countDisabledByKind(now)
	gt := t.globalTags()
	for _, r := range spotWorkloadResources {
		tags := append(workloadKindTags(r.kind), gt...)
		t.sender.Gauge(MetricNameWorkloads, float64(counts[r.kind]), "", tags)
		t.sender.Gauge(MetricNameActiveFallbacks, float64(fallbacks[r.kind]), "", tags)
	}
	t.sender.Commit()
}

func generateWorkloadMetrics(snap workloadSnapshot) metricsstore.StructuredMetrics {
	baseTags := workloadTags(snap.ref)
	spotTags := append(append([]string{}, baseTags...), tagCapacityTypeSpot)
	onDemandTags := append(append([]string{}, baseTags...), tagCapacityTypeOnDemand)
	return metricsstore.StructuredMetrics{
		{Name: MetricNamePods, Type: metricsstore.MetricTypeGauge, Value: float64(snap.spot), Tags: spotTags},
		{Name: MetricNamePods, Type: metricsstore.MetricTypeGauge, Value: float64(snap.onDemand), Tags: onDemandTags},
		{Name: MetricNameExcessPods, Type: metricsstore.MetricTypeGauge, Value: float64(snap.excessSpot), Tags: spotTags},
		{Name: MetricNameExcessPods, Type: metricsstore.MetricTypeGauge, Value: float64(snap.excessOnDemand), Tags: onDemandTags},
	}
}

// observeWorkload updates workload snapshot in the metrics store.
func (t *telemetry) observeWorkload(snap workloadSnapshot) {
	t.workloadMetrics.Add(snap.ref.String(), snap)
}

// deleteWorkload removes workload snapshot from the metrics store.
func (t *telemetry) deleteWorkload(o objectRef) {
	t.workloadMetrics.Delete(o.String())
}

// observeFallback records one fallback event for a workload.
func (t *telemetry) observeFallback(o objectRef) {
	if !t.isLeader() {
		return
	}
	t.sender.Count(MetricNameFallbacks, 1, "", slices.Concat(t.globalTags(), workloadTags(o)))
	t.sender.Commit()
}

// observeRebalanceEviction records one pod eviction by the rebalancer for a workload.
func (t *telemetry) observeRebalanceEviction(o objectRef, isSpot bool) {
	if !t.isLeader() {
		return
	}
	capacityType := tagCapacityTypeOnDemand
	if isSpot {
		capacityType = tagCapacityTypeSpot
	}
	t.sender.Count(MetricNameRebalanceEvictions, 1, "", append(slices.Concat(t.globalTags(), workloadTags(o)), capacityType))
	t.sender.Commit()
}

// observePendingSeconds records the time a spot pod spent in the Pending phase.
func (t *telemetry) observePendingSeconds(d time.Duration) {
	if !t.isLeader() {
		return
	}
	t.sender.Distribution(MetricNamePendingSeconds, d.Seconds(), "", t.globalTags())
	t.sender.Commit()
}

func workloadTags(o objectRef) []string {
	kind := strings.ToLower(o.Kind)
	return []string{
		"kube_namespace:" + o.Namespace,
		"kube_" + kind + ":" + o.Name,
	}
}

func workloadKindTags(kind string) []string {
	return []string{
		"workload_kind:" + strings.ToLower(kind),
	}
}
