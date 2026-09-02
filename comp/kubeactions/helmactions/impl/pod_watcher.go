// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (w *jobWatcher) handlePod(_ context.Context, ev watch.Event) {
	pod, ok := ev.Object.(*corev1.Pod)
	if !ok {
		return
	}
	switch ev.Type {
	case watch.Added, watch.Modified:
		rec, justFailed := w.store.UpdatePod(pod)

		// TODO: report POD failure to EVP
		if justFailed {
			log.Warnf("[HelmActions] Pod %s/%s failed (job=%s reason=%q exit=%d): %s",
				rec.Namespace, rec.Name, rec.JobName, rec.Reason, rec.ExitCode, rec.Message)
			captureLogs(w.store, pod, rec)
		}
	case watch.Deleted:
		w.store.RemovePod(pod.UID)
		log.Debugf("[HelmActions] Pod %s/%s deleted, dropped from store", pod.Namespace, pod.Name)
	}
}

// captureLogs attaches the tail of the helm container's output to the
// PodRecord, sourced from the container's termination message rather than the
// "pods/log" API (which the cluster agent's service account isn't granted).
// buildRollbackJob sets TerminationMessagePolicy: FallbackToLogsOnError on the
// helm container, so on non-zero exit the kubelet copies the last ~4KB of its
// stdout/stderr into ContainerStatus.State.Terminated.Message, which arrives
// here as part of the Pod object the watch already delivers — no extra API
// call, and no extra RBAC, needed.
func captureLogs(store *ActionStore, pod *corev1.Pod, rec *PodRecord) {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name != helmContainerName {
			continue
		}
		t := cs.State.Terminated
		if t == nil || t.Message == "" {
			log.Debugf("[HelmActions] No termination message captured for failed pod %s/%s", pod.Namespace, pod.Name)
			return
		}
		store.AttachPodLogs(rec.UID, t.Message)
		log.Infof("[HelmActions] Captured %d bytes of logs from failed pod %s/%s (via terminationMessage)",
			len(t.Message), pod.Namespace, pod.Name)
		return
	}
}
