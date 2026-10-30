// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// HelmReleaseLabelSelector selects the ConfigMaps Helm uses to store releases.
	HelmReleaseLabelSelector = "owner=helm"
	// NonHelmReleaseLabelSelector excludes the ConfigMaps Helm uses to store releases.
	NonHelmReleaseLabelSelector = "owner!=helm"
)

// NewHelmReleaseConfigMapCollectorVersions builds the group of collector versions.
func NewHelmReleaseConfigMapCollectorVersions(_ tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewHelmReleaseConfigMapCollector(),
	)
}

// HelmReleaseConfigMapCollector is a collector for Helm releases stored in Kubernetes ConfigMaps (Helm's configmap driver).
// Every revision is collected and sent as is, as a ConfigMap. Its informer only watches ConfigMaps labeled owner=helm,
// which the ConfigMap collector excludes, so each ConfigMap is reported by a single collector.
type HelmReleaseConfigMapCollector struct {
	informer  corev1Informers.ConfigMapInformer
	lister    corev1Listers.ConfigMapLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewHelmReleaseConfigMapCollector creates a new collector for Helm releases.
func NewHelmReleaseConfigMapCollector() *HelmReleaseConfigMapCollector {
	return &HelmReleaseConfigMapCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			IsStable:                             pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.helm_releases.configmaps.enabled"),
			SupportsManifestBuffering:            true,
			SupportsTerminatedResourceCollection: true,
			Group:                                utilTypes.ConfigMapGroup,
			Version:                              utilTypes.ConfigMapVersion,
			Kind:                                 kubernetes.ConfigMapKind,
			Name:                                 utilTypes.HelmReleaseConfigMapName,
			NodeType:                             orchestrator.K8sConfigMap,
		},
		processor: processors.NewProcessor(k8sProcessors.NewHelmReleaseConfigMapHandlers()),
	}
}

// Informer returns the shared informer.
func (c *HelmReleaseConfigMapCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *HelmReleaseConfigMapCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.HelmReleaseConfigMapInformerFactory.Core().V1().ConfigMaps()
	if err := c.informer.Informer().SetTransform(trimHelmReleaseConfigMap); err != nil {
		log.Debugf("Unable to set transform on the Helm release ConfigMap informer: %v", err)
	}
	c.lister = c.informer.Lister()
}

// trimHelmReleaseConfigMap drops fields that are never sent before objects are
// stored in the informer cache, to reduce its memory footprint. The
// last-applied-configuration annotation, when present, holds a full copy of
// the ConfigMap data.
func trimHelmReleaseConfigMap(obj interface{}) (interface{}, error) {
	if cm, ok := obj.(*corev1.ConfigMap); ok {
		cm.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(cm.Annotations, cm.Labels)
	}
	return obj, nil
}

// Metadata is used to access information about the collector.
func (c *HelmReleaseConfigMapCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *HelmReleaseConfigMapCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *HelmReleaseConfigMapCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	processResult, listed, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    listed,
		ResourcesProcessed: processed,
	}

	return result, nil
}
