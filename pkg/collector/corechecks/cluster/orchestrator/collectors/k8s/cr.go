// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package k8s

import (
	"fmt"
	"strings"

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
func NewCRCollectorVersions(grv string) *CRCollector {
	return NewCRCollector(grv)
}

// CRCollector is a collector for Kubernetes CRs.
type CRCollector struct {
	grv       string
	informer  informers.GenericInformer
	lister    cache.GenericLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewCRCollector creates a new collector for the Kubernetes CR
// resource.
func NewCRCollector(grv string) *CRCollector {
	grvSplit := strings.Split(grv, "/")

	return &CRCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion: true,
			IsStable:         false,
			Name:             fmt.Sprintf("%s", grvSplit[3]),
			NodeType:         orchestrator.K8sCR,
			Version:          fmt.Sprintf("%s/%s", grvSplit[0], grvSplit[1]),
		},
		grv:       grv,
		processor: processors.NewProcessor(new(k8sProcessors.CRHandlers)),
	}
}

// Informer returns the shared informer.
func (c *CRCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// input: crd/datadoghq.com/v1alpha1/DatadogMetric output -> schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetric"}
func (c *CRCollector) getGRV() schema.GroupVersionResource {
	// TODO: add special handling for non proper formatted cr
	list := strings.Split(c.grv, "/")
	if len(list) != 4 {
		panic("not ok!")
	}
	return schema.GroupVersionResource{Group: list[1], Version: list[2], Resource: list[3]}
}

// Init is used to initialize the collector.
func (c *CRCollector) Init(rcfg *collectors.CollectorRunConfig) {
	grv := c.getGRV()
	c.informer = rcfg.APIClient.DynamicInformerFactory.ForResource(grv)
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
	list, err := c.lister.List(labels.Everything()) // later panics because I cannot convert unstructured
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
