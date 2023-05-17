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
func NewCronJobV1Beta1Collector() *CronJobV1Beta1Collector {
	return &CronJobV1Beta1Collector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          false,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "cronjobs",
			NodeType:                  orchestrator.K8sCronJob,
			Version:                   "batch/v1beta1",
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
	c.informer = rcfg.APIClient.InformerFactory.Batch().V1beta1().CronJobs()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *CronJobV1Beta1Collector) IsAvailable() bool { return true }

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

	ctx := collectors.NewProcessorContext(rcfg, c.metadata)

	processResult, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(list),
		ResourcesProcessed: processed,
	}

	return result, nil
}
