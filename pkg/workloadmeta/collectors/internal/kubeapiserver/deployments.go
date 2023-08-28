// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/config"
	ddkube "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const deploymentStoreName = "deployments-store"

func init() {
	resourceSpecificGenerator[deploymentStoreName] = newDeploymentStore
}

// deploymentFilter filters out deployments that can't be used for unified service tagging or process language detection
type deploymentFilter struct{}

func (f *deploymentFilter) filteredOut(obj metav1.Object) bool {
	labels := obj.GetLabels()
	if _, ok := labels[ddkube.EnvTagLabelKey]; ok {
		return false
	}

	if _, ok := labels[ddkube.ServiceTagLabelKey]; ok {
		return false
	}

	if _, ok := labels[ddkube.VersionTagLabelKey]; ok {
		return false
	}

	// if _, ok := deployment.Annotations["tags.datadog.com/languages"] { // stub, exact annotation will need to be defined in the future
	// 	return false
	// }

	return true
}

func newDeploymentStore(ctx context.Context, cfg config.Config, wlm workloadmeta.Store, client kubernetes.Interface) (*cache.Reflector, *reflectorStore, error) {
	if !cfg.GetBool("language_detection.enabled") {
		return nil, nil, fmt.Errorf("language detection is enabled") // we might remove this if we want to use deployments info in unified service tagging
	}
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
	return deploymentReflector, deploymentStore, nil
}

func newDeploymentReflectorStore(wlmetaStore workloadmeta.Store) *reflectorStore {
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

func (p deploymentParser) Parse(obj interface{}) workloadmeta.Entity {
	deployment := obj.(*appsv1.Deployment)

	return &workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   deployment.Name, // not sure if we should use the UID or the name here
		},
		Env:     getOrDefault(deployment.Labels, ddkube.EnvTagLabelKey, ""),
		Service: getOrDefault(deployment.Labels, ddkube.ServiceTagLabelKey, ""),
		Version: getOrDefault(deployment.Labels, ddkube.VersionTagLabelKey+"version", ""),
	}
}

func getOrDefault(m map[string]string, key, defaultV string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return defaultV
}
