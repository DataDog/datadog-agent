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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NewCRCollectorVersions builds the group of collector versions.
func NewCRCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewCRCollector(),
	)
}

// CRCollector is a collector for Kubernetes CRs.
type CRCollector struct {
	informer  informers.GenericInformer
	lister    cache.GenericLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewCRCollector creates a new collector for the Kubernetes CR
// resource.
func NewCRCollector() *CRCollector {
	return &CRCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion: true,
			IsStable:         false,
			Name:             "customresources",
			NodeType:         orchestrator.K8sCR,
		},
		processor: processors.NewProcessor(new(k8sProcessors.CRHandlers)),
	}
}

// Informer returns the shared informer.
// TODO: we can init the informer here
func (c *CRCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *CRCollector) Init(rcfg *collectors.CollectorRunConfig) {
	// TODO: assume we have a list of names in the init config where we can make a map of CR <> cr
	grv := schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogagents"} // that's a CR
	c.informer = rcfg.APIClient.DDInformerFactory.ForResource(grv)
	c.lister = c.informer.Lister() // return that Lister
}

// IsAvailable returns whether the collector is available.
func (c *CRCollector) IsAvailable() bool { return true }

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

	ctx := &processors.ProcessorContext{
		APIClient:  rcfg.APIClient,
		Cfg:        rcfg.Config,
		ClusterID:  rcfg.ClusterID,
		MsgGroupID: rcfg.MsgGroupRef.Inc(),
		NodeType:   c.metadata.NodeType,
	}

	processResult, processed := c.processor.Process(ctx, list)
	processResult.MetadataMessages = nil

	// This would happen when recovering from a processor panic. In the nominal
	// case we would have a positive integer set at the very end of processing.
	// If this is not the case then it means code execution stopped sooner.
	// Panic recovery will log more information about the error so we can figure
	// out the root cause.
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
