// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// workloadConfigStore provides spot configuration for workloads.
type workloadConfigStore interface {
	// run starts the store's background update and blocks until ctx is cancelled.
	run(ctx context.Context)
	// waitSynced blocks until the store has completed its initial sync.
	waitSynced()
	// getConfig returns the workloadSpotConfig for the workload if present.
	getConfig(key workload) (workloadSpotConfig, bool)
	// disable disables spot scheduling for workload.
	// If already disabled returns existing timestamp and false,
	// otherwise sets disabledUntil and returns the new timestamp and true.
	disable(key workload, now time.Time, until time.Time) (time.Time, bool)
}

type workloadResource struct {
	gvr  schema.GroupVersionResource
	kind string
}

var spotWorkloadResources = []workloadResource{
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, kubernetes.DeploymentKind},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, kubernetes.StatefulSetKind},
}

// kubeWorkloadConfigStore is a workloadConfigStore backed by Kubernetes informers.
// It watches Deployments and StatefulSets labeled with SpotEnabledLabelSelector and
// maintains a map of objectKey → workloadSpotConfig updated on informer events.
type kubeWorkloadConfigStore struct {
	defaultConfig workloadSpotConfig
	podFetcher    *podFetcher

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	hasSynced       []cache.InformerSynced
	synced          chan struct{}

	mu      sync.RWMutex
	configs map[workload]workloadSpotConfig
}

func newKubeWorkloadConfigStore(dynamicClient dynamic.Interface, defaultConfig Config, pf *podFetcher) *kubeWorkloadConfigStore {
	s := &kubeWorkloadConfigStore{
		defaultConfig: workloadSpotConfig{percentage: defaultConfig.Percentage, minOnDemand: defaultConfig.MinOnDemandReplicas},
		podFetcher:    pf,
		configs:       make(map[workload]workloadSpotConfig),
		synced:        make(chan struct{}),
	}

	s.informerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		0, // no resync
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = SpotEnabledLabelKey + "=" + SpotEnabledLabelValue
		},
	)

	for _, r := range spotWorkloadResources {
		kind := r.kind
		inf := s.informerFactory.ForResource(r.gvr)
		if _, err := inf.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				s.onUpdated(kind, obj)
			},
			UpdateFunc: func(_, obj any) {
				s.onUpdated(kind, obj)
			},
			DeleteFunc: func(obj any) {
				s.onDeleted(kind, obj)
			},
		}); err != nil {
			log.Errorf("Failed to add event handler for %s: %v", r.gvr.Resource, err)
		}
		s.hasSynced = append(s.hasSynced, inf.Informer().HasSynced)
	}

	return s
}

func (s *kubeWorkloadConfigStore) run(ctx context.Context) {
	s.informerFactory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), s.hasSynced...) {
		log.Error("Failed to sync informer caches")
		close(s.synced)
		return
	}
	log.Info("Spot workload config store synced")
	close(s.synced)
	<-ctx.Done()
}

func (s *kubeWorkloadConfigStore) waitSynced() {
	<-s.synced
}

func (s *kubeWorkloadConfigStore) getConfig(key workload) (workloadSpotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[key]
	return cfg, ok
}

func (s *kubeWorkloadConfigStore) disable(key workload, now time.Time, until time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.configs[key]
	if !ok {
		return time.Time{}, false
	}
	if now.Before(cfg.disabledUntil) {
		return cfg.disabledUntil, false
	}
	cfg.disabledUntil = until
	s.configs[key] = cfg
	return until, true
}

func (s *kubeWorkloadConfigStore) onUpdated(kind string, obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	// Guard against watch events delivered without label selector filtering for dynamicfake client,
	// see https://github.com/kubernetes/kubernetes/issues/106754
	// With a real API server the informer factory label selector ensures only spot-enabled workloads reach this handler.
	if u.GetLabels()[SpotEnabledLabelKey] != SpotEnabledLabelValue {
		return
	}
	key := workload{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}
	cfg := s.defaultConfig
	overrideFromAnnotations(&cfg, u.GetAnnotations())

	s.mu.Lock()
	_, existed := s.configs[key]
	s.configs[key] = cfg
	s.mu.Unlock()

	log.Debugf("Spot workload config updated %s: %#v", key, cfg)

	// Enqueue a pod backfill when a workload opts in for the first time.
	// Pods created before the workload config key appeared were not delivered by WLM
	// (the filter rejected them); backfilling ensures the rebalancer can act on them.
	if !existed && s.podFetcher != nil {
		matchLabels, _, _ := unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
		if len(matchLabels) > 0 {
			s.podFetcher.enqueue(key, labels.SelectorFromSet(labels.Set(matchLabels)))
		}
	}
}

func (s *kubeWorkloadConfigStore) onDeleted(kind string, obj any) {
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = d.Obj
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	key := workload{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}

	s.mu.Lock()
	delete(s.configs, key)
	s.mu.Unlock()

	log.Debugf("Spot workload config deleted %s", key)
}
