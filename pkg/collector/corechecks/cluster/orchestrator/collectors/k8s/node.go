// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewNodeCollectorVersions builds the group of collector versions.
func NewNodeCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewNodeCollector(),
	)
}

// NodeCollector is a collector for Kubernetes Nodes.
type NodeCollector struct {
	informer  corev1Informers.NodeInformer
	lister    corev1Listers.NodeLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewNodeCollector creates a new collector for the Kubernetes Node resource.
func NewNodeCollector() *NodeCollector {
	return &NodeCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "nodes",
			NodeType:                  orchestrator.K8sNode,
			Version:                   "v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.NodeHandlers)),
	}
}

// Informer returns the shared informer.
func (c *NodeCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *NodeCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Core().V1().Nodes()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *NodeCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *NodeCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *NodeCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
