// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	networkingv1Informers "k8s.io/client-go/informers/networking/v1"
	networkingv1Listers "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// NewNetworkPolicyCollectorVersions builds the group of collector versions.
func NewNetworkPolicyCollectorVersions(tagger tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewNetworkPolicyCollector(tagger),
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
func NewNetworkPolicyCollector(tagger tagger.Component) *NetworkPolicyCollector {
	return &NetworkPolicyCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 utilTypes.NetworkPolicyName,
			Kind:                                 kubernetes.NetworkPolicyKind,
			NodeType:                             orchestrator.K8sNetworkPolicy,
			Group:                                utilTypes.NetworkPolicyGroup,
			Version:                              utilTypes.NetworkPolicyVersion,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(k8sProcessors.NewNetworkPolicyHandlers(tagger)),
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

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *NetworkPolicyCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	processResult, listed, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    listed,
		ResourcesProcessed: processed,
	}

	return result, nil
}
