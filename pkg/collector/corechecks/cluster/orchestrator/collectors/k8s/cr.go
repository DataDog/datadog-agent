// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	defaultMaximumCRDQuota = 10000
)

// NewCRCollectorVersion builds the group of collector versions.
func NewCRCollectorVersion(resource string, groupVersion string) (*CRCollector, error) {
	return NewCRCollector(resource, groupVersion)
}

// CRCollector is a collector for Kubernetes Custom Resources.
// See https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/ for more detail.
type CRCollector struct {
	gvr             schema.GroupVersionResource
	informer        informers.GenericInformer
	lister          cache.GenericLister
	maximumCRDQuota int
	metadata        *collectors.CollectorMetadata
	processor       *processors.Processor
}

// NewCRCollector creates a new collector for Kubernetes CRs.
func NewCRCollector(name string, groupVersion string) (*CRCollector, error) {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return nil, err
	}
	return &CRCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             false,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			SupportsManifestBuffering:            false,
			Name:                                 name,
			NodeType:                             orchestrator.K8sCR,
			Version:                              groupVersion,
			SupportsTerminatedResourceCollection: true,
		},
		gvr:       gv.WithResource(name),
		processor: processors.NewProcessor(new(k8sProcessors.CRHandlers)),

		// Allow users to set max number of custom resources to collect
		// but do not allow that to exceeded our definied maximum
		maximumCRDQuota: min(pkgconfigsetup.Datadog().GetInt("orchestrator_explorer.custom_resources.max_count"), defaultMaximumCRDQuota),
	}, nil
}

// Informer returns the shared informer.
func (c *CRCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

func (c *CRCollector) getGRV() schema.GroupVersionResource {
	return c.gvr
}

// Init is used to initialize the collector.
func (c *CRCollector) Init(rcfg *collectors.CollectorRunConfig) {
	grv := c.getGRV()
	c.informer = rcfg.OrchestratorInformerFactory.DynamicInformerFactory.ForResource(grv)
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *CRCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *CRCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}
	if len(list) > c.maximumCRDQuota {
		return nil, collectors.NewListingError(fmt.Errorf("crd collector %s/%s has reached to the limit %d, skipping it", c.metadata.Version, c.metadata.Name, c.maximumCRDQuota))
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *CRCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
