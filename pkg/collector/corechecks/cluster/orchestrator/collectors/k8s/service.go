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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewServiceCollectorVersions builds the group of collector versions.
func NewServiceCollectorVersions(metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewServiceCollector(metadataAsTags),
	)
}

// ServiceCollector is a collector for Kubernetes Services.
type ServiceCollector struct {
	informer  corev1Informers.ServiceInformer
	lister    corev1Listers.ServiceLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewServiceCollector creates a new collector for the Kubernetes Service
// resource.
func NewServiceCollector(metadataAsTags utils.MetadataAsTags) *ServiceCollector {
	resourceType := getResourceType(serviceName, serviceVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &ServiceCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 serviceName,
			Kind:                                 kubernetes.ServiceKind,
			NodeType:                             orchestrator.K8sService,
			Version:                              serviceVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.ServiceHandlers)),
	}
}

// Informer returns the shared informer.
func (c *ServiceCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *ServiceCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Core().V1().Services()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *ServiceCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ServiceCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *ServiceCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
