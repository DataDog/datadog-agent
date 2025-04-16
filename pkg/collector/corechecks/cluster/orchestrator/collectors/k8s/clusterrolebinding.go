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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	rbacv1Informers "k8s.io/client-go/informers/rbac/v1"
	rbacv1Listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

// NewClusterRoleBindingCollectorVersions builds the group of collector versions.
func NewClusterRoleBindingCollectorVersions(metadataAsTags utils.MetadataAsTags) collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewClusterRoleBindingCollector(metadataAsTags),
	)
}

// ClusterRoleBindingCollector is a collector for Kubernetes ClusterRoleBindings.
type ClusterRoleBindingCollector struct {
	informer  rbacv1Informers.ClusterRoleBindingInformer
	lister    rbacv1Listers.ClusterRoleBindingLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewClusterRoleBindingCollector creates a new collector for the Kubernetes
// ClusterRoleBinding resource.
func NewClusterRoleBindingCollector(metadataAsTags utils.MetadataAsTags) *ClusterRoleBindingCollector {
	resourceType := getResourceType(clusterRoleBindingName, clusterRoleBindingVersion)
	labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()[resourceType]
	annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()[resourceType]

	return &ClusterRoleBindingCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             true,
			IsMetadataProducer:                   true,
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            true,
			Name:                                 clusterRoleBindingName,
			Kind:                                 kubernetes.ClusterRoleBindingKind,
			NodeType:                             orchestrator.K8sClusterRoleBinding,
			Version:                              clusterRoleBindingVersion,
			LabelsAsTags:                         labelsAsTags,
			AnnotationsAsTags:                    annotationsAsTags,
			SupportsTerminatedResourceCollection: true,
		},
		processor: processors.NewProcessor(new(k8sProcessors.ClusterRoleBindingHandlers)),
	}
}

// Informer returns the shared informer.
func (c *ClusterRoleBindingCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *ClusterRoleBindingCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.OrchestratorInformerFactory.InformerFactory.Rbac().V1().ClusterRoleBindings()
	c.lister = c.informer.Lister()
}

// Metadata is used to access information about the collector.
func (c *ClusterRoleBindingCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ClusterRoleBindingCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	list, err := c.lister.List(labels.Everything())
	if err != nil {
		return nil, collectors.NewListingError(err)
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result.
func (c *ClusterRoleBindingCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	processResult, processed := c.processor.Process(ctx, list)

	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result:             processResult,
		ResourcesListed:    len(c.processor.Handlers().ResourceList(ctx, list)),
		ResourcesProcessed: processed,
	}

	return result, nil
}
