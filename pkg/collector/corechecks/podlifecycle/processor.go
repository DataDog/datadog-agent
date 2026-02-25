// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package podlifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	metricTimeToReady   = "kubernetes.pod.time_to_ready"
	metricTimeToRunning = "kubernetes.pod.time_to_running"
)

type podStatus int

const (
	podStatusPending podStatus = iota
	podStatusDone
)

type podRecord struct {
	status    podStatus
	createdAt time.Time
}

type processor struct {
	sender sender.Sender
	tagger tagger.Component

	mu        sync.Mutex
	podStates map[string]*podRecord // uid → state
}

func newProcessor(s sender.Sender, t tagger.Component) *processor {
	return &processor{
		sender:    s,
		tagger:    t,
		podStates: make(map[string]*podRecord),
	}
}

// start spawns a goroutine that periodically calls sender.Commit so that
// metrics emitted in handleSet are flushed to the aggregator.
func (p *processor) start(ctx context.Context, commitInterval time.Duration) {
	ticker := time.NewTicker(commitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.sender.Commit()
		case <-ctx.Done():
			p.sender.Commit()
			return
		}
	}
}

// processEvents handles a workloadmeta event bundle.
func (p *processor) processEvents(evBundle workloadmeta.EventBundle) {
	evBundle.Acknowledge()

	for _, event := range evBundle.Events {
		pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
		if !ok {
			continue
		}
		uid := pod.GetID().ID

		switch event.Type {
		case workloadmeta.EventTypeSet:
			p.handleSet(uid, pod)
		case workloadmeta.EventTypeUnset:
			p.handleUnset(uid)
		}
	}
}

func (p *processor) handleSet(uid string, pod *workloadmeta.KubernetesPod) {
	p.mu.Lock()
	defer p.mu.Unlock()

	rec, known := p.podStates[uid]

	if !known {
		if isReadyAndRunning(pod) {
			// Agent missed the initial transition; record as done with no metric.
			p.podStates[uid] = &podRecord{status: podStatusDone}
		} else {
			p.podStates[uid] = &podRecord{
				status:    podStatusPending,
				createdAt: pod.CreationTimestamp,
			}
		}
		return
	}

	if rec.status == podStatusDone {
		return
	}

	// rec.status == podStatusPending
	if !isReadyAndRunning(pod) {
		return
	}

	// Pod just became Ready && Running – emit startup durations exactly once.
	// Use LowCardinality so that pod_name (OrchestratorCardinality) is excluded.
	entityID := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, uid)
	tags, err := p.tagger.Tag(entityID, taggertypes.LowCardinality)
	if err != nil {
		log.Debugf("pod_lifecycle: cannot get tags for pod %s: %v", uid, err)
		tags = []string{}
	}

	if ttr, err := computeTimeToReady(pod, rec.createdAt); err != nil {
		log.Debugf("pod_lifecycle: cannot compute time_to_ready for pod %s: %v", uid, err)
	} else {
		p.sender.Distribution(metricTimeToReady, ttr, "", tags)
	}

	if ttrun, err := computeTimeToRunning(pod, rec.createdAt); err != nil {
		log.Debugf("pod_lifecycle: cannot compute time_to_running for pod %s: %v", uid, err)
	} else {
		p.sender.Distribution(metricTimeToRunning, ttrun, "", tags)
	}

	rec.status = podStatusDone
}

func (p *processor) handleUnset(uid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.podStates, uid)
}

// isReadyAndRunning returns true when the pod is in Running phase and the
// Ready condition is True.
func isReadyAndRunning(pod *workloadmeta.KubernetesPod) bool {
	if pod.Phase != "Running" {
		return false
	}
	for _, c := range pod.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return true
		}
	}
	return false
}

// computeTimeToReady returns the seconds from pod creation to when the Ready
// condition first transitioned to True, using LastTransitionTime.
func computeTimeToReady(pod *workloadmeta.KubernetesPod, createdAt time.Time) (float64, error) {
	for _, c := range pod.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			if c.LastTransitionTime.IsZero() {
				return 0, fmt.Errorf("LastTransitionTime not set for Ready condition")
			}
			d := c.LastTransitionTime.Sub(createdAt).Seconds()
			if d < 0 {
				return 0, fmt.Errorf("negative time_to_ready (%v)", d)
			}
			return d, nil
		}
	}
	return 0, fmt.Errorf("Ready condition not found")
}

// computeTimeToRunning returns the seconds from pod creation to when the first
// container started running (minimum StartedAt across ContainerStatuses).
func computeTimeToRunning(pod *workloadmeta.KubernetesPod, createdAt time.Time) (float64, error) {
	var earliest time.Time
	for _, cs := range pod.ContainerStatuses {
		if cs.State.Running != nil && !cs.State.Running.StartedAt.IsZero() {
			t := cs.State.Running.StartedAt
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
		}
	}
	if earliest.IsZero() {
		return 0, fmt.Errorf("no running container found")
	}
	d := earliest.Sub(createdAt).Seconds()
	if d < 0 {
		return 0, fmt.Errorf("negative time_to_running (%v)", d)
	}
	return d, nil
}
