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

// NewRoleBindingCollectorVersions builds the group of collector versions.
func NewRoleBindingCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewRoleBindingCollector(),
	)
}

// RoleBindingCollector is a collector for Kubernetes RoleBindings.
type RoleBindingCollector struct {
	informer  rbacv1Informers.RoleBindingInformer
	lister    rbacv1Listers.RoleBindingLister
	metadata  *collectors.CollectorMetadata
	processor *processors.Processor
}

// NewRoleBindingCollector creates a new collector for the Kubernetes
// RoleBinding resource.
func NewRoleBindingCollector() *RoleBindingCollector {
	return &RoleBindingCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:          true,
			IsStable:                  true,
			IsMetadataProducer:        true,
			IsManifestProducer:        true,
			SupportsManifestBuffering: true,
			Name:                      "rolebindings",
			NodeType:                  orchestrator.K8sRoleBinding,
			Version:                   "rbac.authorization.k8s.io/v1",
		},
		processor: processors.NewProcessor(new(k8sProcessors.RoleBindingHandlers)),
	}
}

// Informer returns the shared informer.
func (c *RoleBindingCollector) Informer() cache.SharedInformer {
	return c.informer.Informer()
}

// Init is used to initialize the collector.
func (c *RoleBindingCollector) Init(rcfg *collectors.CollectorRunConfig) {
	c.informer = rcfg.APIClient.InformerFactory.Rbac().V1().RoleBindings()
	c.lister = c.informer.Lister()
}

// IsAvailable returns whether the collector is available.
func (c *RoleBindingCollector) IsAvailable() bool { return true }

// Metadata is used to access information about the collector.
func (c *RoleBindingCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *RoleBindingCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
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
