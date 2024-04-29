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
	corev1Informers "k8s.io/client-go/informers/core/v1"
	corev1Listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// NewLimitRangeCollectorVersions builds the group of collector versions.
func NewLimitRangeCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewLimitRangeCollector(),
	)
}

// LimitRangeCollector is a collector for Kubernetes LimitRanges.
type LimitRangeCollector struct {
	informer  corev1Informers.LimitRangeInformer
	lister    corev1Listers.LimitRangeLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewLimitRangeCollector creates a new collector for the Kubernetes
// LimitRange resource.
func NewLimitRangeCollector() *LimitRangeCollector {
	return &LimitRangeCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  false,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "limitranges",
			NodeType:                  orchestrator.K8sLimitRange,
			Version:                   "v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.LimitRangeHandlers)),
	}
}

// Informer returns the shared informer.
func (c *LimitRangeCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *LimitRangeCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Core().V1().LimitRanges()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *LimitRangeCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *LimitRangeCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
