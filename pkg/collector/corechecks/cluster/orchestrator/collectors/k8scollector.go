// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package collectors

import (
	"k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	vpai "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

// K8sCollector is an interface that represents the collection process for a k8s resource type.
type K8sCollector interface {
	Collector

	// Informer returns the shared informer for that resource.
	Informer() cache.SharedInformer
}

// CollectorVersions represents the list of collector implementations that are
// supported, each one being tied to a specific kubernetes group and version.
type CollectorVersions struct {
	Collectors []K8sCollector
}

// NewCollectorVersions is used to build the collector version list.
func NewCollectorVersions(versions ...K8sCollector) CollectorVersions {
	return CollectorVersions{
		versions,
	}
}

// CollectorForVersion retrieves the collector implementing a given version. If
// no collector is known for that version, returns (nil, false).
func (cv *CollectorVersions) CollectorForVersion(version string) (K8sCollector, bool) {
	for _, collector := range cv.Collectors {
		if collector.Metadata().Version == version {
			return collector, true
		}
	}
	return nil, false
}

// OrchestratorInformerFactory contains all informer factories used by the orchestration check
type OrchestratorInformerFactory struct {
	InformerFactory              informers.SharedInformerFactory
	UnassignedPodInformerFactory informers.SharedInformerFactory
	TerminatedPodInformerFactory informers.SharedInformerFactory
	DynamicInformerFactory       dynamicinformer.DynamicSharedInformerFactory
	CRDInformerFactory           externalversions.SharedInformerFactory
	VPAInformerFactory           vpai.SharedInformerFactory
}

// NewK8sProcessorContext creates a new processor context for k8s resources.
func NewK8sProcessorContext(rcfg *CollectorRunConfig, metadata *CollectorMetadata) *processors.K8sProcessorContext {
	return &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:                 rcfg.Config,
			MsgGroupID:          rcfg.MsgGroupRef.Inc(),
			NodeType:            metadata.NodeType,
			ManifestProducer:    true,
			ClusterID:           rcfg.ClusterID,
			Kind:                metadata.Kind,
			APIVersion:          metadata.Version,
			CollectorTags:       metadata.CollectorTags(),
			TerminatedResources: rcfg.TerminatedResources,
			AgentVersion:        rcfg.AgentVersion,
		},
		APIClient:         rcfg.APIClient,
		LabelsAsTags:      metadata.LabelsAsTags,
		AnnotationsAsTags: metadata.AnnotationsAsTags,
		HostName:          rcfg.HostName,
	}
}
