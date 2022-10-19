// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// NewCRDCollectorVersions builds the group of collector versions.
func NewCRDCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewCRDCollector(),
	)
}

// CRDCollector is a collector for Kubernetes CRDs.
type CRDCollector struct {
	informer  informers.GenericInformer
	lister    cache.GenericLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewCRDCollector creates a new collector for the Kubernetes CRD
// resource.
func NewCRDCollector() *CRDCollector {
	return &CRDCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion: true,
			IsStable:         false,
			Name:             "customresourcedefinitions",
			NodeType:         orchestrator.K8sCRD,
		},
		processor: processors.NewProcessor(new(k8sProcessors.CRDHandlers)),
	}
}

// Informer returns the shared informer.
func (c *CRDCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *CRDCollector) Init(rcfg *collectors.CollectorRunConfig) {
	grv := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "CustomResourceDefinition"} // that's a CRD
	c.informer = rcfg.APIClient.DDInformerFactory.ForResource(grv)
	c.lister = c.informer.Lister() // return that Lister
}

// IsAvailable returns whether the collector is available.
func (c *CRDCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *CRDCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *CRDCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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

	// This would happen when recovering from a processor panic. In the nominal
	// case we would have a positive integer set at the very end of processing.
	// If this is not the case then it means code execution stopped sooner.
	// Panic recovery will log more information about the error so we can figure
	// out the root cause.
	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	// The CRD processor can return errors since it has to grab extra
	// information from the API server during processing.
	if err != nil {
		return nil, collectors.NewProcessingError(err)
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    1,
		ResourcesProcessed: processed,
	}

	return result, nil
}
