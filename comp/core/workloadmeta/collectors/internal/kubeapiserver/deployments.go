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
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	languagedetectionUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"

	ddkube "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// deploymentFilter filters out deployments that can't be used for unified service tagging or process language detection
type deploymentFilter struct{}

func (f *deploymentFilter) filteredOut(entity workloadmeta.Entity) bool {
	deployment := entity.(*workloadmeta.KubernetesDeployment)
	return deployment == nil
}

func newDeploymentStore(ctx context.Context, wlm workloadmeta.Component, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	deploymentListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.AppsV1().Deployments(metav1.NamespaceAll).Watch(ctx, options)
		},
	}

	deploymentStore := newDeploymentReflectorStore(wlm)
	deploymentReflector := cache.NewNamedReflector(
		componentName,
		deploymentListerWatcher,
		&appsv1.Deployment{},
		deploymentStore,
		noResync,
	)
	return deploymentReflector, deploymentStore
}

func newDeploymentReflectorStore(wlmetaStore workloadmeta.Component) *reflectorStore {
	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      newdeploymentParser(),
		filter:      &deploymentFilter{},
	}

	return store
}

type deploymentParser struct{}

func newdeploymentParser() objectParser {
	return deploymentParser{}
}

func updateContainerLanguage(cl languagedetectionUtil.ContainersLanguages, container languagedetectionUtil.Container, languages string) {
	if _, found := cl[container]; !found {
		cl[container] = make(languagedetectionUtil.LanguageSet)
	}

	for _, lang := range strings.Split(languages, ",") {
		cl[container][languagedetectionUtil.Language(strings.TrimSpace(lang))] = struct{}{}
	}
}

func (p deploymentParser) Parse(obj interface{}) workloadmeta.Entity {
	deployment := obj.(*appsv1.Deployment)
	containerLanguages := make(languagedetectionUtil.ContainersLanguages)

	for annotation, languages := range deployment.Annotations {

		containerName, isInitContainer := languagedetectionUtil.ExtractContainerFromAnnotationKey(annotation)
		if containerName != "" && languages != "" {

			updateContainerLanguage(
				containerLanguages,
				languagedetectionUtil.Container{
					Name: containerName,
					Init: isInitContainer,
				},
				languages)
		}
	}

	return &workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   deployment.Namespace + "/" + deployment.Name, // we use the namespace/name as id to make it easier for the admission controller to retrieve the corresponding deployment
		},
		Env:                 deployment.Labels[ddkube.EnvTagLabelKey],
		Service:             deployment.Labels[ddkube.ServiceTagLabelKey],
		Version:             deployment.Labels[ddkube.VersionTagLabelKey],
		InjectableLanguages: containerLanguages,
	}
}
