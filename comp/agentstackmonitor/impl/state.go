// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
)

type stateKey struct {
	podUID        string
	containerName string
}

type subjectState struct {
	subjectKind   SubjectKind
	controller    controllerRef
	namespace     string
	podName       string
	podUID        string
	containerName string

	memRatio      ring[float64]
	restartDeltas ring[int32]
	waitingReason ring[string]

	cpuTotal float64
	memUsage float64
	memLimit float64

	// haveRestartBaseline avoids treating a container's very first observed
	// RestartCount as a fresh delta — historical restarts predate our window.
	lastRestartCount    int32
	haveRestartBaseline bool

	lastTermReason    string
	restartAtLastTerm int32
	haveTermBaseline  bool

	podReason string
	podPhase  string

	lastSeenAt time.Time
}

func (s *subjectState) observeStats(stats *provider.ContainerStats) {
	if stats == nil {
		return
	}
	if stats.CPU != nil && stats.CPU.Total != nil {
		s.cpuTotal = *stats.CPU.Total
	}
	if stats.Memory != nil {
		if stats.Memory.UsageTotal != nil {
			s.memUsage = *stats.Memory.UsageTotal
		}
		if stats.Memory.Limit != nil {
			s.memLimit = *stats.Memory.Limit
		}
		if s.memLimit > 0 && s.memUsage > 0 {
			s.memRatio.push(s.memUsage / s.memLimit)
		}
	}
}

func (s *subjectState) observeStatus(status *workloadmeta.KubernetesContainerStatus) {
	if s.haveRestartBaseline {
		delta := status.RestartCount - s.lastRestartCount
		if delta < 0 {
			delta = 0
		}
		s.restartDeltas.push(delta)
	} else {
		s.restartDeltas.push(0)
		s.haveRestartBaseline = true
	}
	s.lastRestartCount = status.RestartCount

	waitReason := ""
	if status.State.Waiting != nil {
		waitReason = status.State.Waiting.Reason
	}
	s.waitingReason.push(waitReason)

	termReason := ""
	if status.LastTerminationState.Terminated != nil {
		termReason = status.LastTerminationState.Terminated.Reason
	}
	if termReason != s.lastTermReason {
		s.lastTermReason = termReason
		s.restartAtLastTerm = status.RestartCount
		s.haveTermBaseline = termReason != ""
	}
}

func (s *subjectState) observePod(pod *workloadmeta.KubernetesPod) {
	s.podReason = pod.Reason
	s.podPhase = pod.Phase
	s.namespace = pod.Namespace
	s.podName = pod.Name
	s.podUID = string(pod.EntityID.ID)
}
