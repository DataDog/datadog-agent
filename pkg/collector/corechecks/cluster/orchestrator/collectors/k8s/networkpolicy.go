// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"k8s.io/apimachinery/pkg/labels"
	networkingv1Informers "k8s.io/client-go/informers/networking/v1"
	networkingv1Listers "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// NewNetworkPolicyCollectorVersions builds the group of collector versions.
func NewNetworkPolicyCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewNetworkPolicyCollector(),
	)
}

// NetworkPolicyCollector is a collector for Kubernetes NetworkPolicies.
type NetworkPolicyCollector struct {
	informer  networkingv1Informers.NetworkPolicyInformer
	lister    networkingv1Listers.NetworkPolicyLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewNetworkPolicyCollector creates a new collector for the Kubernetes
// NetworkPolicy resource.
func NewNetworkPolicyCollector() *NetworkPolicyCollector {
	return &NetworkPolicyCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "networkpolicies",
			NodeType:                  orchestrator.K8sNetworkPolicy,
			Version:                   "networking.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.NetworkPolicyHandlers)),
	}
}

// Informer returns the shared informer.
func (c *NetworkPolicyCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *NetworkPolicyCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Networking().V1().NetworkPolicies()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *NetworkPolicyCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *NetworkPolicyCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
