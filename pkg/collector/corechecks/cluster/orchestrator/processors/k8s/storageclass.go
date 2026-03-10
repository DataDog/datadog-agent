// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	storagev1 "k8s.io/api/storage/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/redact"

	"k8s.io/apimachinery/pkg/types"
)

// StorageClassHandlers implements the Handlers interface for Kubernetes StorageClass.
type StorageClassHandlers struct {
	common.BaseHandlers
	tagger tagger.Component
}

// NewStorageClassHandlers creates a new StorageClassHandlers.
func NewStorageClassHandlers(tagger tagger.Component) *StorageClassHandlers {
	return &StorageClassHandlers{tagger: tagger}
}

// BeforeCacheCheck is a handler called before cache lookup.
//
//nolint:revive
func (h *StorageClassHandlers) BeforeCacheCheck(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*storagev1.StorageClass)
	m := resourceModel.(*model.StorageClass)

	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), "", r.Name)),
	)
	taggerTags, err := h.tagger.Tag(entityID, taggertypes.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for storageclass %s: %s", r.Name, err)
		return
	}

	m.Tags = append(m.Tags, taggerTags...)
	return
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive
func (h *StorageClassHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *StorageClassHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	r := resource.(*storagev1.StorageClass)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *StorageClassHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]*model.StorageClass, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.StorageClass))
	}

	return &model.CollectorStorageClass{
		ClusterName:    pctx.Cfg.KubeClusterName,
		ClusterId:      pctx.ClusterID,
		GroupId:        pctx.MsgGroupID,
		GroupSize:      int32(groupSize),
		StorageClasses: models,
		Tags:           util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion:   ctx.GetAgentVersion(),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive
func (h *StorageClassHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (StorageClassModel interface{}) {
	r := resource.(*storagev1.StorageClass)
	return k8sTransformers.ExtractStorageClass(ctx, r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive
func (h *StorageClassHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*storagev1.StorageClass)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource.DeepCopy())
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
//
//nolint:revive
func (h *StorageClassHandlers) ResourceUID(ctx processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*storagev1.StorageClass).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
//
//nolint:revive
func (h *StorageClassHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*storagev1.StorageClass).ResourceVersion
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (h *StorageClassHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(*model.StorageClass)
	if !ok {
		return nil
	}
	return m.Tags
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive
func (h *StorageClassHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*storagev1.StorageClass)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}
