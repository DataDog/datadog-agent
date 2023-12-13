// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	"k8s.io/apimachinery/pkg/types"
)

// CRDHandlers implements the Handlers interface for Kubernetes CronJobs.
type CRDHandlers struct {
	BaseHandlers
}

// BuildManifestMessageBody builds the manifest payload body
func (crd *CRDHandlers) BuildManifestMessageBody(ctx *processors.ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody {
	cm := ExtractModelManifests(ctx, resourceManifests, groupSize)
	return &model.CollectorManifestCRD{
		Manifest: cm,
		Tags:     append(ctx.Cfg.ExtraTags, ctx.ApiGroupVersionTag),
	}
}

// AfterMarshalling is a handler called after resource marshalling.
func (crd *CRDHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (crd *CRDHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (crd *CRDHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (crd *CRDHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]runtime.Object)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (crd *CRDHandlers) ResourceUID(ctx *processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*v1.CustomResourceDefinition).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (crd *CRDHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*v1.CustomResourceDefinition).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (crd *CRDHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*v1.CustomResourceDefinition)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}
