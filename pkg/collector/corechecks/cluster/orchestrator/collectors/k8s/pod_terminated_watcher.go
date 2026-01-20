// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
)

const (
	backoffResetInterval = 15 * time.Second
)

// PodDeletionWatcher watches for pod deletion events using the Kubernetes watch API.
// It uses client-go's RetryWatcher for automatic retry handling and performs client-side
// filtering to process only deletion events. This ensures complete coverage of all pod
// deletions regardless of phase, including force-deleted pods.
type PodDeletionWatcher struct {
	backoff      wait.Backoff
	client       kubernetes.Interface
	clock        clock.Clock
	eventHandler func(*v1.Pod)
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewPodDeletionWatcher creates a new pod deletion event watcher that will run until the provided stop channel is closed.
// The watcher will use exponential backoff to retry 410 Gone errors (resource version expired).
func NewPodDeletionWatcher(client kubernetes.Interface, eventHandler func(*v1.Pod), stopCh chan struct{}) *PodDeletionWatcher {
	backoff := wait.Backoff{
		Duration: 1 * time.Second,  // initial backoff
		Factor:   2.0,              // backoff factor
		Jitter:   0.0,              // jitter (no jitter)
		Steps:    100,              // max retry attempts
		Cap:      30 * time.Second, // max backoff
	}
	watcher := &PodDeletionWatcher{
		client:       client,
		clock:        clock.RealClock{},
		eventHandler: eventHandler,
		stopCh:       stopCh,
		backoff:      backoff,
	}

	watcher.start()
	return watcher
}

// getInitialResourceVersion gets the current resource version using a cheap List operation.
// Using limit=1 ensures minimal data transfer while getting the current resource version.
func (w *PodDeletionWatcher) getInitialResourceVersion(ctx context.Context) (string, error) {
	listOpts := metav1.ListOptions{
		Limit: 1,
	}

	podList, err := w.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, listOpts)
	if err != nil {
		return "", fmt.Errorf("failed to get initial resource version: %w", err)
	}

	return podList.ResourceVersion, nil
}

// watchLoop uses exponential backoff to retry the list+watch operation on errors.
func (w *PodDeletionWatcher) watchLoop(ctx context.Context) error {
	delayFunc := w.backoff.DelayWithReset(w.clock, backoffResetInterval)
	return delayFunc.Until(ctx, true, true, func(context.Context) (done bool, err error) {
		rv, err := w.getInitialResourceVersion(ctx)
		if err != nil {
			log.Errorf("Failed to get resource version: %v", err)
			return false, nil
		}

		err = w.runWatch(ctx, rv)
		// Special handling for 410 Gone errors, we want to retry them.
		if err != nil && errors.IsResourceExpired(err) {
			log.Debugf("Watch returned resource version expired error: %w", err)
			return false, nil
		}

		return true, err
	})
}

// runWatch is used to watch events until the context is cancelled or an error not retried by the underlying
// RetryWatcher occurs. In the later case the [errors.StatusError] object is returned.
func (w *PodDeletionWatcher) runWatch(ctx context.Context, resourceVersion string) error {
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.ResourceVersion = resourceVersion
		return w.client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
	}

	retryWatcher, err := toolswatch.NewRetryWatcherWithContext(ctx, resourceVersion, &cache.ListWatch{
		WatchFunc: watchFunc,
	})
	if err != nil {
		return fmt.Errorf("failed to create retry watcher: %w", err)
	}

	defer retryWatcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-retryWatcher.ResultChan():
			if !ok {
				return nil
			}

			switch event.Type {
			case watch.Deleted:
				pod, ok := event.Object.(*v1.Pod)
				if !ok {
					log.Warnf("Delete event with non-pod object: %T", event.Object)
					continue
				}

				w.eventHandler(pod)

			// Decide what to do on errors not retried by RetryWatcher
			case watch.Error:
				status, ok := event.Object.(*metav1.Status)
				if !ok {
					continue
				}

				return errors.FromObject(status)

			// Ignore non-deletion events
			case watch.Added, watch.Modified, watch.Bookmark:

			default:
				log.Debugf("Unexpected watch event type: %v", event.Type)
			}
		}
	}
}

// start is used to run the pod deletion event watch loop. It creates two goroutines:
//   - One that watches for stop signal and cancels the watch context
//   - One that runs the watch loop which handles the watch lifecycle
func (w *PodDeletionWatcher) start() {
	// Create cancellable context for the watch loop
	watchCtx, cancel := context.WithCancel(context.Background())

	// Goroutine 1: Watch for stop channel closure and cancel context
	w.wg.Go(func() {
		select {
		case <-w.stopCh:
			cancel()
		case <-watchCtx.Done():
			cancel()
		}
	})
	// Goroutine 2: Run the watch loop (blocking)
	w.wg.Go(func() {
		if err := w.watchLoop(watchCtx); err != nil && err != context.Canceled {
			log.Errorf("Watch loop exited with error: %v", err)
		}
	})
}
