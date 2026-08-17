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

func resourceClaimGVRStrings() []string {
	return []string{
		kubernetes.DRAGroupName + "/" + kubernetes.DRAResourceClaimResourceName,
	}
}

// draCollectionEnabled reports whether DRA objects must be collected.
func draCollectionEnabled(cfg config.Reader) bool {
	return cfg.GetBool("cluster_agent.dra.enabled")
}

func newResourceClaimStore(ctx context.Context, wlmetaStore workloadmeta.Component, client dynamic.Interface, gvr schema.GroupVersionResource) (*cache.Reflector, *reflectorStore) {
	listerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing DRA %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching DRA %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      kubernetesresourceparsers.NewResourceClaimParser(),
		filter:      nil,
	}
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store
}
