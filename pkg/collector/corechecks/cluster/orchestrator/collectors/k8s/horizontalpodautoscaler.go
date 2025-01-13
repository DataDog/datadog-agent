// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	v2Informers "k8s.io/client-go/informers/autoscaling/v2"
	v2Listers "k8s.io/client-go/listers/autoscaling/v2"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NewHorizontalPodAutoscalerCollectorVersions builds the group of collector versions.
func NewHorizontalPodAutoscalerCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewHorizontalPodAutoscalerCollector(),
	)
}

// HorizontalPodAutoscalerCollector is a collector for Kubernetes HPAs.
type HorizontalPodAutoscalerCollector struct {
	informer  v2Informers.HorizontalPodAutoscalerInformer
	lister    v2Listers.HorizontalPodAutoscalerLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewHorizontalPodAutoscalerCollector creates a new collector for the Kubernetes
// HorizontalPodAutoscaler resource.
func NewHorizontalPodAutoscalerCollector() *HorizontalPodAutoscalerCollector {
	return &HorizontalPodAutoscalerCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 "horizontalpodautoscalers",
			NodeType:                             orchestrator.K8sHorizontalPodAutoscaler,
			Version:                              "autoscaling/v2",
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.HorizontalPodAutoscalerHandlers)),
	}
}

// Informer returns the shared informer.
func (c *HorizontalPodAutoscalerCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *HorizontalPodAutoscalerCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Autoscaling().V2().HorizontalPodAutoscalers()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *HorizontalPodAutoscalerCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the hpa collection process.
func (c *HorizontalPodAutoscalerCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list, false)
}

// Process is used to process the list of resources and return the result.
func (c *HorizontalPodAutoscalerCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}, isTerminatedResource bool) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata, isTerminatedResource)

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
