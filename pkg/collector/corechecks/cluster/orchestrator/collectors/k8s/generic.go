// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// GenericResource is a generic resource that can be used to collect data from a Kubernetes API.
// It collects data as regular manifests, not CR manifests.
type GenericResource struct {
	Name     string
	Group    string
	Version  string
	Stable   bool
	NodeType pkgorchestratormodel.NodeType
}

// NewCollectorVersions creates a new collector versions for the generic resource.
func (r GenericResource) NewCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(r.NewGenericCollector())
}

// NewGenericCollector creates a new generic collector for the generic resource.
func (r GenericResource) NewGenericCollector() *CRCollector {
	gvr := schema.GroupVersionResource{
		Group:    r.Group,
		Resource: r.Name,
		Version:  r.Version,
	}
	return &CRCollector{
		metadata: &collectors.CollectorMetadata{
			IsDefaultVersion:                     true,
			IsStable:                             r.Stable,
			IsManifestProducer:                   true,
			IsMetadataProducer:                   false,
			SupportsManifestBuffering:            true,
			Name:                                 r.Name,
			NodeType:                             r.NodeType,
			Group:                                r.Group,
			Version:                              r.Version,
			SupportsTerminatedResourceCollection: true,
			IsGenericCollector:                   true,
		},
		gvr:       gvr,
		processor: processors.NewProcessor(&k8sProcessors.CRHandlers{IsGenericResource: true}),
	}
}
