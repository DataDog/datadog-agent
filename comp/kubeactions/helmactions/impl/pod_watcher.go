// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// podLogTailLines is the number of trailing log lines fetched from a failed
// helm container. Bounded to keep the in-memory store reasonable on chatty
// failures — the goal is enough context to debug, not full audit history.
const podLogTailLines int64 = 200

// podLogMaxBytes caps the log payload regardless of line count, as a defence
// against single very long lines (helm can emit large diff dumps).
const podLogMaxBytes int64 = 64 * 1024

// podLogFetchTimeout bounds the time spent collecting logs for a single Pod.
const podLogFetchTimeout = 15 * time.Second

func (w *jobWatcher) handlePod(ctx context.Context, ev watch.Event) {
	pod, ok := ev.Object.(*corev1.Pod)
	if !ok {
		return
	}
	switch ev.Type {
	case watch.Added, watch.Modified:
		rec, justFailed := w.store.UpdatePod(pod)
		if justFailed {
			log.Warnf("[HelmActions] Pod %s/%s failed (job=%s reason=%q exit=%d): %s",
				rec.Namespace, rec.Name, rec.JobName, rec.Reason, rec.ExitCode, rec.Message)
			// Fetch logs in a separate goroutine so the watch loop keeps
			// draining events. The fetch is bounded by its own timeout.
			go w.captureLogs(ctx, rec)
		}
	case watch.Deleted:
		w.store.RemovePod(pod.UID)
		log.Debugf("[HelmActions] Pod %s/%s deleted, dropped from store", pod.Namespace, pod.Name)
	}
}

// captureLogs reads the tail of the helm container's logs and attaches them to
// the PodRecord. Best-effort: on any error we log and move on — the failure
// itself is already recorded by UpdatePod.
func (w *jobWatcher) captureLogs(parent context.Context, rec PodRecord) {
	ctx, cancel := context.WithTimeout(parent, podLogFetchTimeout)
	defer cancel()

	tail := podLogTailLines
	req := w.client.CoreV1().Pods(rec.Namespace).GetLogs(rec.Name, &corev1.PodLogOptions{
		Container: helmContainerName,
		TailLines: &tail,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		log.Warnf("[HelmActions] Failed to open log stream for pod %s/%s: %v", rec.Namespace, rec.Name, err)
		return
	}
	defer stream.Close()

	buf, err := io.ReadAll(io.LimitReader(stream, podLogMaxBytes))
	if err != nil {
		log.Warnf("[HelmActions] Failed to read logs for pod %s/%s: %v", rec.Namespace, rec.Name, err)
		return
	}
	w.store.AttachPodLogs(rec.UID, string(buf))
	log.Infof("[HelmActions] Captured %d bytes of logs from failed pod %s/%s", len(buf), rec.Namespace, rec.Name)
}
