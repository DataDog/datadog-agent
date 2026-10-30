// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	"fmt"

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

var secretGVR = schema.GroupVersionResource{
	Group:    utilTypes.SecretGroup,
	Version:  utilTypes.SecretVersion,
	Resource: utilTypes.SecretName,
}

// NewSecretCollectorVersions builds the group of collector versions.
func NewSecretCollectorVersions(_ tagger.Component) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewSecretCollector(),
	)
}

// SecretCollector is a collector for Kubernetes Secrets.
// It relies on a metadata-only informer so that Secret data and stringData
// are never fetched from the API server nor held in the informer cache.
type SecretCollector struct {
	informer  informers.GenericInformer
	lister    metadatalister.Lister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
	// initErr is set when the cache transform could not be registered. The
	// collector then refuses to run, since cached objects may still hold the
	// last-applied-configuration annotation, which contains the Secret data.
	initErr error
}

// NewSecretCollector creates a new collector for the Kubernetes Secret resource.
func NewSecretCollector() *SecretCollector {
	return &SecretCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			IsStable:                             false,
			SupportsManifestBuffering:            true,
			SupportsTerminatedResourceCollection: true,
			Group:                                utilTypes.SecretGroup,
			Version:                              utilTypes.SecretVersion,
			Kind:                                 kubernetes.SecretKind,
			Name:                                 utilTypes.SecretName,
			NodeType:                             orchestrator.K8sSecret,
		},
		processor: processors.NewProcessor(k8sProcessors.NewSecretHandlers()),
	}
}

// Informer returns the shared informer.
func (c *SecretCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *SecretCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.MetadataInformerFactory.ForResource(secretGVR)
	if err := c.informer.Informer().SetTransform(trimSecretMetadata); err != nil {
		c.initErr = fmt.Errorf("unable to set transform on the Secret informer, Secret collection is disabled: %w", err)
		log.Warn(c.initErr)
	}
	c.lister = metadatalister.New(c.informer.Informer().GetIndexer(), secretGVR)
}

// trimSecretMetadata drops fields that are never sent before objects are
// stored in the informer cache. Sensitive annotations and labels are deleted
// rather than redacted: the last-applied-configuration annotation, when
// present, holds a full copy of the Secret, including its data.
func trimSecretMetadata(obj interface{}) (interface{}, error) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.ManagedFields = nil
		for _, key := range redact.GetSensitiveAnnotationsAndLabels() {
			delete(m.Annotations, key)
			delete(m.Labels, key)
		}
	}
	return obj, nil
}

// Metadata is used to access information about the collector.
func (c *SecretCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *SecretCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *SecretCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

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
