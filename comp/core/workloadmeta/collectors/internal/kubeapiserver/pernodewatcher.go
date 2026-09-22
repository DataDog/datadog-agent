// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"sync"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

// perNodePodWatcher is responsible for watching pods on a per-node basis and
// reporting the sync state of the pods on each node.
type perNodePodWatcher struct {
	wmeta    workloadmeta.Component
	config   config.Reader
	client   kubernetes.Interface
	scope    workloadmeta.PodWatchScope
	reporter workloadmeta.NodeSyncReporter

	mu      sync.Mutex
	watches map[string]*nodeWatch
}

type nodeWatch struct {
	cancel context.CancelFunc
	store  *reflectorStore
}

func newPerNodePodWatcher(wmeta workloadmeta.Component, config config.Reader, client kubernetes.Interface, scope workloadmeta.PodWatchScope, reporter workloadmeta.NodeSyncReporter) *perNodePodWatcher {
	return &perNodePodWatcher{
		wmeta:    wmeta,
		config:   config,
		client:   client,
		scope:    scope,
		reporter: reporter,
		watches:  make(map[string]*nodeWatch),
	}
}

func (w *perNodePodWatcher) run(ctx context.Context) {
	unsubscribe := w.scope.Subscribe(func() { w.reconcile(ctx) })
	defer unsubscribe()
	w.reconcile(ctx)

	<-ctx.Done()
	w.stopAll()
}

// stopAll stops every node watch
func (w *perNodePodWatcher) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for node := range w.watches {
		w.stopLocked(node)
	}
}

// reconcile aligns the watch set with the scope's current nodes.
func (w *perNodePodWatcher) reconcile(ctx context.Context) {
	desired := make(map[string]struct{})
	for _, node := range w.scope.Nodes() {
		desired[node] = struct{}{}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for node := range w.watches {
		if _, ok := desired[node]; !ok {
			w.stopLocked(node)
		}
	}
	for node := range desired {
		if _, ok := w.watches[node]; !ok {
			w.startLocked(ctx, node)
		}
	}
}

func (w *perNodePodWatcher) startLocked(ctx context.Context, node string) {
	nodeCtx, cancel := context.WithCancel(ctx)

	store := newPodReflectorStoreWithFullPodParser(w.wmeta, w.config)
	reportingStore := &syncReportingStore{
		Store:    store,
		node:     node,
		reporter: w.reporter,
	}
	reflector := cache.NewNamedReflector(
		componentName,
		newNodePodListerWatcher(w.client, node),
		&corev1.Pod{},
		reportingStore,
		noResync,
	)

	w.watches[node] = &nodeWatch{cancel: cancel, store: store}
	go reflector.Run(nodeCtx.Done())
	log.Debugf("per-node pod reflector started for node %s", node)
}

func (w *perNodePodWatcher) stopLocked(node string) {
	watch, ok := w.watches[node]
	if !ok {
		return
	}
	delete(w.watches, node)

	watch.cancel()
	if err := watch.store.flushUnset(); err != nil {
		log.Errorf("failed to flush pod store for node %s: %v", node, err)
	}
	if w.reporter != nil {
		w.reporter.NodeSynced(node, false)
	}
	log.Debugf("per-node pod reflector stopped for node %s", node)
}

// syncReportingStore wraps a reflectorStore and reports the first completed
// list of one node's watch as its sync state.
type syncReportingStore struct {
	cache.Store

	node     string
	reporter workloadmeta.NodeSyncReporter
	reported atomic.Bool
}

func (s *syncReportingStore) Replace(list []interface{}, resourceVersion string) error {
	if err := s.Store.Replace(list, resourceVersion); err != nil {
		return err
	}
	if s.reporter != nil && s.reported.CompareAndSwap(false, true) {
		s.reporter.NodeSynced(s.node, true)
	}
	return nil
}

// newNodePodListerWatcher returns a pod ListWatch scoped to one node through
// the spec.nodeName field selector.
func newNodePodListerWatcher(client kubernetes.Interface, node string) *cache.ListWatch {
	selector := fields.OneTermEqualSelector("spec.nodeName", node).String()

	return &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = selector
			return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = selector
			return client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
		},
	}
}
