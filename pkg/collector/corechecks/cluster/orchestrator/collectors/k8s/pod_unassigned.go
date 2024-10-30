// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewUnassignedPodCollectorVersions builds the group of collector versions.
func NewUnassignedPodCollectorVersions(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewUnassignedPodCollector(cfg, store, tagger),
	)
}

// UnassignedPodCollector is a collector for Kubernetes Pods that are not
// assigned to a node yet.
type UnassignedPodCollector struct {
	informer  corev1Informers.PodInformer
	lister    corev1Listers.PodLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewUnassignedPodCollector creates a new collector for the Kubernetes Pod
// resource that is not assigned to any node.
func NewUnassignedPodCollector(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *UnassignedPodCollector {
	return &UnassignedPodCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "pods",
			NodeType:                  orchestrator.K8sPod,
			Version:                   "v1",
		},
		processor: processors.NewProcessor(k8sProcessors.NewPodHandlers(cfg, store, tagger)),
	}
}

// Informer returns the shared informer.
func (c *UnassignedPodCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *UnassignedPodCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.UnassignedPodInformerFactory.Core().V1().Pods()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *UnassignedPodCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *UnassignedPodCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
