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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	appsv1Informers "k8s.io/client-go/informers/apps/v1"
	appsv1Listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

// NewDaemonSetCollectorVersions builds the group of collector versions.
func NewDaemonSetCollectorVersions(metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewDaemonSetCollector(metadataAsTags),
	)
}

// DaemonSetCollector is a collector for Kubernetes DaemonSets.
type DaemonSetCollector struct {
	informer  appsv1Informers.DaemonSetInformer
	lister    appsv1Listers.DaemonSetLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewDaemonSetCollector creates a new collector for the Kubernetes DaemonSet
// resource.
func NewDaemonSetCollector(metadataAsTags utils.MetadataAsTags) *DaemonSetCollector {
	resourceType := getResourceType(daemonSetName, daemonSetVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &DaemonSetCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 daemonSetName,
			NodeType:                             orchestrator.K8sDaemonSet,
			Version:                              daemonSetVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.DaemonSetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *DaemonSetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *DaemonSetCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Apps().V1().DaemonSets()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *DaemonSetCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *DaemonSetCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *DaemonSetCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	processResult, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(c.processor.Handlers().ResourceList(ctx, list)),
		ResourcesProcessed: processed,
	}

	return result, nil
}
