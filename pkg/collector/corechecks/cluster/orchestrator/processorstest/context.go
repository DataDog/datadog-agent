// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

//nolint:revive
package processorstest

import (
	"github.com/benbjohnson/clock"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// ProcessorContext is a test context for processors.
type ProcessorContext struct {
	AgentVersion        *model.AgentVersion
	APIVersion          string
	Kind                string
	Clock               *clock.Mock
	ClusterID           string
	CollectorTags       []string
	HostName            string
	ManifestProducer    bool
	MsgGroupID          int32
	NodeType            pkgorchestratormodel.NodeType
	OrchestratorConfig  *config.OrchestratorConfig
	SystemInfo          *model.SystemInfo
	TerminatedResources bool
}

// NewProcessorContext creates a new test ProcessorContext.
func NewProcessorContext() *ProcessorContext {
	return &ProcessorContext{
		AgentVersion: &model.AgentVersion{
			Major:  1,
			Minor:  0,
			Patch:  0,
			Commit: "commit",
		},
		APIVersion:       "apiGroup/v1",
		Kind:             "ResourceKind",
		Clock:            clock.NewMock(),
		ClusterID:        "cluster-id",
		CollectorTags:    []string{"collector_tag:collector_tag_value"},
		HostName:         "host-name",
		ManifestProducer: true,
		MsgGroupID:       1,
		NodeType:         1,
		OrchestratorConfig: &config.OrchestratorConfig{
			ExtraTags:                      []string{"extra_tag:extra_tag_value"},
			IsManifestCollectionEnabled:    true,
			KubeClusterName:                "cluster",
			MaxPerMessage:                  100,
			OrchestrationCollectionEnabled: true,
		},
		SystemInfo: &model.SystemInfo{
			Uuid: "system-uuid",
		},
		TerminatedResources: false,
	}
}

//nolint:revive
func (pc *ProcessorContext) GetAgentVersion() *model.AgentVersion {
	return pc.AgentVersion
}

//nolint:revive
func (pc *ProcessorContext) GetAPIVersion() string {
	return pc.APIVersion
}

//nolint:revive
func (pc *ProcessorContext) GetClock() clock.Clock {
	return pc.Clock
}

//nolint:revive
func (pc *ProcessorContext) GetClusterID() string {
	return pc.ClusterID
}

//nolint:revive
func (pc *ProcessorContext) GetCollectorTags() []string {
	return pc.CollectorTags
}

//nolint:revive
func (pc *ProcessorContext) GetKind() string {
	return pc.Kind
}

//nolint:revive
func (pc *ProcessorContext) GetHostName() string {
	return pc.HostName
}

//nolint:revive
func (pc *ProcessorContext) GetMsgGroupID() int32 {
	return pc.MsgGroupID
}

//nolint:revive
func (pc *ProcessorContext) GetNodeType() pkgorchestratormodel.NodeType {
	return pc.NodeType
}

//nolint:revive
func (pc *ProcessorContext) GetOrchestratorConfig() *config.OrchestratorConfig {
	return pc.OrchestratorConfig
}

//nolint:revive
func (pc *ProcessorContext) IsManifestProducer() bool {
	return pc.ManifestProducer
}

//nolint:revive
func (pc *ProcessorContext) IsTerminatedResources() bool {
	return pc.TerminatedResources
}

//nolint:revive
func (pc *ProcessorContext) GetSystemInfo() *model.SystemInfo {
	return pc.SystemInfo
}
