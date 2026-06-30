// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// jobWatchSelector targets only the Jobs this component creates.
const jobWatchSelector = "app.kubernetes.io/managed-by=datadog-cluster-agent,app.kubernetes.io/component=helm-rollback"

// watchReconnectBackoff is the delay before reopening a closed Watch. The
// upstream watch can close legitimately (idle timeout, apiserver restart) and
// we want to reconnect promptly without hot-looping if the apiserver is down.
const watchReconnectBackoff = 5 * time.Second

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
func (w *jobWatcher) run(ctx context.Context) {
	log.Infof("[HelmActions] Job watcher started (selector=%q)", jobWatchSelector)
	defer log.Infof("[HelmActions] Job watcher stopped")

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

// watchOnce opens a single Watch stream and drains its events. Returns when the
// stream closes or the context is cancelled.
func (w *jobWatcher) watchOnce(ctx context.Context) error {
	watcher, err := w.client.BatchV1().Jobs(metav1.NamespaceAll).Watch(ctx, metav1.ListOptions{
		LabelSelector: jobWatchSelector,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.ResultChan():
			if !ok {
				return nil // stream closed by server; caller will reconnect
			}
			w.handle(ev)
		}
	}
}

func (w *jobWatcher) handle(ev watch.Event) {
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
		}
	case watch.Deleted:
		w.store.RemoveJob(job.UID)
		log.Debugf("[HelmActions] Job %s/%s deleted, dropped from store", job.Namespace, job.Name)
	}
}
