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
	rbacv1Informers "k8s.io/client-go/informers/rbac/v1"
	rbacv1Listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

// NewClusterRoleCollectorVersions builds the group of collector versions.
func NewClusterRoleCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewClusterRoleCollector(),
	)
}

// ClusterRoleCollector is a collector for Kubernetes ClusterRoles.
type ClusterRoleCollector struct {
	informer  rbacv1Informers.ClusterRoleInformer
	lister    rbacv1Listers.ClusterRoleLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewClusterRoleCollector creates a new collector for the Kubernetes
// ClusterRole resource.
func NewClusterRoleCollector() *ClusterRoleCollector {
	return &ClusterRoleCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "clusterroles",
			NodeType:                  orchestrator.K8sClusterRole,
			Version:                   "rbac.authorization.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.ClusterRoleHandlers)),
	}
}

// Informer returns the shared informer.
func (c *ClusterRoleCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *ClusterRoleCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Rbac().V1().ClusterRoles()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *ClusterRoleCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *ClusterRoleCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ClusterRoleCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
