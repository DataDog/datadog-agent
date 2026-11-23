// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package registry

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The informers of the registry entries that do not use the typed informer factory.

// TypedInformer returns the informer of a resource from the typed informer factory. It is the default informer.
func TypedInformer(rcfg *collectors.CollectorRunConfig, gvr schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	informer, err := rcfg.OrchestratorInformerFactory.InformerFactory.ForResource(gvr)
	if err != nil {
		return nil, err
	}
	return informer.Informer(), nil
}

// DynamicInformer returns the informer of a resource from the dynamic informer factory.
func DynamicInformer(rcfg *collectors.CollectorRunConfig, gvr schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	return rcfg.OrchestratorInformerFactory.DynamicInformerFactory.ForResource(gvr).Informer(), nil
}

func crdInformer(rcfg *collectors.CollectorRunConfig, _ schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	informer, err := newCRDInformer(rcfg)
	if err != nil {
		return nil, err
	}
	return informer.Informer(), nil
}

func vpaInformer(rcfg *collectors.CollectorRunConfig, gvr schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	informer, err := rcfg.OrchestratorInformerFactory.VPAInformerFactory.ForResource(gvr)
	if err != nil {
		return nil, err
	}
	return informer.Informer(), nil
}

func unassignedPodInformer(rcfg *collectors.CollectorRunConfig, gvr schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	informer, err := rcfg.OrchestratorInformerFactory.UnassignedPodInformerFactory.ForResource(gvr)
	if err != nil {
		return nil, err
	}
	return informer.Informer(), nil
}

// podDeletionWatcher reports terminated pods from a dedicated watch, as deleted resources. There is no informer, so
// the bundles share one watch through the informer factory.
func podDeletionWatcher(rcfg *collectors.CollectorRunConfig, _ schema.GroupVersionResource, onDelete func(any)) (cache.SharedIndexInformer, error) {
	deletions := &rcfg.OrchestratorInformerFactory.PodDeletions
	if deletions.Add(onDelete) {
		NewPodDeletionWatcher(rcfg.APIClient.Cl, func(pod *v1.Pod) { deletions.Notify(pod) }, rcfg.StopCh)
	}
	return nil, nil
}

func configMapMetadataInformer(rcfg *collectors.CollectorRunConfig, _ schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	return newConfigMapMetadataInformer(rcfg).Informer(), nil
}

func secretMetadataInformer(rcfg *collectors.CollectorRunConfig, _ schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	informer, err := newSecretMetadataInformer(rcfg)
	if err != nil {
		return nil, err
	}
	return informer.Informer(), nil
}

func helmReleaseConfigMapInformer(rcfg *collectors.CollectorRunConfig, _ schema.GroupVersionResource, _ func(any)) (cache.SharedIndexInformer, error) {
	return newHelmReleaseConfigMapInformer(rcfg).Informer(), nil
}

const (
	// HelmReleaseLabelSelector selects the ConfigMaps Helm uses to store releases.
	HelmReleaseLabelSelector = "owner=helm"
	// NonHelmReleaseLabelSelector excludes the ConfigMaps Helm uses to store releases.
	NonHelmReleaseLabelSelector = "owner!=helm"
)

// newHelmReleaseConfigMapInformer returns the Helm release ConfigMap informer, which trims objects before it stores
// them. The collector and the resource registry share it.
func newHelmReleaseConfigMapInformer(rcfg *collectors.CollectorRunConfig) corev1Informers.ConfigMapInformer {
	informer := rcfg.OrchestratorInformerFactory.HelmReleaseConfigMapInformerFactory.Core().V1().ConfigMaps()
	if err := informer.Informer().SetTransform(trimHelmReleaseConfigMap); err != nil {
		log.Debugf("Unable to set transform on the Helm release ConfigMap informer: %v", err)
	}
	return informer
}

// trimHelmReleaseConfigMap drops fields that are never sent before objects are
// stored in the informer cache, to reduce its memory footprint. The
// last-applied-configuration annotation, when present, holds a full copy of
// the ConfigMap data.
func trimHelmReleaseConfigMap(obj interface{}) (interface{}, error) {
	if cm, ok := obj.(*v1.ConfigMap); ok {
		cm.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(cm.Annotations, cm.Labels)
	}
	return obj, nil
}

var configMapGVR = schema.GroupVersionResource{
	Group:    utilTypes.ConfigMapGroup,
	Version:  utilTypes.ConfigMapVersion,
	Resource: utilTypes.ConfigMapName,
}

// newConfigMapMetadataInformer returns the ConfigMap metadata informer, shared by the collector and the resource registry.
func newConfigMapMetadataInformer(rcfg *collectors.CollectorRunConfig) informers.GenericInformer {
	informer := rcfg.OrchestratorInformerFactory.ConfigMapMetadataInformerFactory.ForResource(configMapGVR)
	if err := informer.Informer().SetTransform(trimConfigMapMetadata); err != nil {
		log.Debugf("Unable to set transform on the ConfigMap informer: %v", err)
	}
	return informer
}

// trimConfigMapMetadata drops fields that are never sent before objects are
// stored in the informer cache, to reduce its memory footprint. The
// last-applied-configuration annotation, when present, holds a full copy of
// the ConfigMap data.
func trimConfigMapMetadata(obj interface{}) (interface{}, error) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(m.Annotations, m.Labels)
	}
	return obj, nil
}

var secretGVR = schema.GroupVersionResource{
	Group:    utilTypes.SecretGroup,
	Version:  utilTypes.SecretVersion,
	Resource: utilTypes.SecretName,
}

// newSecretMetadataInformer returns the Secret metadata informer, shared by the collector and the resource registry. Its
// error means that Secrets must not be collected, as the cache keeps fields that are never sent.
func newSecretMetadataInformer(rcfg *collectors.CollectorRunConfig) (informers.GenericInformer, error) {
	informer := rcfg.OrchestratorInformerFactory.SecretMetadataInformerFactory.ForResource(secretGVR)
	if err := informer.Informer().SetTransform(trimSecretMetadata); err != nil {
		return informer, fmt.Errorf("unable to set transform on the Secret informer, Secret collection is disabled: %w", err)
	}
	return informer, nil
}

// trimSecretMetadata drops fields that are never sent before objects are
// stored in the informer cache. Sensitive annotations and labels are deleted
// rather than redacted: the last-applied-configuration annotation, when
// present, holds a full copy of the Secret, including its data.
func trimSecretMetadata(obj interface{}) (interface{}, error) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.ManagedFields = nil
		for _, key := range redact.GetSensitiveAnnotationsAndLabels() {
			delete(m.Annotations, key)
			delete(m.Labels, key)
		}
	}
	return obj, nil
}

// newCRDInformer returns the CustomResourceDefinition informer, shared by the collector and the resource registry.
func newCRDInformer(rcfg *collectors.CollectorRunConfig) (informers.GenericInformer, error) {
	return rcfg.OrchestratorInformerFactory.CRDInformerFactory.ForResource(apiextensionsv1.SchemeGroupVersion.WithResource(utilTypes.CrdName))
}
