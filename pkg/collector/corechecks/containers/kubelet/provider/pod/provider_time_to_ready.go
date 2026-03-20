// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// maxTimeToReady is the maximum duration we consider valid for a pod to
	// go from scheduled to ready. Anything beyond this is likely bad data
	// from a readiness probe re-transition rather than the initial startup.
	maxTimeToReady = 1 * time.Hour

	// maxReadinessProbeFailures is the threshold above which we consider the
	// pod's Ready condition LastTransitionTime unreliable
	maxReadinessProbeFailures = 3

	// probesEndpointPath is the path to the kubelet's /metrics/probes endpoint.
	probesEndpointPath = "/metrics/probes"

	// proberProbeTotalMetricName is the name of the prometheus metric that
	// counts the number of readiness probe results.
	proberProbeTotalMetricName = "prober_probe_total"

	// podPhaseRunning is the phase of a pod that is running.
	podPhaseRunning = "Running"

	// podConditionTypePodScheduled is the type of a pod condition that is scheduled.
	podConditionTypePodScheduled = "PodScheduled"

	// podConditionTypeReady is the type of a pod condition that is ready.
	podConditionTypeReady = "Ready"
)

// readinessProbeTooManyFailuresMap maps pod UID to whether we think the pod's Ready condition is unreliable.
// true means at least one container has had more than maxReadinessProbeFailures failures.
// false means we think the pod's Ready condition is reliable.
type readinessProbeTooManyFailuresMap map[string]bool

// scrapeReadinessFailures queries the kubelet's /metrics/probes endpoint and
// returns a map from pod_uid to whether we think the pod's Ready condition is unreliable.
func scrapeReadinessFailures(kc kubelet.KubeUtilInterface, timeout time.Duration) readinessProbeTooManyFailuresMap {
	if kc == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	data, _, err := kc.QueryKubelet(ctx, probesEndpointPath)
	if err != nil {
		log.Debugf("Failed to scrape %s: %v", probesEndpointPath, err)
		return nil
	}

	families, err := prom.ParseMetrics(data)
	if err != nil {
		log.Debugf("Failed to parse %s metrics: %v", probesEndpointPath, err)
		return nil
	}

	result := make(readinessProbeTooManyFailuresMap)
	for _, fam := range families {
		if fam.Name != proberProbeTotalMetricName {
			continue
		}
		for _, sample := range fam.Samples {
			if sample.Metric["probe_type"] != "Readiness" || sample.Metric["result"] != "failed" {
				continue
			}
			podUID := sample.Metric["pod_uid"]
			if podUID == "" {
				continue
			}

			result[podUID] = result[podUID] || sample.Value > maxReadinessProbeFailures
		}
	}
	return result
}

// podStartupTimings holds the computed durations from pod scheduling to
// the ready and running states. Either field may be zero if the
// corresponding timing could not be determined.
type podStartupTimings struct {
	timeToReady   time.Duration
	timeToRunning time.Duration
}

// computePodStartupTimings extracts time_to_ready and time_to_running from the
// pod's conditions and container statuses, applying all heuristic validation.
// Returns an error if none of the timings can be trusted.
//
// Heuristics applied:
//   - Requires a PodScheduled condition with a valid LastTransitionTime.
//   - Any container restart (regular or init) makes all timings unreliable.
//   - Readiness probe failures above the threshold make time_to_ready unreliable
//     (time_to_running is still valid since container start time is unaffected).
//   - Durations that are negative or exceed maxTimeToReady are discarded.
//
// The "running" definition mirrors the kubelet's HasAnyActiveRegularContainerStarted:
// at least one regular (non-init) container has started.
// Reference: https://github.com/kubernetes/kubernetes/blob/08d246509c/pkg/kubelet/container/helpers.go#L511
func computePodStartupTimings(pod *workloadmeta.KubernetesPod, podReadinessFailure bool) (podStartupTimings, error) {
	if anyContainerRestarted(pod) {
		return podStartupTimings{}, errors.New("container has restarted")
	}

	var scheduledTime, readyTime time.Time
	for _, cond := range pod.Conditions {
		switch cond.Type {
		case podConditionTypePodScheduled:
			if cond.Status == "True" {
				scheduledTime = cond.LastTransitionTime
			}
		case podConditionTypeReady:
			if cond.Status == "True" {
				readyTime = cond.LastTransitionTime
			}
		}
	}

	if scheduledTime.IsZero() {
		return podStartupTimings{}, errors.New("pod has no PodScheduled condition")
	}

	var timings podStartupTimings

	// time_to_ready: only trust if readiness probes haven't failed too many
	// times, otherwise LastTransitionTime may reflect a re-ready cycle.
	if !podReadinessFailure && !readyTime.IsZero() {
		d := readyTime.Sub(scheduledTime)
		if d > 0 && d <= maxTimeToReady {
			timings.timeToReady = d
		}
	}

	// time_to_running: earliest regular container StartedAt is not affected
	// by readiness probes, so it's always trustworthy when present.
	var earliestRunningTime time.Time
	for _, cs := range pod.ContainerStatuses {
		if cs.State.Running == nil || cs.State.Running.StartedAt.IsZero() {
			continue
		}
		if earliestRunningTime.IsZero() || cs.State.Running.StartedAt.Before(earliestRunningTime) {
			earliestRunningTime = cs.State.Running.StartedAt
		}
	}

	if !earliestRunningTime.IsZero() {
		d := earliestRunningTime.Sub(scheduledTime)
		if d > 0 && d <= maxTimeToReady {
			timings.timeToRunning = d
		}
	}

	if timings.timeToReady == 0 && timings.timeToRunning == 0 {
		return podStartupTimings{}, errors.New("no valid timings could be computed")
	}

	return timings, nil
}

// anyContainerRestarted returns true if any container (regular or init) has
// been restarted.
func anyContainerRestarted(pod *workloadmeta.KubernetesPod) bool {
	for _, cs := range pod.ContainerStatuses {
		if cs.RestartCount > 0 {
			return true
		}
	}
	for _, cs := range pod.InitContainerStatuses {
		if cs.RestartCount > 0 {
			return true
		}
	}
	return false
}

// generatePodStartupMetrics emits kubernetes.pod.time_to_ready and
// kubernetes.pod.time_to_running as gauges for pods that pass heuristic
// filters indicating the timings likely reflect the genuine first startup.
func (p *Provider) generatePodStartupMetrics(s sender.Sender, pod *workloadmeta.KubernetesPod, readinessFailures readinessProbeTooManyFailuresMap) {
	// Only consider pods that are currently Ready and Running.
	if pod.Phase != podPhaseRunning || !pod.Ready {
		return
	}

	if pod.CreationTimestamp.IsZero() || pod.ID == "" {
		return
	}

	timings, err := computePodStartupTimings(pod, readinessFailures[pod.ID])
	if err != nil {
		return
	}

	entityID := types.NewEntityID(types.KubernetesPodUID, pod.ID)
	// OrchestratorCardinality includes pod_name (low only has namespace/owner).
	// pod_uid is high cardinality and not included here.
	tagList, _ := p.tagger.Tag(entityID, types.OrchestratorCardinality)
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	if timings.timeToReady > 0 {
		s.Gauge(common.KubeletMetricsPrefix+"pod.time_to_ready", timings.timeToReady.Seconds(), "", tagList)
	}

	if timings.timeToRunning > 0 {
		s.Gauge(common.KubeletMetricsPrefix+"pod.time_to_running", timings.timeToRunning.Seconds(), "", tagList)
	}
}
