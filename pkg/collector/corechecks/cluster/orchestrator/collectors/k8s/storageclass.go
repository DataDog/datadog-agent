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
	storagev1Informers "k8s.io/client-go/informers/storage/v1"
	storagev1Listers "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
)

// NewStorageClassCollectorVersions builds the group of collector versions.
func NewStorageClassCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewStorageClassCollector(),
	)
}

// StorageClassCollector is a collector for Kubernetes StorageClasss.
type StorageClassCollector struct {
	informer  storagev1Informers.StorageClassInformer
	lister    storagev1Listers.StorageClassLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewStorageClassCollector creates a new collector for the Kubernetes
// StorageClass resource.
func NewStorageClassCollector() *StorageClassCollector {
	return &StorageClassCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  false,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "storageclasses",
			NodeType:                  orchestrator.K8sStorageClass,
			Version:                   "storage.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.StorageClassHandlers)),
	}
}

// Informer returns the shared informer.
func (c *StorageClassCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *StorageClassCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Storage().V1().StorageClasses()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *StorageClassCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *StorageClassCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
