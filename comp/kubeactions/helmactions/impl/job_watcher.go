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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// jobWatchSelector targets only the Jobs this component creates. Derived from
// the same label constants used by the rollback executor (rollback.go) so the
// watcher and the writer cannot drift out of sync.
const jobWatchSelector = labelManagedBy + "=" + managedByValue + "," + labelComponent + "=" + componentValue

// watchReconnectBackoff is the delay before reopening a closed Watch. The
// upstream watch can close legitimately (idle timeout, apiserver restart) and
// we want to reconnect promptly without hot-looping if the apiserver is down.
const watchReconnectBackoff = 5 * time.Second

// podLogTailLines is the number of trailing log lines fetched from a failed
// helm container. Bounded to keep the in-memory store reasonable on chatty
// failures — the goal is enough context to debug, not full audit history.
const podLogTailLines int64 = 200

// podLogMaxBytes caps the log payload regardless of line count, as a defence
// against single very long lines (helm can emit large diff dumps).
const podLogMaxBytes int64 = 64 * 1024

// podLogFetchTimeout bounds the time spent collecting logs for a single Pod.
const podLogFetchTimeout = 15 * time.Second

// jobWatcher reconciles tracked Job state by streaming Watch events from the
// apiserver. It runs as a single goroutine started in helmactions.start() and
// terminates when its context is cancelled.
type jobWatcher struct {
	client kubernetes.Interface
	store  *ActionStore
}

func newJobWatcher(client kubernetes.Interface, store *ActionStore) *jobWatcher {
	return &jobWatcher{client: client, store: store}
}

// run blocks until ctx is done, restarting the underlying Watch as needed.
func (w *jobWatcher) run(ctx context.Context, done chan struct{}) {
	log.Infof("[HelmActions] Job watcher started (selector=%q)", jobWatchSelector)
	defer func() {
		log.Infof("[HelmActions] Job watcher stopped")
		close(done)
	}()

	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.watchOnce(ctx); err != nil && ctx.Err() == nil {
			log.Warnf("[HelmActions] Job watch ended: %v — reconnecting in %s", err, watchReconnectBackoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(watchReconnectBackoff):
		}
	}
}

// watchOnce opens a two Watch streams and drains its events. Returns when the
// streams closes or the context is cancelled.
func (w *jobWatcher) watchOnce(ctx context.Context) error {
	jobWatcher, err := w.client.BatchV1().Jobs(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{
		LabelSelector: jobWatchSelector,
	})
	if err != nil {
		return err
	}
	defer jobWatcher.Stop()

	podWatcher, err := w.client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{
		LabelSelector: jobWatchSelector,
	})
	if err != nil {
		return err
	}
	defer podWatcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-jobWatcher.ResultChan():
			if !ok {
				// stream closed by server; caller will reconnect
				return nil
			}
			w.handleJob(ev)
		case ev, ok := <-podWatcher.ResultChan():
			if !ok {
				// stream closed by server; caller will reconnect
				return nil
			}
			w.handlePod(ctx, ev)
		}
	}
}

func (w *jobWatcher) handleJob(ev watch.Event) {
	job, ok := ev.Object.(*batchv1.Job)
	if !ok {
		// Error or Bookmark event — ignore.
		return
	}
	switch ev.Type {
	case watch.Added, watch.Modified:
		rec, terminal := w.store.UpdateJob(job)
		if terminal {
			log.Infof("[HelmActions] Job %s/%s reached terminal phase=%s (succeeded=%d failed=%d): %s",
				rec.Namespace, rec.Name, rec.Phase, rec.Succeeded, rec.Failed, rec.Message)
			// todo(dp): report job status
		}
	case watch.Deleted:
		w.store.RemoveJob(job.UID)
		log.Debugf("[HelmActions] Job %s/%s deleted, dropped from store", job.Namespace, job.Name)
		// todo(dp): report job removed
	}
}

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

func (h *jobWatcher) OnRollback(in *helmactions.RollbackInputs, job *batchv1.Job) {
	h.store.TrackJob(job, in)
}
