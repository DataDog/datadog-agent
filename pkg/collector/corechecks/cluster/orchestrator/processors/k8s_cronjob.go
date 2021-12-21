// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

// K8sCronJobHandlers implements the Handlers interface for Kubernetes CronJobs.
type K8sCronJobHandlers struct{}

// AfterMarshalling is a handler called after resource marshalling.
func (h *K8sCronJobHandlers) AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	m := resourceModel.(*model.CronJob)
	m.Yaml = yaml
	return
}

// BeforeCacheCheck is a handler called before cache lookup.
func (h *K8sCronJobHandlers) BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
func (h *K8sCronJobHandlers) BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (h *K8sCronJobHandlers) BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	models := make([]*model.CronJob, 0, len(resourceModels))

	for _, m := range resourceModels {
		models = append(models, m.(*model.CronJob))
	}

	return &model.CollectorCronJob{
		ClusterName: ctx.Cfg.KubeClusterName,
		ClusterId:   ctx.ClusterID,
		GroupId:     ctx.MsgGroupID,
		GroupSize:   int32(groupSize),
		CronJobs:    models,
		Tags:        ctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (h *K8sCronJobHandlers) ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{}) {
	r := resource.(*batchv1beta1.CronJob)
	return transformers.ExtractK8sCronJob(r)
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (h *K8sCronJobHandlers) ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*batchv1beta1.CronJob)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (h *K8sCronJobHandlers) ResourceUID(ctx *ProcessorContext, resource, resourceModel interface{}) types.UID {
	return resource.(*batchv1beta1.CronJob).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (h *K8sCronJobHandlers) ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*batchv1beta1.CronJob).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (h *K8sCronJobHandlers) ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*batchv1beta1.CronJob)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (h *K8sCronJobHandlers) ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{}) {
	r := resource.(*batchv1beta1.CronJob)
	if ctx.Cfg.IsScrubbingEnabled {
		for c := 0; c < len(r.Spec.JobTemplate.Spec.Template.Spec.InitContainers); c++ {
			redact.ScrubContainer(&r.Spec.JobTemplate.Spec.Template.Spec.InitContainers[c], ctx.Cfg.Scrubber)
		}
		for c := 0; c < len(r.Spec.JobTemplate.Spec.Template.Spec.Containers); c++ {
			redact.ScrubContainer(&r.Spec.JobTemplate.Spec.Template.Spec.Containers[c], ctx.Cfg.Scrubber)
		}
	}
}
