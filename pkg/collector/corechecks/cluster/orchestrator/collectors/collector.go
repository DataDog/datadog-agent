// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package collectors

import (
	"fmt"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// Collector is an interface that represents the collection process for a
// resource type.
type Collector interface {
	// Init is where the collector initialization happens. It is used to create
	// informers and listers.
	Init(*CollectorRunConfig)

	// Metadata is used to access information describing the collector.
	Metadata() *CollectorMetadata

	// Run triggers the collection process given a configuration and returns the
	// collection result. Returns an error if the collection failed.
	Run(*CollectorRunConfig) (*CollectorRunResult, error)

	// Process is used to process the list of resources and return the result.
	Process(rcfg *CollectorRunConfig, list interface{}) (*CollectorRunResult, error)
}

// CollectorMetadata contains information about a collector.
type CollectorMetadata struct {
	IsDefaultVersion                     bool
	IsMetadataProducer                   bool
	IsManifestProducer                   bool
	IsStable                             bool
	SupportsManifestBuffering            bool
	Name                                 string
	NodeType                             pkgorchestratormodel.NodeType
	Kind                                 string
	Version                              string
	IsSkipped                            bool
	SkippedReason                        string
	LabelsAsTags                         map[string]string
	AnnotationsAsTags                    map[string]string
	SupportsTerminatedResourceCollection bool
	IsGenericCollector                   bool
}

// CollectorTags returns static tags to be added to all resources collected by the collector.
func (cm CollectorMetadata) CollectorTags() []string {
	// This is only set for Kubernetes collectors that rely on a dedicated resource API.
	// This is not applicable to certain collectors like ECS Task collector or Kubernetes cluster collector.
	if cm.Version == "" {
		return nil
	}
	return []string{fmt.Sprintf("kube_api_version:%s", cm.Version)}
}

// FullName returns a string that contains the collector name and version.
func (cm CollectorMetadata) FullName() string {
	if cm.Version != "" {
		return fmt.Sprintf("%s/%s", cm.Version, cm.Name)
	}
	return cm.Name
}

// K8sCollectorRunConfig is the configuration used to initialize or run the kubernetes collector.
type K8sCollectorRunConfig struct {
	APIClient                   *apiserver.APIClient
	OrchestratorInformerFactory *OrchestratorInformerFactory
}

// ECSCollectorRunConfig is the configuration used to initialize or run the ECS collector.
type ECSCollectorRunConfig struct {
	WorkloadmetaStore workloadmeta.Component
	AWSAccountID      string
	Region            string
	ClusterName       string
	SystemInfo        *model.SystemInfo
	HostName          string
}

// CollectorRunConfig is the configuration used to initialize or run the
// collector.
type CollectorRunConfig struct {
	K8sCollectorRunConfig
	ECSCollectorRunConfig
	ClusterID           string
	Config              *config.OrchestratorConfig
	MsgGroupRef         *atomic.Int32
	TerminatedResources bool
	AgentVersion        *model.AgentVersion
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
