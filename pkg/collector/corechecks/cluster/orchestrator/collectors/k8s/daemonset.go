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

// NewDaemonSetCollectorVersions builds the group of collector versions.
func NewDaemonSetCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewDaemonSetCollector(),
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
func NewDaemonSetCollector() *DaemonSetCollector {
	return &DaemonSetCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "daemonsets",
			NodeType:                  orchestrator.K8sDaemonSet,
			Version:                   "apps/v1",
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
	c.informer = rcfg.APIClient.InformerFactory.Apps().V1().DaemonSets()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *DaemonSetCollector) IsAvailable() bool { return true }

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
