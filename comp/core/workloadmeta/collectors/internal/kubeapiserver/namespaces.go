// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver contains the collector that collects data metadata from the API server.
package kubeapiserver

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newNamespaceStore(ctx context.Context, wlm workloadmeta.Component, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	namespaceListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Namespaces().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Namespaces().Watch(ctx, options)
		},
	}

	namespaceStore := newNamespaceReflectorStore(wlm)
	namespaceReflector := cache.NewNamedReflector(
		componentName,
		namespaceListerWatcher,
		&corev1.Namespace{},
		namespaceStore,
		noResync,
	)
	log.Debug("namespace reflector enabled")
	return namespaceReflector, namespaceStore
}

func newNamespaceReflectorStore(wlmetaStore workloadmeta.Component) *reflectorStore {
	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      newNamespaceParser(),
	}

	return store
}

type namespaceParser struct{}

func newNamespaceParser() objectParser {
	return namespaceParser{}
}

func (p namespaceParser) Parse(obj interface{}) workloadmeta.Entity {
	namespace := obj.(*corev1.Namespace)

	wlmNamespace := &workloadmeta.KubernetesNamespace{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNamespace,
			ID:   namespace.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        namespace.Name,
			Annotations: namespace.Annotations,
			Labels:      namespace.Labels,
		},
	}
	return wlmNamespace
}
