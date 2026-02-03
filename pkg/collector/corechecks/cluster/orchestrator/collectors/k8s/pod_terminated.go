// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewTerminatedPodCollectorVersions builds the group of collector versions.
func NewTerminatedPodCollectorVersions(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	if pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_pods_improved.enabled") {
		return collectors.NewCollectorVersions(NewImprovedTerminatedPodCollector(cfg, store, tagger, metadataAsTags))
	}
	return collectors.NewCollectorVersions(NewTerminatedPodCollector(cfg, store, tagger, metadataAsTags))
}

// TerminatedPodCollector is a collector for Kubernetes Pods that have been deleted.
type TerminatedPodCollector struct {
	informer  corev1Informers.PodInformer
	lister    corev1Listers.PodLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewTerminatedPodCollector creates a new collector for terminated pods.
func NewTerminatedPodCollector(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) *TerminatedPodCollector {
	resourceType := utilTypes.GetResourceType(utilTypes.PodName, utilTypes.PodVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &TerminatedPodCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_pods.enabled"),
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 utilTypes.TerminatedPodName,
			Kind:                                 kubernetes.PodKind,
			NodeType:                             orchestrator.K8sPod,
			Version:                              utilTypes.PodVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(k8sProcessors.NewPodHandlers(cfg, store, tagger)),
	}
}

// Informer returns the shared informer.
func (c *TerminatedPodCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *TerminatedPodCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.TerminatedPodInformerFactory.Core().V1().Pods()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *TerminatedPodCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *TerminatedPodCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	// TerminatedPodCollector does not process any resources as it is only used to collect terminated pods by deletion handler.
	return c.Process(rcfg, []*v1.Pod{})
}

// Process is used to process the list of resources and return the result.
func (c *TerminatedPodCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
