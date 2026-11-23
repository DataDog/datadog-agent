// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package registry defines the Kubernetes resources that the orchestrator check collects, and how to list them.
package registry

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// Resource is an entry of the resource registry. Adding an entry is enough to collect a new resource type.
type Resource struct {
	// Group is the API group.
	Group string
	// Versions are the API versions, preferred first. Custom resources use the first served version, built-in resources
	// get a collector for each version.
	Versions []string
	// Resource is the resource name. For a custom resource, an empty name selects every resource of the group.
	Resource string
	// Kind is the resource kind. Discovery provides the kind of custom resources.
	Kind string
	// NodeType is the payload type. Custom resources use orchestrator.K8sCR.
	NodeType pkgorchestratormodel.NodeType
	// Enabled reports whether the resource is collected when the check configuration does not select collectors. Nil
	// means always.
	Enabled func() bool
	// IgnoreDeletions disables the collection of deleted resources from the informer.
	IgnoreDeletions bool
	// ManifestOnly marks a resource that has handlers but no structured metadata payload.
	ManifestOnly bool
	// NewHandlers creates the handlers that shape the metadata and manifest of a version: scrubbing, tags and identity.
	// Nil processes the resource as a generic unstructured object, which only has a manifest.
	NewHandlers func(d Dependencies, version string) processors.Handlers
	// Informer creates the informer that lists the resource. Nil uses the typed informer factory.
	Informer InformerFunc
}

// Dependencies are the components that the handlers use.
type Dependencies struct {
	Config config.Component
	Store  workloadmeta.Component
	Tagger tagger.Component
}

// InformerFunc creates the informer of a resource. onDelete reports a deleted resource that no informer reports.
type InformerFunc func(rcfg *collectors.CollectorRunConfig, gvr schema.GroupVersionResource, onDelete func(obj any)) (cache.SharedIndexInformer, error)

// IsCustomResource reports whether the entry is a custom resource, selected through API discovery.
func (r Resource) IsCustomResource() bool {
	return r.NodeType == orchestrator.K8sCR
}

// HasMetadata reports whether the resource has a structured metadata payload.
func (r Resource) HasMetadata() bool {
	return r.NewHandlers != nil && !r.ManifestOnly
}

// IsEnabled reports whether the resource is collected when the check configuration does not select collectors.
func (r Resource) IsEnabled() bool {
	return r.Enabled == nil || r.Enabled()
}

// Resources returns the resource registry.
func Resources() []Resource {
	return resourceRegistry
}

// builtin returns a registry entry for a built-in resource. Its kind is the name of its node type.
func builtin(nodeType pkgorchestratormodel.NodeType, group, version, resource string, options ...resourceOption) Resource {
	r := Resource{Group: group, Versions: []string{version}, Resource: resource, Kind: nodeType.String(), NodeType: nodeType}
	for _, option := range options {
		option(&r)
	}
	return r
}

// customResource returns a registry entry for a custom resource collected by default.
func customResource(group, resource string, enabled func() bool, versions ...string) Resource {
	return Resource{Group: group, Versions: versions, Resource: resource, NodeType: orchestrator.K8sCR, Enabled: enabled}
}

// resourceOption sets an optional field of a built-in registry entry.
type resourceOption func(*Resource)

// withHandlers sets handlers that only depend on the tagger, which most resources use.
func withHandlers[H processors.Handlers](newHandlers func(tagger.Component) H) resourceOption {
	return withHandlerFunc(func(d Dependencies, _ string) processors.Handlers { return newHandlers(d.Tagger) })
}

func withHandlerFunc(newHandlers func(d Dependencies, version string) processors.Handlers) resourceOption {
	return func(r *Resource) { r.NewHandlers = newHandlers }
}

func withInformer(informer InformerFunc) resourceOption {
	return func(r *Resource) { r.Informer = informer }
}

func withEnabled(enabled func() bool) resourceOption {
	return func(r *Resource) { r.Enabled = enabled }
}

// withExtraVersions adds older versions, each collected by its own collector.
func withExtraVersions(versions ...string) resourceOption {
	return func(r *Resource) { r.Versions = append(r.Versions, versions...) }
}

// withManifestHandlers sets the handlers of a resource that only has a manifest.
func withManifestHandlers[P common.TypedObject[P]]() resourceOption {
	return func(r *Resource) {
		r.NewHandlers = func(Dependencies, string) processors.Handlers { return new(common.ManifestHandlers[P]) }
		r.ManifestOnly = true
	}
}

func manifestOnly() resourceOption {
	return func(r *Resource) { r.ManifestOnly = true }
}

func withoutDeletions() resourceOption {
	return func(r *Resource) { r.IgnoreDeletions = true }
}
