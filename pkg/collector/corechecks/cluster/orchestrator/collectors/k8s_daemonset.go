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

// K8sDaemonSetCollector is a collector for Kubernetes DaemonSets.
type K8sDaemonSetCollector struct {
	informer appsv1Informers.DaemonSetInformer
	lister   appsv1Listers.DaemonSetLister
	meta     *CollectorMetadata
	proc     *processors.Processor
}

func newK8sDaemonSetCollector() *K8sDaemonSetCollector {
	return &K8sDaemonSetCollector{
		meta: &CollectorMetadata{
			IsStable: true,
			Name:     "daemonsets",
			NodeType: orchestrator.K8sDaemonSet,
		},
		proc: processors.NewProcessor(new(processors.K8sDaemonSetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sDaemonSetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sDaemonSetCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Apps().V1().DaemonSets()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sDaemonSetCollector) Metadata() *CollectorMetadata {
	return c.meta
}

// Run triggers the collection process.
func (c *K8sDaemonSetCollector) Run(rcfg *CollectorRunConfig) (res *CollectorRunResult, err error) {
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

	messages, processed := c.proc.Process(ctx, list)

	// This would happen when recovering from a processor panic. In the nominal
	// case we would have a positive integer set at the very end of processing.
	// If this is not the case then it means code execution stopped sooner.
	// Panic recovery will log more information about the error so we can figure
	// out the root cause.
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
