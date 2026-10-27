// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/metadata/metadatalister"
	"k8s.io/client-go/tools/cache"
)

var configMapGVR = schema.GroupVersionResource{
	Group:    utilTypes.ConfigMapGroup,
	Version:  utilTypes.ConfigMapVersion,
	Resource: utilTypes.ConfigMapName,
}

// NewConfigMapCollectorVersions builds the group of collector versions.
func NewConfigMapCollectorVersions(_ tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewConfigMapCollector(),
	)
}

// ConfigMapCollector is a collector for Kubernetes ConfigMaps.
// It relies on a metadata-only informer so that ConfigMap data and binaryData
// are never fetched from the API server nor held in the informer cache.
type ConfigMapCollector struct {
	informer  informers.GenericInformer
	lister    metadatalister.Lister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewConfigMapCollector creates a new collector for the Kubernetes ConfigMap resource.
func NewConfigMapCollector() *ConfigMapCollector {
	return &ConfigMapCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			IsStable:                             false,
			SupportsManifestBuffering:            true,
			SupportsTerminatedResourceCollection: true,
			Group:                                utilTypes.ConfigMapGroup,
			Version:                              utilTypes.ConfigMapVersion,
			Kind:                                 kubernetes.ConfigMapKind,
			Name:                                 utilTypes.ConfigMapName,
			NodeType:                             orchestrator.K8sConfigMap,
		},
		processor: processors.NewProcessor(k8sProcessors.NewConfigMapHandlers()),
	}
}

// Informer returns the shared informer.
func (c *ConfigMapCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *ConfigMapCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.MetadataInformerFactory.ForResource(configMapGVR)
	if err := c.informer.Informer().SetTransform(trimConfigMapMetadata); err != nil {
		log.Debugf("Unable to set transform on the ConfigMap informer: %v", err)
	}
	c.lister = metadatalister.New(c.informer.Informer().GetIndexer(), configMapGVR)
}

// trimConfigMapMetadata drops fields that are never sent before objects are
// stored in the informer cache, to reduce its memory footprint. The
// last-applied-configuration annotation, when present, holds a full copy of
// the ConfigMap data.
func trimConfigMapMetadata(obj interface{}) (interface{}, error) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(m.Annotations, m.Labels)
	}
	return obj, nil
}

// Metadata is used to access information about the collector.
func (c *ConfigMapCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ConfigMapCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *ConfigMapCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
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
