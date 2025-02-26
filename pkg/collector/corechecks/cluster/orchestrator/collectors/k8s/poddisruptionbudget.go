// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"k8s.io/apimachinery/pkg/labels"
	v1policyinformer "k8s.io/client-go/informers/policy/v1"
	v1policylister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// NewPodDisruptionBudgetCollectorVersions builds the group of collector versions.
func NewPodDisruptionBudgetCollectorVersions(metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewPodDisruptionBudgetCollectorVersion(metadataAsTags),
	)
}

// PodDisruptionBudgetCollector is a collector for Kubernetes Pod Disruption Budgets.
type PodDisruptionBudgetCollector struct {
	informer  v1policyinformer.PodDisruptionBudgetInformer
	lister    v1policylister.PodDisruptionBudgetLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewPodDisruptionBudgetCollectorVersion creates a new collector for the Kubernetes Pod Disruption Budget
// resource.
func NewPodDisruptionBudgetCollectorVersion(metadataAsTags utils.MetadataAsTags) *PodDisruptionBudgetCollector {
	resourceType := getResourceType(podDisruptionBudgetName, podDisruptionBudgetVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &PodDisruptionBudgetCollector{
		informer: nil,
		lister:   nil,
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 podDisruptionBudgetName,
			NodeType:                             orchestrator.K8sPodDisruptionBudget,
			Version:                              podDisruptionBudgetVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.PodDisruptionBudgetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *PodDisruptionBudgetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *PodDisruptionBudgetCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Policy().V1().PodDisruptionBudgets()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *PodDisruptionBudgetCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *PodDisruptionBudgetCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *PodDisruptionBudgetCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
