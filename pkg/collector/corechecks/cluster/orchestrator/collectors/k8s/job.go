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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	batchv1Informers "k8s.io/client-go/informers/batch/v1"
	batchv1Listers "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
)

// NewJobCollectorVersions builds the group of collector versions.
func NewJobCollectorVersions(metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewJobCollector(metadataAsTags),
	)
}

// JobCollector is a collector for Kubernetes Jobs.
type JobCollector struct {
	informer  batchv1Informers.JobInformer
	lister    batchv1Listers.JobLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewJobCollector creates a new collector for the Kubernetes Job resource.
func NewJobCollector(metadataAsTags utils.MetadataAsTags) *JobCollector {
	resourceType := getResourceType(jobName, jobVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &JobCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 jobName,
			Kind:                                 kubernetes.JobKind,
			NodeType:                             orchestrator.K8sJob,
			Version:                              jobVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.JobHandlers)),
	}
}

// Informer returns the shared informer.
func (c *JobCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *JobCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Batch().V1().Jobs()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *JobCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *JobCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *JobCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
