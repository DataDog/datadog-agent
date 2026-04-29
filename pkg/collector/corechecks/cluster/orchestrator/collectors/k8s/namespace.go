// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewNamespaceCollectorVersions builds the group of collector versions.
func NewNamespaceCollectorVersions(tagger tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewNamespaceCollector(tagger),
	)
}

// NamespaceCollector is a collector for Kubernetes Namespaces.
type NamespaceCollector struct {
	informer  corev1Informers.NamespaceInformer
	lister    corev1Listers.NamespaceLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewNamespaceCollector creates a new collector for the Kubernetes
// Namespace resource.
func NewNamespaceCollector(tagger tagger.Component) *NamespaceCollector {
	return &NamespaceCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 utilTypes.NamespaceName,
			Kind:                                 kubernetes.NamespaceKind,
			NodeType:                             orchestrator.K8sNamespace,
			Group:                                utilTypes.NamespaceGroup,
			Version:                              utilTypes.NamespaceVersion,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(k8sProcessors.NewNamespaceHandlers(tagger)),
	}
}

// Informer returns the shared informer.
func (c *NamespaceCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *NamespaceCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Core().V1().Namespaces()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *NamespaceCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *NamespaceCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *NamespaceCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
