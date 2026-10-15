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
)

func newRayServiceStore(wlmetaStore workloadmeta.Component, cfg config.Reader, client dynamic.Interface, gvr schema.GroupVersionResource) (*cache.Reflector, *reflectorStore, error) {
	listerWatcher := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing RayService %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching RayService %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	parser, err := kubernetesresourceparsers.NewRayServiceParser(gvr, cfg.GetStringSlice("cluster_agent.kube_metadata_collection.resource_annotations_exclude")...)
	if err != nil {
		return nil, nil, fmt.Errorf("creating RayService parser: %w", err)
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
