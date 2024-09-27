// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver contains the collector that collects data metadata from the API server.
package kubeapiserver

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
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

// deploymentFilter filters out deployments that can't be used for unified service tagging or process language detection
type deploymentFilter struct{}

func (f *deploymentFilter) filteredOut(entity workloadmeta.Entity) bool {
	deployment := entity.(*workloadmeta.KubernetesDeployment)
	return deployment == nil
}

func newDeploymentStore(ctx context.Context, wlm workloadmeta.Component, cfg config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	deploymentListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.AppsV1().Deployments(metav1.NamespaceAll).Watch(ctx, options)
		},
	}

	deploymentStore := newDeploymentReflectorStore(wlm, cfg)
	deploymentReflector := cache.NewNamedReflector(
		componentName,
		deploymentListerWatcher,
		&appsv1.Deployment{},
		deploymentStore,
		noResync,
	)
	return deploymentReflector, deploymentStore
}

func newDeploymentReflectorStore(wlmetaStore workloadmeta.Component, cfg config.Reader) *reflectorStore {
	annotationsExclude := cfg.GetStringSlice("cluster_agent.kubernetes_resources_collection.deployment_annotations_exclude")
	parser, err := kubernetesresourceparsers.NewDeploymentParser(annotationsExclude)
	if err != nil {
		_ = log.Errorf("unable to parse all deployment_annotations_exclude: %v, err:", err)
		parser, _ = kubernetesresourceparsers.NewDeploymentParser(nil)
	}

	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
		filter:      &deploymentFilter{},
	}

	return store
}
