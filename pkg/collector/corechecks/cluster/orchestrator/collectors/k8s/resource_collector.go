// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ResourceCollector collects one version of a resource of the registry, from its informer store. A manifest collector
// only sends manifests, as regular manifest payloads whatever the resource type.
type ResourceCollector struct {
	collectors.TerminatedQueue
	resource      registry.Resource
	metadata      *collectors.CollectorMetadata
	processor     *processors.Processor
	informer      cache.SharedIndexInformer
	manifestsOnly bool
	// initErr is the informer creation error. The collector then fails closed: it sends nothing.
	initErr error
	// maxCount is the maximum number of resources to list, or 0 when unlimited.
	maxCount int
}

// NewResourceCollector creates a collector for a version of a resource of the registry. manifestsOnly selects a
// collector of the dedicated manifest pipeline.
func NewResourceCollector(resource registry.Resource, version string, d registry.Dependencies, manifestsOnly bool) *ResourceCollector {
	// Custom resources keep their envelope in the combined pipeline.
	var handlers processors.Handlers = &k8sProcessors.CRHandlers{IsGenericResource: !resource.IsCustomResource()}
	if resource.NewHandlers != nil {
		handlers = resource.NewHandlers(d, version)
	}

	return &ResourceCollector{
		resource: resource,
		// The collector bundle selects collectors from their metadata. Buffered payloads are regular manifest payloads,
		// which cannot carry the CR and CRD envelopes of the combined pipeline.
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     version == resource.Versions[0],
			IsStable:                             resource.IsEnabled(),
			IsMetadataProducer:                   !manifestsOnly && resource.HasMetadata(),
			IsManifestProducer:                   true,
			SupportsManifestBuffering:            manifestsOnly || (resource.NodeType != orchestrator.K8sCRD && !resource.IsCustomResource()),
			SupportsTerminatedResourceCollection: !resource.IgnoreDeletions,
			Group:                                resource.Group,
			Version:                              version,
			Name:                                 resource.Resource,
			Kind:                                 resource.Kind,
			NodeType:                             resource.NodeType,
		},
		processor:     processors.NewProcessor(handlers),
		manifestsOnly: manifestsOnly,
	}
}

// defaultMaximumCRDQuota is the maximum number of resources of a custom resource collector.
const defaultMaximumCRDQuota = 10000

// NewCRCollector creates the collector of a custom resource that API discovery selected. It lists at most
// orchestrator_explorer.custom_resources.max_count resources.
func NewCRCollector(name, groupVersion string) (*ResourceCollector, error) {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return nil, err
	}
	collector := NewResourceCollector(registry.Resource{
		Group:    gv.Group,
		Versions: []string{gv.Version},
		Resource: name,
		NodeType: orchestrator.K8sCR,
		Enabled:  registry.Never,
		Informer: registry.DynamicInformer,
	}, gv.Version, registry.Dependencies{}, false)
	collector.maxCount = min(pkgconfigsetup.Datadog().GetInt("orchestrator_explorer.custom_resources.max_count"), defaultMaximumCRDQuota)
	return collector, nil
}

// ForManifests returns the collector of the manifest pipeline for the same resource. It keeps the kind that discovery
// set and the resource quota.
func (c *ResourceCollector) ForManifests() *ResourceCollector {
	resource := c.resource
	resource.Kind = c.metadata.Kind
	isStable := c.metadata.IsStable
	resource.Enabled = func() bool { return isStable }
	collector := NewResourceCollector(resource, c.metadata.Version, registry.Dependencies{}, true)
	collector.maxCount = c.maxCount
	return collector
}

// GVR returns the group, version and resource of the collector.
func (c *ResourceCollector) GVR() schema.GroupVersionResource {
	return c.gvr()
}

// Informer returns the shared informer, or nil when the resource has no informer.
func (c *ResourceCollector) Informer() cache.SharedInformer {
	if c.informer == nil {
		return nil
	}
	return c.informer
}

