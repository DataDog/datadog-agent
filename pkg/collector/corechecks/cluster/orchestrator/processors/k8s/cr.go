// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"k8s.io/apimachinery/pkg/types"
)

// CRHandlers implements the Handlers interface for Kubernetes CronJobs.
type CRHandlers struct {
	BaseHandlers
}

// AfterMarshalling is a handler called after resource marshalling.
func (cr *CRHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (cr *CRHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	return nil
}

func (cr *CRHandlers) BuildManifestMessageBody(ctx *processors.ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody {
	cm := ExtractModelManifests(ctx, resourceManifests, groupSize)
	return &model.CollectorManifestCR{
		Manifest: cm,
		Tags:     append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (cr *CRHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return nil
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (cr *CRHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]runtime.Object)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (cr *CRHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*unstructured.Unstructured).GetUID()
}

// ResourceVersion is a handler called to retrieve the resource version.
func (cr *CRHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*unstructured.Unstructured).GetResourceVersion()
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (cr *CRHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*unstructured.Unstructured)
	annotations := r.GetAnnotations()
	redact.RemoveLastAppliedConfigurationAnnotation(annotations)
	r.SetAnnotations(annotations)
}
