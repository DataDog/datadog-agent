// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	ddkubernetes "github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/client-go/tools/cache"
)

// ImprovedTerminatedPodCollector is a collector for Kubernetes Pods that have been deleted.
// Unlike [TerminatedPodCollector] this collector can catch all deletion events, including force deletions.
// For performance reasons it uses a custom event watch logic instead of an informer (no local storage, no heavy List
// calls).
type ImprovedTerminatedPodCollector struct {
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
	watcher   *PodDeletionWatcher
}

// NewImprovedTerminatedPodCollector creates a new collector for terminated pods.
func NewImprovedTerminatedPodCollector(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) *ImprovedTerminatedPodCollector {
	resourceType := utilTypes.GetResourceType(utilTypes.PodName, utilTypes.PodVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &ImprovedTerminatedPodCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_pods_improved.enabled"),
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 utilTypes.TerminatedPodName,
			Kind:                                 ddkubernetes.PodKind,
			NodeType:                             orchestrator.K8sPod,
			Version:                              utilTypes.PodVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(k8sProcessors.NewPodHandlers(cfg, store, tagger)),
	}
}

// Informer returns the shared informer, nil in this case because the collector does not use one.
func (c *ImprovedTerminatedPodCollector) Informer() cache.SharedInformer {
	return nil
}

// Init is used to initialize the collector.
func (c *ImprovedTerminatedPodCollector) Init(rcfg *collectors.CollectorRunConfig) {
	eventHandler := func(pod *v1.Pod) { rcfg.TerminatedResourceHandler(c, pod) }
	c.watcher = NewPodDeletionWatcher(rcfg.APIClient.Cl, eventHandler, rcfg.StopCh)
}

// Metadata is used to access information about the collector.
func (c *ImprovedTerminatedPodCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ImprovedTerminatedPodCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	// ImprovedTerminatedPodCollector does not process any resources as it is only used to collect terminated pods by deletion handler.
	return c.Process(rcfg, []*v1.Pod{})
}

// Process is used to process the list of resources and return the result.
func (c *ImprovedTerminatedPodCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