// Init is used to initialize the collector.
func (c *ResourceCollector) Init(rcfg *collectors.CollectorRunConfig) {
	newInformer := c.resource.Informer
	if newInformer == nil {
		newInformer = registry.TypedInformer
	}

	c.SetTerminatedResourceGate(rcfg.TerminatedResourceGate)
	c.informer, c.initErr = newInformer(rcfg, c.gvr(), c.QueueTerminated)
	if c.initErr != nil {
		log.Warnf("Unable to create the informer of the %s collector: %s", c.metadata.FullName(), c.initErr)
	}
}

// Metadata is used to access information about the collector.
func (c *ResourceCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

// Run triggers the collection process.
func (c *ResourceCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	var list []interface{}
	if c.informer != nil {
		list = c.informer.GetStore().List()
	}
	if c.maxCount > 0 && len(list) > c.maxCount {
		return nil, collectors.NewListingError(fmt.Errorf("crd collector %s/%s has reached to the limit %d, skipping it", c.metadata.GroupVersion(), c.metadata.Name, c.maxCount))
	}

	return c.Process(rcfg, list)
}

// Process is used to process the list of resources and return the result. The list is either the content of an
// informer store or a slice of terminated resources.
func (c *ResourceCollector) Process(rcfg *collectors.CollectorRunConfig, list interface{}) (*collectors.CollectorRunResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	ctx := collectors.NewK8sProcessorContext(rcfg, c.metadata)

	resources := toInterfaceSlice(list)
	if !c.manifestsOnly {
		processResult, listed, processed := c.processor.ProcessList(ctx, resources)
		if processed == -1 {
			return nil, collectors.ErrProcessingPanic
		}
		return &collectors.CollectorRunResult{Result: processResult, ResourcesListed: listed, ResourcesProcessed: processed}, nil
	}

	manifests, processed := c.processor.ProcessManifests(ctx, resources)
	if processed == -1 {
		return nil, collectors.ErrProcessingPanic
	}

	result := &collectors.CollectorRunResult{
		Result: processors.ProcessResult{
			ManifestMessages: processors.ChunkManifest(ctx, common.BaseHandlers{}.BuildManifestMessageBody, manifests),
		},
		ResourcesListed:    len(resources),
		ResourcesProcessed: processed,
	}

	return result, nil
}

func (c *ResourceCollector) gvr() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: c.metadata.Group, Version: c.metadata.Version, Resource: c.metadata.Name}
}

// toInterfaceSlice converts the list of an informer store, or the typed slice of terminated resources, to a slice of
// interfaces.
func toInterfaceSlice(list interface{}) []interface{} {
	switch resources := list.(type) {
	case nil:
		return nil
	case []interface{}:
		return resources
	}

	value := reflect.ValueOf(list)
	if value.Kind() != reflect.Slice {
		log.Warnf("Unexpected resource list type %T", list)
		return nil
	}
	converted := make([]interface{}, 0, value.Len())
	for i := range value.Len() {
		converted = append(converted, value.Index(i).Interface())
	}
	return converted
}

// NewResourceCollectorVersions returns the collectors of the built-in resources of the registry, one for each version,
// and the cluster collector. Custom resources are selected through API discovery.
func NewResourceCollectorVersions(d registry.Dependencies, manifestsOnly bool) []collectors.CollectorVersions {
	versions := make([]collectors.CollectorVersions, 0, len(registry.Resources())+1)
	for _, resource := range registry.Resources() {
		if resource.IsCustomResource() {
			continue
		}
		resourceCollectors := make([]collectors.K8sCollector, 0, len(resource.Versions))
		for _, version := range resource.Versions {
			resourceCollectors = append(resourceCollectors, NewResourceCollector(resource, version, d, manifestsOnly))
		}
		versions = append(versions, collectors.NewCollectorVersions(resourceCollectors...))
	}
	// The cluster is not a Kubernetes resource: its manifest aggregates the nodes.
	if manifestsOnly {
		return append(versions, collectors.NewCollectorVersions(NewClusterManifestCollector()))
	}
	return append(versions, NewClusterCollectorVersions())
}
