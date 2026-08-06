// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func kueueQueueGVRStrings() []string {
	return []string{
		kubernetes.KueueGroupName + "/" + kubernetes.KueueLocalQueueResourceName,
		kubernetes.KueueGroupName + "/" + kubernetes.KueueClusterQueueResourceName,
	}
}

func kueueResourceFlavorGVRStrings() []string {
	return []string{
		kubernetes.KueueGroupName + "/" + kubernetes.KueueResourceFlavorResourceName,
	}
}

func kueueWorkloadGVRStrings() []string {
	return []string{
		kubernetes.KueueGroupName + "/" + kubernetes.KueueWorkloadResourceName,
	}
}

func shouldHaveKueueMetadata(cfg config.Reader) bool {
	return cfg.GetBool("cluster_agent.kueue.enabled")
}

func newKueueQueueStore(ctx context.Context, wlmetaStore workloadmeta.Component, client dynamic.Interface, gvr schema.GroupVersionResource, queueType workloadmeta.KueueQueueType) (*cache.Reflector, *reflectorStore, error) {
	listerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing Kueue %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching Kueue %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	parser, err := kubernetesresourceparsers.NewKueueQueueParser(queueType)
	if err != nil {
		return nil, nil, fmt.Errorf("creating Kueue %s parser: %w", gvr.Resource, err)
	}

	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
		filter:      nil,
	}
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store, nil
}

func newKueueResourceFlavorStore(ctx context.Context, wlmetaStore workloadmeta.Component, client dynamic.Interface, gvr schema.GroupVersionResource) (*cache.Reflector, *reflectorStore, error) {
	listerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing Kueue %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching Kueue %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      kubernetesresourceparsers.NewKueueResourceFlavorParser(),
		filter:      nil,
	}
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store, nil
}

func newKueueWorkloadStore(ctx context.Context, wlmetaStore workloadmeta.Component, client dynamic.Interface, gvr schema.GroupVersionResource) (*cache.Reflector, *reflectorStore, error) {
	listerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing Kueue %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching Kueue %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      kubernetesresourceparsers.NewKueueWorkloadParser(),
		filter:      nil,
	}
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store, nil
}
