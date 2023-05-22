// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package collectors

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	"go.uber.org/atomic"
	"k8s.io/client-go/tools/cache"
)

// Collector is an interface that represents the collection process for a
// resource type.
type Collector interface {
	// Informer returns the shared informer for that resource.
	Informer() cache.SharedInformer

	// Init is where the collector initialization happens. It is used to create
	// informers and listers.
	Init(*CollectorRunConfig)

	// IsAvailable returns whether a collector is available.
	// A typical use-case is checking whether the targeted apiGroup version
	// used by the collector is available in the cluster.
	// Should be called after Init.
	// FIXME: to be removed after collector discovery has been the default for
	// some time.
	IsAvailable() bool

	// Metadata is used to access information describing the collector.
	Metadata() *CollectorMetadata

	// Run triggers the collection process given a configuration and returns the
	// collection result. Returns an error if the collection failed.
	Run(*CollectorRunConfig) (*CollectorRunResult, error)
}

// CollectorVersions represents the list of collector implementations that are
// supported, each one being tied to a specific kubernetes group and version.
type CollectorVersions struct {
	Collectors []Collector
}

// NewCollectorVersions is used to build the collector version list.
func NewCollectorVersions(versions ...Collector) CollectorVersions {
	return CollectorVersions{
		versions,
	}
}

// CollectorForVersion retrieves the collector implementing a given version. If
// no collector is known for that version, returns (nil, false).
func (cv *CollectorVersions) CollectorForVersion(version string) (Collector, bool) {
	for _, collector := range cv.Collectors {
		if collector.Metadata().Version == version {
			return collector, true
		}
	}
	return nil, false
}

// CollectorMetadata contains information about a collector.
type CollectorMetadata struct {
	IsDefaultVersion          bool
	IsMetadataProducer        bool
	IsManifestProducer        bool
	IsStable                  bool
	SupportsManifestBuffering bool
	Name                      string
	NodeType                  orchestrator.NodeType
	Version                   string
}

// FullName returns a string that contains the collector name and version.
func (cm CollectorMetadata) FullName() string {
	if cm.Version != "" {
		return fmt.Sprintf("%s/%s", cm.Version, cm.Name)
	}
	return cm.Name
}

// CollectorRunConfig is the configuration used to initialize or run the
// collector.
type CollectorRunConfig struct {
	APIClient   *apiserver.APIClient
	ClusterID   string
	Config      *config.OrchestratorConfig
	MsgGroupRef *atomic.Int32
}

// CollectorRunResult contains information about what the collector has done.
// Metadata is a list of payload, each payload contains a list of k8s resources metadata and manifest
// Manifests is a list of payload, each payload contains a list of k8s resources manifest.
// Manifests is a copy of part of Metadata
type CollectorRunResult struct {
	Result             processors.ProcessResult
	ResourcesListed    int
	ResourcesProcessed int
}

func NewProcessorContext(rcfg *CollectorRunConfig, metadata *CollectorMetadata) *processors.ProcessorContext {
	return &processors.ProcessorContext{
		APIClient:          rcfg.APIClient,
		ApiGroupVersionTag: fmt.Sprintf("kube_api_version:%s", metadata.Version),
		Cfg:                rcfg.Config,
		ClusterID:          rcfg.ClusterID,
		MsgGroupID:         rcfg.MsgGroupRef.Inc(),
		NodeType:           metadata.NodeType,
	}
}
