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

// K8sPersistentVolumeClaimCollector is a collector for Kubernetes PersistentVolumeClaims.
type K8sPersistentVolumeClaimCollector struct {
	informer corev1Informers.PersistentVolumeClaimInformer
	lister   corev1Listers.PersistentVolumeClaimLister
	meta     *CollectorMetadata
	proc     *processors.Processor
}

func newK8sPersistentVolumeClaimCollector() *K8sPersistentVolumeClaimCollector {
	return &K8sPersistentVolumeClaimCollector{
		meta: &CollectorMetadata{
			IsStable: false,
			Name:     "persistentvolumeclaims",
			NodeType: orchestrator.K8sPersistentVolumeClaim,
		},
		proc: processors.NewProcessor(new(processors.K8sPersistentVolumeClaimHandlers)),
	}
}

// Informer returns the shared informer.
func (c *K8sPersistentVolumeClaimCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *K8sPersistentVolumeClaimCollector) Init(rcfg *CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Core().V1().PersistentVolumeClaims()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *K8sPersistentVolumeClaimCollector) Metadata() *CollectorMetadata {
	return c.meta
}

// Run triggers the collection process.
func (c *K8sPersistentVolumeClaimCollector) Run(rcfg *CollectorRunConfig) (*CollectorRunResult, error) {
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
