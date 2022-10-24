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
func NewCRCollectorVersions(grv string) (*CRCollector, error) {
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
func NewCRCollector(grv string) (*CRCollector, error) {
	grvSplit := strings.Split(grv, "/")
	if len(grvSplit) < 3 {
		return nil, fmt.Errorf("GRV needs to be of the following format: <apigroup_and_version>/<collector_name")
	}
	version := fmt.Sprintf("%s/%s", grvSplit[0], grvSplit[1])
	_, err := schema.ParseGroupVersion(version)
	if err != nil {
		return nil, err
	}
	return &CRCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion: true,
			IsStable:         false,
			Name:             fmt.Sprintf("%s", grvSplit[2]),
			NodeType:         orchestrator.K8sCR,
			Version:          version,
		},
		grv:       grv,
		processor: processors.NewProcessor(new(k8sProcessors.CRHandlers)),
	}, nil
}

// Informer returns the shared informer.
func (c *CRCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

func (c *CRCollector) getGRV() schema.GroupVersionResource {
	version, _ := schema.ParseGroupVersion(c.metadata.Version)
	return version.WithResource(c.metadata.Name)
}

// Init is used to initialize the collector.
func (c *CRCollector) Init(rcfg *collectors.CollectorRunConfig) {
	grv := c.getGRV()
	c.informer = rcfg.APIClient.DynamicInformerFactory.ForResource(grv)
	c.lister = c.informer.Lister()
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
