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
	appsv1Informers "k8s.io/client-go/informers/apps/v1"
	appsv1Listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

// K8sStatefulSetCollector is a collector for Kubernetes StatefulSets.
type K8sStatefulSetCollector struct {
	informer  appsv1Informers.StatefulSetInformer
	lister    appsv1Listers.StatefulSetLister
	metadata  *CollectorMetadata
	processor *processors.Processor
}

func newK8sStatefulSetCollector() *K8sStatefulSetCollector {
	return &K8sStatefulSetCollector{
		metadata: &CollectorMetadata{
			IsStable: true,
			Name:     "statefulsets",
			NodeType: orchestrator.K8sStatefulSet,
		},
		processor: processors.NewProcessor(new(processors.K8sStatefulSetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sStatefulSetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sStatefulSetCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Apps().V1().StatefulSets()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sStatefulSetCollector) Metadata() *CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *K8sStatefulSetCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
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
