// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func newNodeStore(ctx context.Context, wlm workloadmeta.Component, _ config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	nodeListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Nodes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Nodes().Watch(ctx, options)
		},
	}

	nodeStore := &reflectorStore{
		wlmetaStore: wlm,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      kubernetesresourceparsers.NewNodeParser(),
	}
	nodeReflector := cache.NewNamedReflector(
		componentName,
		nodeListerWatcher,
		&corev1.Node{},
		nodeStore,
		noResync,
	)
	return nodeReflector, nodeStore
}
