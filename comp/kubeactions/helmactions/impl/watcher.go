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
			w.handleJobEvent(ctx, ev)
		case ev, ok := <-podWatcher.ResultChan():
			if !ok {
				// stream closed by server; caller will reconnect
				return nil
			}
			w.handlePod(ctx, ev)
		}
	}
}

func (w *jobWatcher) OnRollback(in *helmactions.RollbackInputs, job *batchv1.Job) {
	w.store.TrackJob(job, in)
}
