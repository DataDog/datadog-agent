// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/redact"

	"k8s.io/apimachinery/pkg/types"
)

// CRHandlers implements the Handlers interface for Kubernetes CronJobs.
type CRHandlers struct{}

// AfterMarshalling is a handler called after resource marshalling.
// TODO: make it work with CRD
func (crd *CRHandlers) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BeforeCacheCheck is a handler called before cache lookup.
func (crd *CRHandlers) BeforeCacheCheck(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
func (crd *CRHandlers) BeforeMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
func (crd *CRHandlers) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	// TODO: make it work with CRD
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
func (crd *CRHandlers) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return nil
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
func (crd *CRHandlers) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	resourceList := list.([]*v1.CustomResourceDefinition)
	resources = make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		resources = append(resources, resource)
	}

	return resources
}

// ResourceUID is a handler called to retrieve the resource UID.
func (crd *CRHandlers) ResourceUID(ctx *processors.ProcessorContext, resource, resourceModel interface{}) types.UID {
	return resource.(*v1.CustomResourceDefinition).UID
}

// ResourceVersion is a handler called to retrieve the resource version.
func (crd *CRHandlers) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	return resource.(*v1.CustomResourceDefinition).ResourceVersion
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
func (crd *CRHandlers) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	r := resource.(*v1.CustomResourceDefinition)
	redact.RemoveLastAppliedConfigurationAnnotation(r.Annotations)
}

// ScrubBeforeMarshalling is a handler called to redact the raw resource before
// it is marshalled to generate a manifest.
func (crd *CRHandlers) ScrubBeforeMarshalling(ctx *processors.ProcessorContext, resource interface{}) {
	return
}
