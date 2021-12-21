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
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// K8sClusterCollector is a collector for Kubernetes clusters.
type K8sClusterCollector struct {
	informer corev1Informers.NodeInformer
	lister   corev1Listers.NodeLister
	meta     *CollectorMetadata
	proc     *processors.K8sClusterProcessor
}

func newK8sClusterCollector() *K8sClusterCollector {
	return &K8sClusterCollector{
		meta: &CollectorMetadata{
			IsStable: true,
			Name:     "clusters",
			NodeType: orchestrator.K8sCluster,
		},
		proc: processors.NewK8sClusterProcessor(),
	}
}

// Informer returns the shared informer.
func (c *K8sClusterCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sClusterCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Core().V1().Nodes()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sClusterCollector) Metadata() *CollectorMetadata {
	return c.meta
}

// Run triggers the collection process.
func (c *K8sClusterCollector) Run(rcfg *CollectorRunConfig) (res *CollectorRunResult, err error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, newListingError(err)
	}

	ctx := &processors.ProcessorContext{
		APIClient:  rcfg.APIClient,
		Cfg:        rcfg.Config,
		ClusterID:  rcfg.ClusterID,
		MsgGroupID: atomic.AddInt32(rcfg.MsgGroupRef, 1),
		NodeType:   c.meta.NodeType,
	}

	messages, processed, err := c.proc.Process(ctx, list)

	// This would happen when recovering from a processor panic. In the nominal
	// case we would have a positive integer set at the very end of processing.
	// If this is not the case then it means code execution stopped sooner.
	// Panic recovery will log more information about the error so we can figure
	// out the root cause.
	if processed == -1 {
		return nil, errProcessingPanic
	}

	// The cluster processor can return errors since it has to grab extra
	// information from the API server during processing.
	if err != nil {
		return nil, newProcessingError(err)
	}

	result := &CollectorRunResult{
		Messages:           messages,
		ResourcesListed:    1,
		ResourcesProcessed: processed,
	}

	return result, nil
}
