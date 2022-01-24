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
	batchv1Informers "k8s.io/client-go/informers/batch/v1"
	batchv1Listers "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
)

// K8sJobCollector is a collector for Kubernetes Jobs.
type K8sJobCollector struct {
	informer  batchv1Informers.JobInformer
	lister    batchv1Listers.JobLister
	metadata      *CollectorMetadata
	processor *processors.Processor
}

func newK8sJobCollector() *K8sJobCollector {
	return &K8sJobCollector{
		metadata: &CollectorMetadata{
			IsStable: true,
			Name:     "jobs",
			NodeType: orchestrator.K8sJob,
		},
		processor: processors.NewProcessor(new(processors.K8sJobHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sJobCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sJobCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Batch().V1().Jobs()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sJobCollector) Metadata() *CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *K8sJobCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
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
