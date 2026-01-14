// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// NewTerminatedPodCollectorVersions builds the group of collector versions.
func NewTerminatedPodCollectorVersions(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewTerminatedPodCollector(cfg, store, tagger, metadataAsTags),
	)
}

// TerminatedPodCollector is a collector for terminated Kubernetes Pods.
// Unlike other collectors, it does not use an informer. Instead, pod deletions
// are captured by PodDeletionWatcher and processed through this collector.
type TerminatedPodCollector struct {
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewTerminatedPodCollector creates a new collector for the Kubernetes Pod
// resource that is not assigned to any node.
func NewTerminatedPodCollector(cfg config.Component, store workloadmeta.Component, tagger tagger.Component, metadataAsTags utils.MetadataAsTags) *TerminatedPodCollector {
	resourceType := utilTypes.GetResourceType(utilTypes.PodName, utilTypes.PodVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &TerminatedPodCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_pods.enabled"),
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      utilTypes.TerminatedPodName,
			Kind:                      kubernetes.PodKind,
			NodeType:                  orchestrator.K8sPod,
			Version:                   utilTypes.PodVersion,
			LabelsAsTags:              labelsAsTags,
			AnnotationsAsTags:         annotationsAsTags,
			// Note: SupportsTerminatedResourceCollection is not set because this collector
			// uses PodDeletionWatcher instead of informer-based delete event handlers.
		},
		processor: processors.NewProcessor(k8sProcessors.NewPodHandlers(cfg, store, tagger)),
	}
}

// Informer returns nil because this collector uses PodDeletionWatcher instead of an informer.
func (c *TerminatedPodCollector) Informer() cache.SharedInformer {
	return nil
}

// Init is a no-op because this collector uses PodDeletionWatcher instead of an informer.
func (c *TerminatedPodCollector) Init(rcfg *collectors.CollectorRunConfig) {
	// No initialization needed - pod deletions are handled by PodDeletionWatcher
}

// Metadata is used to access information about the collector.
func (c *TerminatedPodCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *TerminatedPodCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	// TerminatedPodCollector does not process any resources as it is only used to collect terminated pods by deletion handler.
	return c.Process(rcfg, []*v1.Pod{})
}

// Process is used to process the list of resources and return the result.
func (c *TerminatedPodCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
