// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package highmemoryk8s

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

const (
	monitorName = "high_memory_k8s"
	// Both gauges are recorded by the container checks for the Datadog Agent pods running on the node,
	// see pkg/collector/corechecks/containers/agentperformance.
	usageMetric = "agent_performance__memory_usage"
	limitMetric = "agent_performance__memory_limit"

	threshold = 0.9
	window    = 5 * time.Minute

	// sourceContainer is the container whose memory the monitor watches, the source part of the issue id.
	sourceContainer = "agent"
)

// NewMonitor returns the monitor that reports the high memory issue to store while the node Agent uses more than
// 90% of its container memory limit on average over 5 minutes, and resolves it once usage goes back down.
func NewMonitor(store storedef.Component, hostname hostnameinterface.Component, logger log.Component) metricsevaluator.Monitor {
	return metricsevaluator.Monitor{
		Name:     monitorName,
		Metric:   usageMetric,
		DivideBy: limitMetric,
		// The cluster Agent and cluster checks runner pods on the node have their own series.
		Tags:        map[string]string{"kind": "agent"},
		Window:      window,
		Aggregation: metricsevaluator.AggregationAvg,
		Comparator:  metricsevaluator.ComparatorAbove,
		Threshold:   threshold,
		Callback: func(ev metricsevaluator.Event) {
			onTransition(store, hostname, logger, ev)
		},
	}
}

// onTransition reports or resolves the high memory issue when the monitor changes state.
func onTransition(store storedef.Component, hostname hostnameinterface.Component, logger log.Component, ev metricsevaluator.Event) {
	switch ev.State {
	case metricsevaluator.StateAlert:
		if err := reportIssue(store, hostname, ev.Value); err != nil {
			logger.Warnf("Error reporting agent health issue on high memory transition: %v", err)
		}
	case metricsevaluator.StateOK:
		resolveIssue(store)
	}
}

// reportIssue builds the high memory issue for the given usage ratio and reports it to store.
func reportIssue(store storedef.Component, hostname hostnameinterface.Component, usage float64) error {
	issue, err := HighMemoryK8sIssue{}.BuildIssue(map[string]string{
		contextKeyUsagePercent:     strconv.FormatFloat(usage*100, 'f', 0, 64),
		contextKeyThresholdPercent: strconv.FormatFloat(threshold*100, 'f', 0, 64),
		contextKeyWindow:           strconv.Itoa(int(window.Minutes())) + " minutes",
	})
	if err != nil {
		return err
	}
	// Path B reporters set the id themselves; the store rejects an empty one.
	issue.Id = issueID(store, hostname)
	return store.ReportIssue(issue)
}

// resolveIssue resolves every active high memory issue of this Agent's store. It resolves by issue name rather than
// recomputing the id, because the discriminator may have changed since the issue was reported (hostname before the
// DaemonSet uid is resolved, then the uid), and because an issue persisted by a previous run may carry an older id.
// The store is local to this Agent, so this never resolves an issue reported by another node.
func resolveIssue(store storedef.Component) {
	for _, id := range store.GetActiveIssueIDsByIssueName(IssueName) {
		store.ResolveIssue(id)
	}
}

// issueID returns IssueIDPrefix:<reporter>:<source>, so the id is unique per reporting Agent and per source of the
// high memory usage.
//
// The reporter is a hash of the Agent's discriminator: the uid of the DaemonSet owning the Agent pod when
// resolvable, else the hostname. All node Agents of a DaemonSet share one issue, since its remediation (the
// DaemonSet memory limit) is shared. The source is the container whose memory is watched.
//
// It is computed on every report, so a report made after the DaemonSet uid is resolved uses it.
func issueID(store storedef.Component, hostname hostnameinterface.Component) string {
	h := fnv.New64a()
	h.Write([]byte(store.IssueDiscriminator(hostname.GetSafe(context.Background()))))
	return fmt.Sprintf("%s:%016x:%s", IssueIDPrefix, h.Sum64(), sourceContainer)
}
