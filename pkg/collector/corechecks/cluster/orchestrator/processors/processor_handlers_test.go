// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
)

type TestResourceHandlers struct {
	SkipAfterMarshalling  bool
	SkipBeforeCacheCheck  bool
	SkipBeforeMarshalling bool
	PanicInResourceList   bool
}

//nolint:revive
func (rh *TestResourceHandlers) AfterMarshalling(ctx ProcessorContext, resource, resourceModel any, yaml []byte) (skip bool) {
	if rh.SkipAfterMarshalling {
		return true
	}
	resource.(*processorstest.Resource).PropertyToSetAfterMarshalling = "value-after-marshalling"
	resourceModel.(*processorstest.Resource).PropertyToSetAfterMarshalling = "value-after-marshalling"
	return false
}

//nolint:revive
func (rh *TestResourceHandlers) BeforeCacheCheck(ctx ProcessorContext, resource, resourceModel any) (skip bool) {
	if rh.SkipBeforeCacheCheck {
		return true
	}
	resource.(*processorstest.Resource).PropertyToSetBeforeCacheCheck = "value-before-cache-check"
	resourceModel.(*processorstest.Resource).PropertyToSetBeforeCacheCheck = "value-before-cache-check"
	return false
}

//nolint:revive
func (rh *TestResourceHandlers) BeforeMarshalling(ctx ProcessorContext, resource, resourceModel any) (skip bool) {
	if rh.SkipBeforeMarshalling {
		return true
	}
	resource.(*processorstest.Resource).PropertyToSetBeforeMarshalling = "value-before-marshalling"
	resourceModel.(*processorstest.Resource).PropertyToSetBeforeMarshalling = "value-before-marshalling"
	return false
}

//nolint:revive
func (rh *TestResourceHandlers) BuildMessageBody(ctx ProcessorContext, resourceModels []any, groupSize int) model.MessageBody {
	pctx := ctx.(*processorstest.ProcessorContext)
	models := make([]*model.Manifest, 0, len(resourceModels))

	for _, resourceModel := range resourceModels {
		models = append(models, &model.Manifest{
			Content: processorstest.MustMarshalJSON(resourceModel),
		})
	}

	return &model.CollectorManifest{
		AgentVersion: ctx.GetAgentVersion(),
		ClusterName:  pctx.GetOrchestratorConfig().KubeClusterName,
		ClusterId:    pctx.GetClusterID(),
		GroupId:      pctx.GetMsgGroupID(),
		GroupSize:    int32(groupSize),
		Manifests:    models,
	}
}

//nolint:revive
func (rh *TestResourceHandlers) BuildManifestMessageBody(ctx ProcessorContext, resourceManifests []any, groupSize int) model.MessageBody {
	pctx := ctx.(*processorstest.ProcessorContext)
	manifests := make([]*model.Manifest, 0, len(resourceManifests))

	for _, resourceManifest := range resourceManifests {
		manifests = append(manifests, resourceManifest.(*model.Manifest))
	}

	return &model.CollectorManifest{
		AgentVersion: ctx.GetAgentVersion(),
		ClusterName:  pctx.GetOrchestratorConfig().KubeClusterName,
		ClusterId:    pctx.GetClusterID(),
		GroupId:      pctx.GetMsgGroupID(),
		GroupSize:    int32(groupSize),
		HostName:     string(pctx.HostName),
		SystemInfo:   pctx.GetSystemInfo(),
		Manifests:    manifests,
	}
}

//nolint:revive
func (rh *TestResourceHandlers) ExtractResource(ctx ProcessorContext, resource any) (resourceModel any) {
	return resource.(*processorstest.Resource).DeepCopy()
}

//nolint:revive
func (rh *TestResourceHandlers) GetMetadataTags(ctx ProcessorContext, resourceMetadataModel any) []string {
	return []string{"metadata_tag:metadata_tag_value"}
}

//nolint:revive
func (rh *TestResourceHandlers) GetNodeName(ctx ProcessorContext, resource any) string {
	return "node"
}

//nolint:revive
func (rh *TestResourceHandlers) ResourceList(ctx ProcessorContext, list any) (resources []any) {
	if rh.PanicInResourceList {
		panic("panicked on purpose")
	}
	resources = make([]any, 0, len(list.([]*processorstest.Resource)))
	for _, resource := range list.([]*processorstest.Resource) {
		resources = append(resources, resource.DeepCopy())
	}
	return resources
}

//nolint:revive
func (rh *TestResourceHandlers) ResourceUID(ctx ProcessorContext, resource any) types.UID {
	return types.UID(resource.(*processorstest.Resource).ResourceUID)
}

//nolint:revive
func (rh *TestResourceHandlers) ResourceVersion(ctx ProcessorContext, resource, resourceModel any) string {
	return resource.(*processorstest.Resource).ResourceVersion
}

//nolint:revive
func (rh *TestResourceHandlers) ScrubBeforeExtraction(ctx ProcessorContext, resource any) {
	resource.(*processorstest.Resource).PropertyToScrubBeforeExtraction = "scrubbed-before-extraction"
}

//nolint:revive
func (rh *TestResourceHandlers) ScrubBeforeMarshalling(ctx ProcessorContext, resource any) {
	resource.(*processorstest.Resource).PropertyToScrubBeforeMarshalling = "scrubbed-before-marshalling"
}
