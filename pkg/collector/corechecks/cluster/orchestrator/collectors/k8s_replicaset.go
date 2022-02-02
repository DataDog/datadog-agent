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

// K8sReplicaSetCollector is a collector for Kubernetes ReplicaSets.
type K8sReplicaSetCollector struct {
	informer appsv1Informers.ReplicaSetInformer
	lister   appsv1Listers.ReplicaSetLister
	meta     *CollectorMetadata
	proc     *processors.Processor
}

func newK8sReplicaSetCollector() *K8sReplicaSetCollector {
	return &K8sReplicaSetCollector{
		meta: &CollectorMetadata{
			IsStable: true,
			Name:     "replicasets",
			NodeType: orchestrator.K8sReplicaSet,
		},
		proc: processors.NewProcessor(new(processors.K8sReplicaSetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sReplicaSetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sReplicaSetCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Apps().V1().ReplicaSets()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sReplicaSetCollector) Metadata() *CollectorMetadata {
	return c.meta
}

// Run triggers the collection process.
func (c *K8sReplicaSetCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
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
