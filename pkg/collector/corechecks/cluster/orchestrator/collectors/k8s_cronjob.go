// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package collectors

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	batchv1Informers "k8s.io/client-go/informers/batch/v1beta1"
	batchv1Listers "k8s.io/client-go/listers/batch/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// K8sCronJobCollector is a collector for Kubernetes CronJobs.
type K8sCronJobCollector struct {
	informer  batchv1Informers.CronJobInformer
	lister    batchv1Listers.CronJobLister
	metadata  *CollectorMetadata
	processor *processors.Processor
}

func newK8sCronJobCollector() *K8sCronJobCollector {
	return &K8sCronJobCollector{
		metadata: &CollectorMetadata{
			IsStable: true,
			Name:     "cronjobs",
			NodeType: orchestrator.K8sCronJob,
		},
		processor: processors.NewProcessor(new(processors.K8sCronJobHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sCronJobCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sCronJobCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Batch().V1beta1().CronJobs()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sCronJobCollector) Metadata() *CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *K8sCronJobCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, newListingError(err)
	}

	ctx := &processors.ProcessorContext{
		APIClient:  rcfg.APIClient,
		Cfg:        rcfg.Config,
		ClusterID:  rcfg.ClusterID,
		MsgGroupID: atomic.AddInt32(rcfg.MsgGroupRef, 1),
		NodeType:   c.metadata.NodeType,
	}

	messages, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, errProcessingPanic
	}

	result := &CollectorRunResult{
		Messages:           messages,
		ResourcesListed:    len(list),
		ResourcesProcessed: processed,
	}

	return result, nil
}
