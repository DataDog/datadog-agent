// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "kubeapiserver"
	componentName = "workloadmeta-kubeapiserver"
	noResync      = time.Duration(0)
)

type collector struct{}

func init() {
	workloadmeta.RegisterClusterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, wlmetaStore workloadmeta.Store) error {
	apiserverClient, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}

	client := apiserverClient.Cl

	nodeListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Nodes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Nodes().Watch(ctx, options)
		},
	}

	nodeReflector := cache.NewNamedReflector(
		componentName,
		nodeListerWatcher,
		&corev1.Node{},
		newNodeReflectorStore(wlmetaStore),
		noResync,
	)

	go nodeReflector.Run(ctx.Done())

	if config.Datadog.GetBool("cluster_agent.collect_kubernetes_tags") {
		podListerWatcher := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
			},
		}

		podReflector := cache.NewNamedReflector(
			componentName,
			podListerWatcher,
			&corev1.Pod{},
			newPodReflectorStore(wlmetaStore),
			noResync,
		)

		go podReflector.Run(ctx.Done())
	}

	return nil
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}
