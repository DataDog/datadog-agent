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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newPodStore(ctx context.Context, wlm workloadmeta.Component, config config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	podListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
		},
	}

	podStore := newPodReflectorStore(wlm, config)
	podReflector := cache.NewNamedReflector(
		componentName,
		podListerWatcher,
		&corev1.Pod{},
		podStore,
		noResync,
	)
	log.Debug("pod reflector enabled")
	return podReflector, podStore
}

func newPodReflectorStore(wlmetaStore workloadmeta.Component, config config.Reader) *reflectorStore {
	annotationsExclude := config.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude")
	parser, err := kubernetesresourceparsers.NewPodParser(annotationsExclude)
	if err != nil {
		_ = log.Errorf("unable to parse all pod_annotations_exclude: %v, err:", err)
		parser, _ = kubernetesresourceparsers.NewPodParser(nil)
	}

	return &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}
}
