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
	appsv1Informers "k8s.io/client-go/informers/apps/v1"
	appsv1Listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

// NewStatefulSetCollectorVersions builds the group of collector versions.
func NewStatefulSetCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewStatefulSetCollector(),
	)
}

// StatefulSetCollector is a collector for Kubernetes StatefulSets.
type StatefulSetCollector struct {
	informer  appsv1Informers.StatefulSetInformer
	lister    appsv1Listers.StatefulSetLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewStatefulSetCollector creates a new collector for the Kubernetes
// StatefulSet resource.
func NewStatefulSetCollector() *StatefulSetCollector {
	return &StatefulSetCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "statefulsets",
			NodeType:                  orchestrator.K8sStatefulSet,
			Version:                   "apps/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.StatefulSetHandlers)),
	}
}

// Informer returns the shared informer.
func (c *StatefulSetCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *StatefulSetCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Apps().V1().StatefulSets()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *StatefulSetCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *StatefulSetCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *StatefulSetCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	ctx := collectors.NewProcessorContext(rcfg, c.metadata)

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
