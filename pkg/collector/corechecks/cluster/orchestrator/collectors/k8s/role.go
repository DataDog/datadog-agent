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

// NewRoleCollectorVersions builds the group of collector versions.
func NewRoleCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewRoleCollector(),
	)
}

// RoleCollector is a collector for Kubernetes Roles.
type RoleCollector struct {
	informer  rbacv1Informers.RoleInformer
	lister    rbacv1Listers.RoleLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewRoleCollector creates a new collector for the Kubernetes Role resource.
func NewRoleCollector() *RoleCollector {
	return &RoleCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "roles",
			NodeType:                  orchestrator.K8sRole,
			Version:                   "rbac.authorization.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.RoleHandlers)),
	}
}

// Informer returns the shared informer.
func (c *RoleCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *RoleCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Rbac().V1().Roles()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *RoleCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *RoleCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *RoleCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
