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

	v1Informers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions/autoscaling.k8s.io/v1"
	v1Listers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/listers/autoscaling.k8s.io/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NewVerticalPodAutoscalerCollectorVersions builds the group of collector versions.
func NewVerticalPodAutoscalerCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewVerticalPodAutoscalerCollector(),
	)
}

// VerticalPodAutoscalerCollector is a collector for Kubernetes VPAs.
type VerticalPodAutoscalerCollector struct {
	informer  v1Informers.VerticalPodAutoscalerInformer
	lister    v1Listers.VerticalPodAutoscalerLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewVerticalPodAutoscalerCollector creates a new collector for the Kubernetes
// VerticalPodAutoscaler resource.
func NewVerticalPodAutoscalerCollector() *VerticalPodAutoscalerCollector {
	return &VerticalPodAutoscalerCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  false,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "verticalpodautoscalers",
			NodeType:                  orchestrator.K8sVerticalPodAutoscaler,
			Version:                   "autoscaling.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.VerticalPodAutoscalerHandlers)),
	}
}

// Informer returns the shared informer.
func (c *VerticalPodAutoscalerCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *VerticalPodAutoscalerCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.VPAInformerFactory.Autoscaling().V1().VerticalPodAutoscalers()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *VerticalPodAutoscalerCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *VerticalPodAutoscalerCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the vpa collection process.
func (c *VerticalPodAutoscalerCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	ctx := &processors.ProcessorContext{
		APIClient:  rcfg.APIClient,
		Cfg:        rcfg.Config,
		ClusterID:  rcfg.ClusterID,
		MsgGroupID: rcfg.MsgGroupRef.Inc(),
		NodeType:   c.metadata.NodeType,
	}

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
