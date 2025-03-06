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
	batchv1Informers "k8s.io/client-go/informers/batch/v1beta1"
	batchv1Listers "k8s.io/client-go/listers/batch/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// CronJobV1Beta1Collector is a collector for Kubernetes CronJobs.
type CronJobV1Beta1Collector struct {
	informer  batchv1Informers.CronJobInformer
	lister    batchv1Listers.CronJobLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewCronJobV1Beta1Collector creates a new collector for the Kubernetes Job resource.
func NewCronJobV1Beta1Collector(metadataAsTags utils.MetadataAsTags) *CronJobV1Beta1Collector {
	resourceType := getResourceType(cronJobName, cronJobVersionV1Beta1)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &CronJobV1Beta1Collector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     false,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 cronJobName,
			Kind:                                 kubernetes.CronJobKind,
			NodeType:                             orchestrator.K8sCronJob,
			Version:                              cronJobVersionV1Beta1,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.CronJobV1Beta1Handlers)),
	}
}

// Informer returns the shared informer.
func (c *CronJobV1Beta1Collector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *CronJobV1Beta1Collector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Batch().V1beta1().CronJobs()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *CronJobV1Beta1Collector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *CronJobV1Beta1Collector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *CronJobV1Beta1Collector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
