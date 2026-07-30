// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package common provides basic handlers used by orchestrator processor
package common

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

//nolint:revive // TODO(CAPP) Fix revive linter
type BaseHandlers struct{}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) EnrichModel(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) ScrubBeforeMarshalling(ctx processors.ProcessorContext, resource interface{}) {}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) BuildManifestMessageBody(ctx processors.ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody {
	return ExtractModelManifests(ctx, resourceManifests, groupSize)
}

// CloneResource returns the resource unchanged. Handlers that mutate resources
// during ScrubBeforeExtraction, BeforeMarshalling, or ScrubBeforeMarshalling
// must override this to return a deep copy so the informer cache is not corrupted.
//
//nolint:revive
func (BaseHandlers) CloneResource(resource interface{}) interface{} {
	return resource
}

// ResourceVersionFromRaw returns an empty string, indicating that the resource
// version cannot be determined without model extraction. Override in handlers
// where the version is directly available from the raw resource.
//
//nolint:revive
func (BaseHandlers) ResourceVersionFromRaw(_ processors.ProcessorContext, _ interface{}) string {
	return ""
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) GetNodeName(ctx processors.ProcessorContext, resource interface{}) string {
	return ""
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (BaseHandlers) GetMetadataTags(ctx processors.ProcessorContext, resource interface{}) []string {
	return nil
}

// ExtractModelManifests creates the model manifest from the given manifests
func ExtractModelManifests(ctx processors.ProcessorContext, resourceManifests []interface{}, groupSize int) *model.CollectorManifest {
	pctx := ctx.(*processors.K8sProcessorContext)
	manifests := make([]*model.Manifest, 0, len(resourceManifests))

	for _, m := range resourceManifests {
		manifests = append(manifests, m.(*model.Manifest))
	}

	cm := &model.CollectorManifest{
		ClusterName:     pctx.Cfg.KubeClusterName,
		ClusterId:       pctx.ClusterID,
		Manifests:       manifests,
		GroupId:         pctx.MsgGroupID,
		GroupSize:       int32(groupSize),
		Tags:            pctx.Cfg.ExtraTags,
		HostName:        pctx.HostName,
		SystemInfo:      pctx.SystemInfo,
		AgentVersion:    ctx.GetAgentVersion(),
		OriginCollector: model.OriginCollector_datadogAgent,
	}
	return cm
}
