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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewTerminatedPodCollectorVersions builds the group of collector versions.
func NewTerminatedPodCollectorVersions(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewTerminatedPodCollector(cfg, store, tagger, metadataAsTags),
	)
}

// TerminatedPodCollector is a collector for Kubernetes Pods that are not
// assigned to a node yet.
type TerminatedPodCollector struct {
	informer  corev1Informers.PodInformer
	lister    corev1Listers.PodLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewTerminatedPodCollector creates a new collector for the Kubernetes Pod
// resource that is not assigned to any node.
func NewTerminatedPodCollector(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) *TerminatedPodCollector {
	resourceType := getResourceType(podName, podVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &TerminatedPodCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_pods.enabled"),
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 "terminated-pods",
			NodeType:                             orchestrator.K8sPod,
			Version:                              "v1",
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

	processResult, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(c.processor.Handlers().ResourceList(ctx, list)),
		ResourcesProcessed: processed,
	}

	return result, nil
}
