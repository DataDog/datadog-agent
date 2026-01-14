// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PodDeletionWatcher watches for pod deletion events using a RetryWatcher.
// Unlike informers, it does not maintain a local cache, making it memory-efficient
// for watching pod deletion among all events at the expense of increased network traffic.
//
// Uses k8s.io/client-go/tools/watch.RetryWatcher which handles:
// - Automatic reconnection with backoff
// - ResourceVersion tracking
//
// Note: RetryWatcher does NOT handle 410 Gone errors. When the watch fails,
// PodDeletionWatcher automatically retries by fetching a new resourceVersion
// via List and restarting the watch.
type PodDeletionWatcher struct {
	clientset kubernetes.Interface
	clock     clock.Clock
	handler   func(pod *corev1.Pod)
	mu        sync.Mutex
	processed int64
	running   bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// PodDeletedHandler is the handler called for each pod deletion event processed by [PodDeletionWatcher].
type PodDeletedHandler func(pod *corev1.Pod)

// NewPodDeletionWatcher creates a new PodDeletionWatcher.
func NewPodDeletionWatcher(clientset kubernetes.Interface, handler PodDeletedHandler) *PodDeletionWatcher {
	return &PodDeletionWatcher{
		clientset: clientset,
		clock:     clock.RealClock{},
		handler:   handler,
	}
}

// Start begins watching for pod deletions in a background goroutine.
// Non-blocking - returns immediately. Safe to call multiple times (idempotent).
func (w *PodDeletionWatcher) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return
	}

	w.stopCh = make(chan struct{})
	w.running = true

	w.run()
}

// Stop gracefully stops the watcher and waits for it to finish.
// Safe to call multiple times. After Stop(), Start() can be called again.
func (w *PodDeletionWatcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}

	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	w.wg.Wait()
}

func (w *PodDeletionWatcher) run() {
	ctx, cancel := context.WithCancel(context.Background())
	backoffManager := wait.NewExponentialBackoffManager(800*time.Millisecond, 30*time.Second, 2*time.Minute, 2.0, 1.0, w.clock)

	w.wg.Go(func() {
		<-w.stopCh
		cancel()
	})

	w.wg.Go(func() {
		defer cancel()

		watchFunc := func() {
			// Terminate if non retryable situation (e.g. regular shutdown, unexpected error)
			if retry := w.watch(ctx); !retry {
				cancel()
				return
			}
		}

		wait.BackoffUntil(watchFunc, backoffManager, true, ctx.Done())
	})
}

func (w *PodDeletionWatcher) watch(ctx context.Context) (retry bool) {
	// Get a valid resourceVersion by doing a List call limited to a single item.
	podList, err := w.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		log.Errorf("Failed to get resourceVersion for pod deletion watcher: %v", err)
		return
	}

	watcherClient := &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return w.clientset.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
		},
	}
	retryWatcher, err := toolswatch.NewRetryWatcherWithContext(ctx, podList.ResourceVersion, watcherClient)
	if err != nil {
		log.Errorf("Failed to create pod deletion watcher: %v", err)
		return
	}

	defer retryWatcher.Stop()

	return w.processEvents(ctx, retryWatcher.ResultChan())
}

func (w *PodDeletionWatcher) processEvents(ctx context.Context, eventCh <-chan watch.Event) (retry bool) {
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-eventCh:
			if !ok {
				return
			}

			if event.Type == watch.Error {
				errObject := apierrors.FromObject(event.Object)
				return apierrors.IsGone(errObject)
			}

			if event.Type != watch.Deleted {
				continue
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			w.handler(pod)
			atomic.AddInt64(&w.processed, 1)
		}
	}
}

// Processed returns the number of pod deletions processed.
func (w *PodDeletionWatcher) Processed() int64 {
	return atomic.LoadInt64(&w.processed)
}
