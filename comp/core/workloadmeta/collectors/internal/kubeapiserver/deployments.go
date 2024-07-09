// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver contains the collector that collects data metadata from the API server.
package kubeapiserver

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	languagedetectionUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	ddkube "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func newDeploymentStore(ctx context.Context, wlm workloadmeta.Component, _ config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
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
		seen:        make(map[string][]workloadmeta.EntityID),
		parser:      newdeploymentParser(),
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

func (p deploymentParser) Parse(obj interface{}) []workloadmeta.Entity {
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

	entities := make([]workloadmeta.Entity, 0, 2)

	deploymentEntity := &workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   deployment.Namespace + "/" + deployment.Name, // we use the namespace/name as id to make it easier for the admission controller to retrieve the corresponding deployment
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels:      deployment.Labels,
			Annotations: deployment.Annotations,
		},
		Env:                 deployment.Labels[ddkube.EnvTagLabelKey],
		Service:             deployment.Labels[ddkube.ServiceTagLabelKey],
		Version:             deployment.Labels[ddkube.VersionTagLabelKey],
		InjectableLanguages: containerLanguages,
	}

	entities = append(entities, deploymentEntity, &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", deployment.Namespace, deployment.Name)),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels:      deploymentEntity.Labels,
			Annotations: deploymentEntity.Annotations,
		},
		GVR: deployment.GroupVersionKind().GroupVersion().WithResource("deployments"),
	})

	return entities
}
