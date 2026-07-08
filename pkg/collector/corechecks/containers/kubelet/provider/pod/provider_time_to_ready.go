// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
)

const (
	// maxStartupDuration is the maximum duration we consider valid for a pod to
	// go from scheduled to ready or running.
	maxStartupDuration = 1 * time.Hour

	// maxReadyLag is the maximum allowed gap between the latest running container
	// start time and the Ready condition LastTransitionTime.
	maxReadyLag = 15 * time.Minute

	// podPhaseRunning is the phase of a pod that is running.
	podPhaseRunning = "Running"

	// podConditionTypePodScheduled is the type of a pod condition that is scheduled.
	podConditionTypePodScheduled = "PodScheduled"

	// podConditionTypeReady is the type of a pod condition that is ready.
	podConditionTypeReady = "Ready"
)

// podStartupTimings holds the computed durations from pod scheduling to
// the ready and running states.
type podStartupTimings struct {
	timeToReady   time.Duration
	timeToRunning time.Duration
}

// computePodStartupTimings extracts time_to_ready and time_to_running from the
// pod's conditions and container statuses, applying some heuristic validation.
//
// Heuristics:
//  1. Any container restart (regular or init) makes all timings unreliable.
//  2. Requires a PodScheduled condition with a valid LastTransitionTime.
//  3. Ready LastTransitionTime must be after PodScheduled LastTransitionTime.
//  4. Ready LastTransitionTime must be within maxReadyLag of the latest running
//     container start time. A larger gap indicates the Ready condition was
//     overwritten by a re-ready cycle after containers had already been running.
//  5. Durations cannot be negative or exceed maxStartupDuration.
//
// The "running" definition mirrors the kubelet's HasAnyActiveRegularContainerStarted:
// at least one regular (non-init) container has started.
// Reference: https://github.com/kubernetes/kubernetes/blob/08d246509c/pkg/kubelet/container/helpers.go#L511
func computePodStartupTimings(pod *workloadmeta.KubernetesPod) (podStartupTimings, error) {
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

	var earliestRunningTime, latestRunningTime time.Time
	for _, cs := range pod.ContainerStatuses {
		if cs.State.Running == nil || cs.State.Running.StartedAt.IsZero() {
			continue
		}
		t := cs.State.Running.StartedAt
		if earliestRunningTime.IsZero() || t.Before(earliestRunningTime) {
			earliestRunningTime = t
		}
		if latestRunningTime.IsZero() || t.After(latestRunningTime) {
			latestRunningTime = t
		}
	}

	var timings podStartupTimings

	// time_to_ready: skip if Ready time is too far from when the latest container
	// started, which indicates a re-ready cycle overwrote LastTransitionTime.
	// If no containers are running we cannot apply this heuristic, so we allow it.
	if readyTime.After(scheduledTime) {
		readyLag := readyTime.Sub(latestRunningTime)
		if latestRunningTime.IsZero() || readyLag <= maxReadyLag {
			d := readyTime.Sub(scheduledTime)
			if d > 0 && d <= maxStartupDuration {
				timings.timeToReady = d
			}
		}
	}

	// time_to_running: use the earliest running container start time.
	if !earliestRunningTime.IsZero() {
		d := earliestRunningTime.Sub(scheduledTime)
		if d > 0 && d <= maxStartupDuration {
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

// generatePodStartupMetrics emits kubernetes.pod.scheduled_time_to_ready and
// kubernetes.pod.scheduled_time_to_running as gauges for pods
func (p *Provider) generatePodStartupMetrics(s sender.Sender, pod *workloadmeta.KubernetesPod) {
	// consider only pods that are currently Ready and Running
	if pod.ID == "" || pod.Phase != podPhaseRunning || !pod.Ready {
		return
	}

	timings, err := computePodStartupTimings(pod)
	if err != nil {
		return
	}

	entityID := types.NewEntityID(types.KubernetesPodUID, pod.ID)
	tagList, _ := p.tagger.Tag(entityID, types.OrchestratorCardinality)
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	// Using GaugeNoIndex writes metric_type_agent_hidden into the metric origin, so that it is not indexed by the backend
	// https://github.com/DataDog/dd-source/blob/56e80f74a5cd7cab793ad52f0071a7c5c5189209/domains/metrics/shared/libs/proto/origin/origin.proto#L145-L148
	if timings.timeToReady > 0 {
		s.GaugeNoIndex(common.KubeletMetricsPrefix+"pod.scheduled_time_to_ready", timings.timeToReady.Seconds(), "", tagList)
	}

	if timings.timeToRunning > 0 {
		s.GaugeNoIndex(common.KubeletMetricsPrefix+"pod.scheduled_time_to_running", timings.timeToRunning.Seconds(), "", tagList)
	}
}
