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
	netv1Informers "k8s.io/client-go/informers/networking/v1"
	netv1Listers "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

// NewIngressCollectorVersions builds the group of collector versions.
func NewIngressCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewIngressCollector(),
	)
}

// IngressCollector is a collector for Kubernetes Ingresss.
type IngressCollector struct {
	informer  netv1Informers.IngressInformer
	lister    netv1Listers.IngressLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewIngressCollector creates a new collector for the Kubernetes Ingress
// resource.
func NewIngressCollector() *IngressCollector {
	return &IngressCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "ingresses",
			NodeType:                  orchestrator.K8sIngress,
			Version:                   "networking.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.IngressHandlers)),
	}
}

// Informer returns the shared informer.
func (c *IngressCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *IngressCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Networking().V1().Ingresses()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *IngressCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *IngressCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

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
