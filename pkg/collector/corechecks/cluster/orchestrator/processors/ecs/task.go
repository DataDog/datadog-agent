// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package ecs defines handlers for processing ECS tasks
package ecs

import (
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	transformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/ecs"
)

// TaskHandlers implements the Handlers interface for ECS Tasks.
type TaskHandlers struct {
	common.BaseHandlers
	tagger tagger.Component
}

// NewTaskHandlers returns a new TaskHandlers.
func NewTaskHandlers(tagger tagger.Component) *TaskHandlers {
	return &TaskHandlers{
		tagger: tagger,
	}
}

// BuildMessageBody is a handler called to build a message body out of a list of extracted resources.
func (t *TaskHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.ECSProcessorContext)
	models := make([]*model.ECSTask, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.ECSTask))
	}

	return &model.CollectorECSTask{
		AwsAccountID: int64(pctx.AWSAccountID),
		ClusterName:  pctx.ClusterName,
		ClusterId:    pctx.ClusterID,
		Region:       pctx.Region,
		GroupId:      pctx.MsgGroupID,
		GroupSize:    int32(groupSize),
		Tasks:        models,
		Info:         pctx.SystemInfo,
		HostName:     pctx.Hostname,
		Tags:         pctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (t *TaskHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(transformers.TaskWithContainers)
	return transformers.ExtractECSTask(r, t.tagger)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (t *TaskHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]transformers.TaskWithContainers)

	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (t *TaskHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return types.UID(resource.(transformers.TaskWithContainers).Task.EntityID.ID)
}

// ResourceVersion sets and returns custom resource version for an ECS task.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (t *TaskHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resourceModel.(*model.ECSTask).ResourceVersion
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *TaskHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *TaskHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
}
