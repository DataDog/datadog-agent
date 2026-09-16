// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package helmactionsimpl implements the helmactions component interface.
package helmactionsimpl

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// failureInfo summarizes why a Job's most recent attempt failed.
type failureInfo struct {
	Source        string // "pod" or "job-event" (no pod was ever created)
	PodName       string // empty if Source == "job-event"
	ContainerName string // empty if Source == "job-event"
	Reason        string // e.g. "OOMKilled", "Error", "FailedCreate"
	Message       string
	ExitCode      int32 // 0 if not applicable
	FailedAt      time.Time
	Logs          string // empty if Source == "job-event" or logs couldn't be fetched
}

// diagnoseJobFailure returns the best available failure info for a Job:
// last failed pod's container error + logs, or the Job-level FailedCreate
// event if no pod was ever created.
func (w *jobWatcher) diagnoseJobFailure(ctx context.Context, job *batchv1.Job) (*failureInfo, error) {
	pod, containerName, reason, message, exitCode, failedAt, previous, err := w.latestFailedContainer(ctx, job)
	if err != nil {
		return nil, err
	}

	if pod == nil {
		// No pod ever ran. Fall back to the Job-level event.
		return w.maybeFailedCreateEvent(ctx, job)
	}

	logs, logErr := w.fetchPodLogs(ctx, pod.Namespace, pod.Name, containerName, previous)
	if logErr != nil {
		// Still return what we know even if logs are unavailable
		// (e.g. pod's logs already garbage-collected).
		logs = fmt.Sprintf("<failed to fetch logs: %v>", logErr)
	}

	return &failureInfo{
		Source:        "pod",
		PodName:       pod.Name,
		ContainerName: containerName,
		Reason:        reason,
		Message:       message,
		ExitCode:      exitCode,
		FailedAt:      failedAt,
		Logs:          logs,
	}, nil
}

// latestFailedContainer finds, across all Pods belonging to the Job, the
// most recently failed container — whether that container is in a Pod
// that's now fully Failed, or in a still-Running Pod that restarted after
// a failure (the restartPolicy=OnFailure case). Returns previous=true if
// the failed run is not the container's current run (so logs must be
// fetched with the "previous" flag).
func (w *jobWatcher) latestFailedContainer(
	ctx context.Context, job *batchv1.Job,
) (pod *corev1.Pod, containerName, reason, message string, exitCode int32, failedAt time.Time, previous bool, err error) {
	pods, listErr := w.client.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
	})
	if listErr != nil {
		return nil, "", "", "", 0, time.Time{}, false, listErr
	}

	type candidate struct {
		pod       *corev1.Pod
		container string
		term      *corev1.ContainerStateTerminated
		isCurrent bool // true if this is the container's *current* terminated state, not LastTerminationState
	}
	var candidates []candidate

	for i := range pods.Items {
		p := &pods.Items[i]

		// Case 1: container currently terminated (covers Pod phase == Failed,
		// and also individual failed containers in a Pod that hasn't fully
		// resolved yet).
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
				candidates = append(candidates, candidate{pod: p, container: cs.Name, term: cs.State.Terminated, isCurrent: true})
			}
		}
		// Case 2: restartPolicy=OnFailure — container previously failed and
		// was restarted, so the failure only shows up in LastTerminationState
		// while the Pod itself is still Running.
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Terminated == nil && cs.LastTerminationState.Terminated != nil {
				candidates = append(candidates, candidate{pod: p, container: cs.Name, term: cs.LastTerminationState.Terminated, isCurrent: false})
			}
		}
	}

	if len(candidates) == 0 {
		return nil, "", "", "", 0, time.Time{}, false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].term.FinishedAt.After(candidates[j].term.FinishedAt.Time)
	})
	best := candidates[0]

	return best.pod, best.container, best.term.Reason, best.term.Message, best.term.ExitCode,
		best.term.FinishedAt.Time, !best.isCurrent, nil
}

// fetchPodLogs pulls logs for a single container. previous=true fetches
// the log of the prior terminated run (needed when the container has since
// restarted, e.g. under restartPolicy=OnFailure).
func (w *jobWatcher) fetchPodLogs(ctx context.Context, namespace, podName, containerName string, previous bool) (string, error) {
	req := w.client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		Previous:  previous,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// maybeFailedCreateEvent handles the "no pod was ever created" case (e.g.
// missing ServiceAccount) by reading the Job's own FailedCreate event.
func (w *jobWatcher) maybeFailedCreateEvent(ctx context.Context, job *batchv1.Job) (*failureInfo, error) {
	fieldSelector := fmt.Sprintf(
		"involvedObject.kind=Job,involvedObject.name=%s,involvedObject.namespace=%s,reason=FailedCreate",
		job.Name, job.Namespace,
	)
	events, err := w.client.CoreV1().Events(job.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}
	if len(events.Items) == 0 {
		return nil, nil // truly nothing to report
	}

	latest := events.Items[0]
	for _, e := range events.Items {
		if e.LastTimestamp.After(latest.LastTimestamp.Time) {
			latest = e
		}
	}

	return &failureInfo{
		Source:   "job-event",
		Reason:   latest.Reason, // "FailedCreate"
		Message:  latest.Message,
		FailedAt: latest.LastTimestamp.Time,
	}, nil
}
