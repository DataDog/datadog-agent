// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	// getConfig returns the spotConfig for the workload if present.
	getConfig(key workload) (spotConfig, bool)
}

var spotWorkloadResources = []struct {
	gvr  schema.GroupVersionResource
	kind string
}{
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, kubernetes.DeploymentKind},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, kubernetes.StatefulSetKind},
}

// kubeWorkloadConfigStore is a workloadConfigStore backed by Kubernetes informers.
// It watches Deployments and StatefulSets labeled with SpotEnabledLabelSelector and
// maintains a map of objectKey → spotConfig updated on informer events.
type kubeWorkloadConfigStore struct {
	defaultConfig spotConfig

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	hasSynced       []cache.InformerSynced

	mu      sync.RWMutex
	configs map[workload]spotConfig
}

func newKubeWorkloadConfigStore(dynamicClient dynamic.Interface, defaultConfig Config) *kubeWorkloadConfigStore {
	s := &kubeWorkloadConfigStore{
		defaultConfig: spotConfig{percentage: defaultConfig.Percentage, minOnDemand: defaultConfig.MinOnDemandReplicas},
		configs:       make(map[workload]spotConfig),
	}

	s.informerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		0, // no resync
		metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = SpotEnabledLabelSelector
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
		return
	}
	log.Info("Spot workload config store synced")
	<-ctx.Done()
}

func (s *kubeWorkloadConfigStore) getConfig(key workload) (spotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[key]
	return cfg, ok
}

func (s *kubeWorkloadConfigStore) onUpdated(kind string, obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	key := workload{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}
	cfg := s.defaultConfig
	overrideFromAnnotations(&cfg, u.GetAnnotations())

	s.mu.Lock()
	s.configs[key] = cfg
	s.mu.Unlock()

	log.Debugf("Spot workload config updated %s: %s", key, cfg)
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
