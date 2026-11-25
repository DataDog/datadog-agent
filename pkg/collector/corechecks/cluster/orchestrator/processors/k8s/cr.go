// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

// CRHandlers implements the Handlers interface for Kubernetes custom resources and generic resources.
type CRHandlers struct {
	common.TypedHandlers[*unstructured.Unstructured]
	// IsGenericResource is a flag to indicate if the resource is a generic resource.
	IsGenericResource bool
}

// BeforeMarshalling keeps the kind and the API version that unstructured objects get from the API server.
func (cr *CRHandlers) BeforeMarshalling(processors.ProcessorContext, interface{}, interface{}) (skip bool) {
	return
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	return nil
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) BuildManifestMessageBody(ctx processors.ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	cm := common.ExtractModelManifests(ctx, resourceManifests, groupSize)
	if cr.IsGenericResource {
		return cm
	}

	return &model.CollectorManifestCR{
		Manifest: cm,
		// CRs are manifests, CollectorTags should be added to the inner Manifests, not the outer (embedded) CollectorManifests
		Tags: pctx.Cfg.ExtraTags,
	}
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return nil
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) ResourceList(_ processors.ProcessorContext, list interface{}) []interface{} {
	return common.RuntimeObjectList(list)
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	return nil
}

// ScrubBeforeExtraction is a handler called to redact the raw resource before
// it is extracted as an internal resource model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (cr *CRHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	cr.TypedHandlers.ScrubBeforeExtraction(ctx, resource)
	if pctx := ctx.(*processors.K8sProcessorContext); pctx.Cfg.IsScrubbingEnabled {
		redact.ScrubCRManifest(resource.(*unstructured.Unstructured), pctx.Cfg.Scrubber)
	}
}
